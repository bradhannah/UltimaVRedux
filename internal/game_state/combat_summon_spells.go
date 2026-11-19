package game_state

import "github.com/bradhannah/UltimaVRedux/internal/references"

const (
	combatSummonPlacementAttempts = 8
	combatSummonRandomCoordinate  = 16
	inBetXenSummonCount           = 4
)

// CastCombatKalXen summons one party-loyal rat, spider, bat, or snake.
func (g *GameState) CastCombatKalXen() CombatSummonResult {
	return g.castCombatKalXen(g.AsRandomProvider())
}

func (g *GameState) castCombatKalXen(rng references.RandomProvider) CombatSummonResult {
	monsterType := kalXenMonsterType(rng.Intn(16))

	return g.summonCombatMonsters(references.KalXen, monsterType, 1, CombatMonsterLoyaltyParty, rng)
}

func kalXenMonsterType(roll int) references.MonsterType {
	switch {
	case roll < 6:
		return references.MonsterRat
	case roll < 11:
		return references.MonsterSpider
	case roll < 14:
		return references.MonsterBat
	default:
		return references.MonsterSnake
	}
}

// CastCombatInBetXen summons up to four independent party-loyal insects on one square.
func (g *GameState) CastCombatInBetXen() CombatSummonResult {
	return g.summonCombatMonsters(
		references.InBetXen,
		references.MonsterInsect,
		inBetXenSummonCount,
		CombatMonsterLoyaltyParty,
		g.AsRandomProvider(),
	)
}

// CastCombatKalXenCorp summons a daemon and attempts to bind it to the party.
func (g *GameState) CastCombatKalXenCorp(casterIdx int) CombatSummonResult {
	return g.castCombatKalXenCorp(casterIdx, g.AsRandomProvider(), func() int {
		return g.RollOriginalD30() // Daemon summoning calls transformed d30 after placement succeeds.
		// See ref:RNG-0001.
	})
}

// UseCombatKalXenCorpScroll summons a daemon with guaranteed party loyalty.
func (g *GameState) UseCombatKalXenCorpScroll(casterIdx int) CombatSummonResult {
	return g.castCombatKalXenCorp(casterIdx, g.AsRandomProvider(), func() int {
		return -1
	})
}

func (g *GameState) castCombatKalXenCorp(
	casterIdx int,
	rng references.RandomProvider,
	rollBinding func() int,
) CombatSummonResult {
	caster := g.PartyState.GetCharacter(casterIdx)
	if caster == nil {
		return CombatSummonResult{
			Spell: references.KalXenCorp, Outcome: CombatSummonInvalidCaster,
			Created: make([]CombatSummonedMonster, 0), Binding: CombatSummonBindingNotApplicable,
		}
	}

	result := g.summonCombatMonsters(
		references.KalXenCorp,
		references.MonsterDevil,
		1,
		CombatMonsterLoyaltyMonster,
		rng,
	)
	if result.Outcome != CombatSummonSucceeded {
		return result
	}

	if rollBinding() >= int(caster.Intelligence) {
		result.Binding = CombatSummonBindingFailed
	} else {
		monster := g.CombatScene.MonsterManager.GetMonster(result.Created[0].Index)
		monster.SetLoyalty(CombatMonsterLoyaltyParty)
		result.Binding = CombatSummonBoundToParty
	}
	g.publishCombatSummon(result)

	return result
}

func (g *GameState) summonCombatMonsters(
	spell references.Spell,
	monsterType references.MonsterType,
	requested int,
	loyalty CombatMonsterLoyalty,
	rng references.RandomProvider,
) CombatSummonResult {
	result := CombatSummonResult{
		Spell: spell, Outcome: CombatSummonCapacityFull,
		Created: make([]CombatSummonedMonster, 0, requested), Binding: CombatSummonBindingNotApplicable,
	}
	if g.CombatScene == nil {
		return result
	}

	enemyRef, err := g.enemyReferenceForCombat(monsterType)
	if err != nil {
		panic("summon monster reference is unavailable: " + err.Error())
	}
	placementType := monsterType
	if spell == references.KalXen {
		// Animal summoning always validates placement with the Rat shape,
		// even when the selected animal is a Spider, Bat, or Snake.
		// See ref:SPELL-0005.
		placementType = references.MonsterRat
	}
	position, attempts, found := g.findCombatSummonPosition(placementType, rng)
	result.Attempts = attempts
	if !found {
		result.Outcome = CombatSummonNoLegalPosition

		return result
	}

	for range requested {
		if !g.CombatScene.TurnQueue.hasFreeSourceSlot(sourceMonsterSlotStart) {
			break
		}
		monster := NewCombatMonster(monsterType, enemyRef, position, rng)
		if !g.CombatScene.hasActiveObjectSlot() {
			break
		}
		monster.SetLoyalty(loyalty)
		monster.summoned = true
		index := g.CombatScene.AddMonsterToQueue(monster)
		result.Created = append(result.Created, CombatSummonedMonster{
			CombatantID: monster.CombatantID(),
			Index:       index,
			MonsterType: monsterType,
			Position:    position,
		})
	}
	if len(result.Created) == 0 {
		return result
	}
	result.Outcome = CombatSummonSucceeded
	result.Count = len(result.Created)
	if loyalty == CombatMonsterLoyaltyParty {
		result.Binding = CombatSummonBoundToParty
	}
	if spell != references.KalXenCorp {
		g.publishCombatSummon(result)
	}

	return result
}

func (g *GameState) findCombatSummonPosition(
	placementType references.MonsterType,
	rng references.RandomProvider,
) (references.Position, int, bool) {
	candidateMonster := &CombatMonster{monsterType: placementType}
	for attempt := 1; attempt <= combatSummonPlacementAttempts; attempt++ {
		position := references.Position{
			X: references.Coordinate(rng.Intn(combatSummonRandomCoordinate)),
			Y: references.Coordinate(rng.Intn(combatSummonRandomCoordinate)),
		}
		if isCombatPositionValidForMonster(g, position, candidateMonster, -1) {
			return position, attempt, true
		}
	}

	return references.Position{}, combatSummonPlacementAttempts, false
}

func (g *GameState) publishCombatSummon(result CombatSummonResult) {
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind: CombatEventSummon,
		Summon: CombatSummonEvent{
			Spell: result.Spell, Created: result.Created, Count: result.Count, Binding: result.Binding,
		},
	})
}
