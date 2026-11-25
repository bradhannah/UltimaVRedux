package main

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
)

const combatKeyRepeatMsPerSecond int64 = 1000

// combatKeyRepeat is the Combat Key Repeat (Measured by default). It owns one physical key from
// a fresh press until release, never outlives its encounter, and only ever
// replays the key through the ordinary command route. The press may come at a
// ready party turn, while an enemy turn or the party's own action is still
// playing, or as the A that confirms an attack.
type combatKeyRepeat struct {
	active    bool
	early     bool // pressed before the turn was ready; its first hold starts when it is
	key       ebiten.Key
	nextAt    int64 // the next repeated action may start at or after this time
	confirmAt int64 // a held A confirms an open aim at or after this time
	encounter *game_state.CombatState
}

func (g *GameScene) combatKeyRepeatRate() int {
	return g.gameConfig.GetCombatKeyRepeat()
}

// combatKeyRepeatIntervalMs is the hard cap between repeated actions, shared by
// every party member. Measured uses the walking pace outside combat.
func combatKeyRepeatIntervalMs(rate int) int64 {
	if rate == config.CombatKeyRepeatMeasured {
		return keyPressDelay
	}

	return combatKeyRepeatMsPerSecond / int64(rate)
}

// combatRepeatableKey reports the keys a rate repeats: arrows (including the
// keypad arrows) and Tab at every rate, and A only at the Fast rates.
func combatRepeatableKey(key ebiten.Key, rate int) bool {
	if rate <= 0 {
		return false
	}
	if key == ebiten.KeyA {
		return rate > config.CombatKeyRepeatMeasured
	}
	movement := primaryMovementKey(key)

	return key == ebiten.KeyTab || movement == ebiten.KeyUp || movement == ebiten.KeyDown ||
		movement == ebiten.KeyLeft || movement == ebiten.KeyRight
}

// combatRepeatKey owns a held repeat key at a ready party turn, as
// explorationPassKey owns Space. A fresh press acts at once; later actions wait
// for the first hold and the rate cap. One Update emits at most one action and
// never catches up missed intervals. Any other fresh press ends the repeat and
// acts normally.
func (g *GameScene) combatRepeatKey() (*ebiten.Key, bool) {
	state := &g.transientCommandInput
	rate := g.combatKeyRepeatRate()
	combat := g.gameState.CombatScene
	if rate == 0 || combat == nil || !combat.CombatTurnsStarted() || state.sample.HasModifiers() {
		return nil, false
	}
	repeat := &state.combatRepeat
	if repeat.active && g.keyboard.IsSuppressedUntilRelease(repeat.key) {
		*repeat = combatKeyRepeat{}
	}
	now := g.getElapsedMs()
	for _, key := range boundKeysGame {
		if !state.sample.Pressed[key] || repeat.active && key == repeat.key {
			continue
		}
		g.stopCombatKeyRepeat()
		if !combatRepeatableKey(key, rate) || !g.keyboard.ConsumePressEdge(state.sample, key) {
			return nil, false
		}
		*repeat = combatKeyRepeat{
			active: true, key: key, encounter: combat,
			nextAt:    now + max(passInitialDelayMs, combatKeyRepeatIntervalMs(rate)),
			confirmAt: now + passInitialDelayMs,
		}

		return &key, true
	}
	if !repeat.active {
		return nil, false
	}
	if repeat.early {
		repeat.early = false
		repeat.nextAt = now + max(passInitialDelayMs, combatKeyRepeatIntervalMs(rate))
		repeat.confirmAt = now + passInitialDelayMs
	}
	if now < repeat.nextAt {
		return nil, true
	}
	repeat.nextAt = now + combatKeyRepeatIntervalMs(rate)
	repeat.confirmAt = now // A repeated attack confirms its aim at once.
	key := repeat.key

	return &key, true
}

// adoptEarlyCombatRepeatKey makes a repeat key that goes down while an enemy
// turn or the party's own action is still playing the repeat's key. Without
// it the press is lost and, while the key stays down, every hand-over waits
// for its release, so the battle stands still. Only a fresh press of a key
// this rate repeats is taken, alone, with no prompt, reader or dialog open and
// no queued solo command. A key that was already down before, or that went
// down in an aim or another prompt, keeps waiting for its release as before.
// It acts only after the usual first hold, counted from the ready turn.
func (g *GameScene) adoptEarlyCombatRepeatKey() {
	state := &g.transientCommandInput
	rate := g.combatKeyRepeatRate()
	combat := g.gameState.CombatScene
	if rate == 0 || combat == nil || !combat.CombatTurnsStarted() || !g.combatActionPlaying() ||
		state.combatRepeat.active || state.pending.active || state.sample.HasModifiers() {
		return
	}
	for _, key := range boundKeysGame {
		if !state.sample.Pressed[key] {
			continue
		}
		if combatRepeatableKey(key, rate) && !g.freshPressOtherThan(key) && !g.combatKeyRepeatInterrupted(key) &&
			!g.keyboard.IsSuppressedUntilRelease(key) && g.keyboard.ConsumePressEdge(state.sample, key) {
			state.combatRepeat = combatKeyRepeat{
				active: true, early: true, key: key, nextAt: 0, confirmAt: 0, encounter: combat,
			}
		}

		return
	}
}

