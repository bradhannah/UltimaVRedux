package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/camping"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// DungeonCampingPlan retains the source-selected encounter until an hourly ambush occurs.
type DungeonCampingPlan struct {
	combatMap    *references.CombatMapReference
	centerChest  *CombatObject
	selected     [references.MaxMonstersPerMap]bool
	monster      references.MonsterType
	monsterCount int
	command      DungeonCommandCycle
}

// PrepareDungeonCampingPlan builds the arena and consumes the original setup RNG
// before camping changes the active map.
func (g *GameState) PrepareDungeonCampingPlan(
	hours int,
	guardIndex int,
	command DungeonCommandCycle,
) (*camping.CampingScene, *DungeonCampingPlan, error) {
	dungeon := g.requireActiveDungeon()
	player := g.MapState.PlayerLocation
	combatMap, err := references.BuildGeneratedDungeonCampingMap(
		dungeon.location,
		&dungeon.cells,
		player.Floor,
		player.Position,
		dungeon.facing,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("building dungeon camping map: %w", err)
	}
	order := [references.MaxMonstersPerMap]int{}
	for index := range order {
		order[index] = index
	}
	for index := range order {
		randomIndex := g.GetRNG().Intn(len(order))
		order[index], order[randomIndex] = order[randomIndex], order[index]
	}
	monster := references.GetDungeonWanderingMonster(g.AsRandomProvider())
	maximum, forcedMaximum := dungeonWandererGroupSize(monster)
	monsterCount := g.GetRNG().Intn(maximum) + 1
	if forcedMaximum {
		monsterCount = maximum
	}
	selected := [references.MaxMonstersPerMap]bool{}
	for _, slot := range order[:monsterCount] {
		selected[slot] = true
	}

	scene := camping.NewDungeonCampingScene(hours, guardIndex, player, combatMap)
	var centerChest *CombatObject
	if chest, found := combatMap.GeneratedDungeonChest(); found {
		object := NewCombatChest(chest.Position, 3*int(chest.SourceFloor)+7, false)
		centerChest = &object
	}
	plan := &DungeonCampingPlan{
		combatMap: combatMap, centerChest: centerChest, selected: selected, monster: monster,
		monsterCount: monsterCount, command: command,
	}

	return scene, plan, nil
}

// QueueDungeonCampingAmbush materializes the preselected homogeneous group and
// defers combat until the camping workflow releases input ownership.
func (g *GameState) QueueDungeonCampingAmbush(
	scene *camping.CampingScene,
	plan *DungeonCampingPlan,
	campingState *CampingState,
	activePlayerIndex int,
) error {
	if plan == nil || plan.command.id == 0 || plan.combatMap == nil {
		return fmt.Errorf("%w: dungeon camping plan is incomplete", ErrInvalidCampingCombat)
	}
	if campingState == nil || !campingState.specialRingsInitialized {
		return fmt.Errorf("%w: camping party rings are not initialized", ErrInvalidCampingCombat)
	}
	if g.isValidCampingGuard(scene.GuardIndex) {
		g.setActiveCampingPartySleepStatus(false)
	}
	if plan.centerChest != nil {
		if _, materialized := plan.combatMap.MaterializeGeneratedDungeonChest(); !materialized {
			return fmt.Errorf("%w: generated dungeon chest was already materialized", ErrInvalidCampingCombat)
		}
	}
	combat, err := g.prepareCampingParty(scene, campingState, plan.combatMap)
	if err != nil {
		return err
	}
	if plan.centerChest != nil && !combat.AddObject(*plan.centerChest) {
		panic("generated dungeon camping chest has no original object slot")
	}
	// REDUX DEVIATION: CAMP-0003: original rest rules reseeds from wall time and rolls an unused
	// camp encounter choices value here. Redux consumes the roll without reseeding.
	// See ref:CAMP-0003, ref:CAMP-0002.
	_ = references.GetCampingAmbusher(g.AsRandomProvider())
	for range 4 {
		_ = g.GetRNG().Intn(8)
	}
	enemyRef, err := g.enemyReferenceForCombat(plan.monster)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCampingCombat, err)
	}
	for slot, selected := range plan.selected {
		if !selected {
			continue
		}
		combat.AddMonsterToQueue(NewCombatMonster(
			plan.monster,
			enemyRef,
			plan.combatMap.MonsterPositions[slot],
			g.AsRandomProvider(),
		))
	}
	if combat.MonsterManager.Count() != plan.monsterCount {
		panic("dungeon camping monster selection and combat population disagree")
	}
	combat.CampingOrigin = &CampingCombatOrigin{
		ReturnLocation: scene.GetOriginalPlayerLocation(), ActivePlayerIndex: activePlayerIndex,
		Ambusher: plan.monster, dungeonCommand: plan.command,
	}

	return g.queueCampingCombat(combat, references.DungeonCombatTerritory)
}

// CompleteDungeonCampingCommand completes only when no pending camp combat owns the token.
func (g *GameState) CompleteDungeonCampingCommand(command DungeonCommandCycle) {
	if g.pendingCampingCombat != nil &&
		g.pendingCampingCombat.combat.CampingOrigin != nil &&
		g.pendingCampingCombat.combat.CampingOrigin.dungeonCommand.id == command.id {
		return
	}
	g.CompleteDungeonCommandCycle(command, DungeonFullEndPlayer)
}