// repeatConfirmingAttackKey makes the A that confirms an attack by hand the
// repeat's key at a rate that repeats A, so holding it attacks again at the
// next turn, as an A held from the ready turn does.
func (g *GameScene) repeatConfirmingAttackKey() {
	state := &g.transientCommandInput
	rate := g.combatKeyRepeatRate()
	if state.combatRepeat.active || !combatRepeatableKey(ebiten.KeyA, rate) || state.sample.HasModifiers() {
		return
	}
	now := g.getElapsedMs()
	state.combatRepeat = combatKeyRepeat{
		active: true, early: false, key: ebiten.KeyA, encounter: g.gameState.CombatScene,
		nextAt:    now + max(passInitialDelayMs, combatKeyRepeatIntervalMs(rate)),
		confirmAt: now + passInitialDelayMs,
	}
}

// observeCombatKeyRepeat runs every frame. Release ends a repeat; another
// fresh key, leaving its encounter, turning the setting Off, or anything else
// taking input stops it until the key is released and pressed again.
func (g *GameScene) observeCombatKeyRepeat() {
	state := &g.transientCommandInput
	repeat := state.combatRepeat
	switch {
	case !repeat.active:
	case !state.sample.Held[repeat.key]:
		state.combatRepeat = combatKeyRepeat{}
	case repeat.encounter != g.gameState.CombatScene || g.combatKeyRepeatRate() == 0 ||
		g.freshPressOtherThan(repeat.key) || g.combatKeyRepeatInterrupted(repeat.key):
		g.stopCombatKeyRepeat()
	}
}

// freshPressOtherThan reports another bound key pressed in this frame's sample,
// such as Escape while a held A aims.
func (g *GameScene) freshPressOtherThan(key ebiten.Key) bool {
	sample := &g.transientCommandInput.sample
	for _, bound := range boundKeysGame {
		if bound != key && sample.Pressed[bound] {
			return true
		}
	}

	return false
}

// combatActionPlaying reports an enemy turn or a party action still playing:
// the battle is taking no command.
func (g *GameScene) combatActionPlaying() bool {
	return g.inputBlocked || g.gameState.EventQueueManager.HasInputBlockingWorkflows()
}

// combatKeyRepeatInterrupted reports a prompt, reader or dialog. Only a held A
// may continue through its own attack aim. Any repeat key may wait out an
// attack that is still playing from that aim; if the aim then asks again, for
// a second weapon, the key is interrupted like any other.
func (g *GameScene) combatKeyRepeatInterrupted(key ebiten.Key) bool {
	aiming := g.secondaryKeyState == CombatAttackTargetInput && (key == ebiten.KeyA || g.combatActionPlaying())

	return g.secondaryKeyState != PrimaryInput && !aiming || g.atlasSession != nil || g.journalSession != nil ||
		g.finalePresentation != nil || g.dialogStack.HasModalDialog()
}

// stopCombatKeyRepeat ends a repeat and holds its key back until release, so
// it cannot act again in this or any later owner.
func (g *GameScene) stopCombatKeyRepeat() {
	if repeat := g.transientCommandInput.combatRepeat; repeat.active {
		g.keyboard.SuppressUntilRelease(repeat.key)
	}
	g.transientCommandInput.combatRepeat = combatKeyRepeat{}
}

// repeatCombatAttackConfirmation confirms an open aim for a held A. An aim that
// opened on the attacker has no target, so the repeat stops for aiming by hand.
func (g *GameScene) repeatCombatAttackConfirmation(targeting *game_state.CombatAttackTargeting) bool {
	repeat := g.transientCommandInput.combatRepeat
	if !repeat.active || repeat.key != ebiten.KeyA || g.getElapsedMs() < repeat.confirmAt {
		return false
	}
	if targeting.Cursor == targeting.Origin {
		g.stopCombatKeyRepeat()

		return false
	}
	g.confirmCombatAttack(targeting)

	return true
}

// combatTurnReleaseReady lets an enemy turn or automatic pass hand over while
// only protected keys, or the held repeat key, are down.
func (g *GameScene) combatTurnReleaseReady() bool {
	if repeat := g.transientCommandInput.combatRepeat; repeat.active {
		return !g.keyboard.HasUnprotectedHeldKeyExcept(repeat.key)
	}

	return !g.keyboard.HasUnprotectedHeldKey()
}

// releaseAfterCombatAction keeps a party action's release gate, except that the
// held repeat key may stay down. With the setting Off the workflow is unchanged.
func (g *GameScene) releaseAfterCombatAction(
	queue *event_queue.SequentialEventBuilder,
) *event_queue.SequentialEventBuilder {
	if g.combatKeyRepeatRate() == 0 {
		return queue
	}

	return queue.InputReleaseReady(func() bool {
		if repeat := g.transientCommandInput.combatRepeat; repeat.active {
			return !g.keyboard.HasUnprotectedHeldKeyExcept(repeat.key)
		}

		return !g.IsAnyKeyPressed()
	})
}
