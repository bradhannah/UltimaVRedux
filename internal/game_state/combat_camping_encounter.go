package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/camping"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// ErrInvalidCampingCombat indicates that a camp ambush cannot form a complete encounter.
var ErrInvalidCampingCombat = errors.New("invalid camping combat")

// CampingAmbushResult reports the encounter selected and prepared by one ambush check.
type CampingAmbushResult struct {
	MonsterType  references.MonsterType
	PrimaryCount int
	FriendCount  int
}

// CampingCombatOrigin retains the exploration context owned by a camping encounter.
type CampingCombatOrigin struct {
	ReturnLocation       references.PlayerLocation
	ActivePlayerIndex    int
	Ambusher             references.MonsterType
	completeSurfaceCycle bool
	dungeonCommand       DungeonCommandCycle
}

type pendingCampingCombat struct {
	combat    *CombatState
	territory references.CombatMapTerritory
}

// QueueOutdoorCampingAmbush prepares one source-less camp encounter while the
// camping workflow still owns input. The depth-zero encounter resolver commits it
// only after that workflow has restored exploration state and released ownership.
func (g *GameState) QueueOutdoorCampingAmbush(
	campingScene *camping.CampingScene,
	campingState *CampingState,
	activePlayerIndex int,
) (CampingAmbushResult, error) {
	if campingScene == nil || g.CampingScene != campingScene || !g.IsInCamping() {
		return CampingAmbushResult{}, fmt.Errorf("%w: outdoor camping scene is not active", ErrInvalidCampingCombat)
	}
	if g.pendingCampingCombat != nil {
		return CampingAmbushResult{}, fmt.Errorf("%w: camping combat is already pending", ErrInvalidCampingCombat)
	}
	if g.CombatScene != nil {
		return CampingAmbushResult{}, ErrCombatEncounterAlreadyActive
	}
	if campingState == nil || !campingState.specialRingsInitialized {
		return CampingAmbushResult{}, fmt.Errorf("%w: camping party rings are not initialized", ErrInvalidCampingCombat)
	}
	if activePlayerIndex < -1 || activePlayerIndex >= party_state.MaxActiveParty {
		return CampingAmbushResult{}, fmt.Errorf(
			"%w: invalid active player index %d",
			ErrInvalidCampingCombat,
			activePlayerIndex,
		)
	}

	combatMap, err := references.LoadCampingMap(g.GameConfig)
	if err != nil {
		return CampingAmbushResult{}, fmt.Errorf("%w: %w", ErrInvalidCampingCombat, err)
	}
	if err = g.validateCampingAmbushReferences(); err != nil {
		return CampingAmbushResult{}, err
	}
	if g.isValidCampingGuard(campingScene.GuardIndex) {
		g.setActiveCampingPartySleepStatus(false)
	}

	combat, err := g.prepareCampingParty(campingScene, campingState, combatMap)
	if err != nil {
		return CampingAmbushResult{}, err
	}
	// REDUX DEVIATION: original rest rules reseeds from wall-time seeding before selecting gollums.
	// Redux retains the centralized GameState RNG so gameplay remains reproducible.
	// See ref:CAMP-0002.
	ambusher := references.GetCampingAmbusher(g.AsRandomProvider())
	result, err := g.populateOutdoorCampingCombat(combat, ambusher)
	if err != nil {
		return CampingAmbushResult{}, err
	}
	combat.CampingOrigin = &CampingCombatOrigin{
		ReturnLocation:    campingScene.GetOriginalPlayerLocation(),
		ActivePlayerIndex: activePlayerIndex,
		Ambusher:          ambusher,
	}
	if err = g.queueCampingCombat(combat, references.BritanniaCombatTerritory); err != nil {
		return CampingAmbushResult{}, err
	}

	return result, nil
}

func (g *GameState) validateCampingAmbushReferences() error {
	for _, monsterType := range references.CampingAmbushMonsters {
		enemyRef, err := g.enemyReferenceForCombat(monsterType)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidCampingCombat, err)
		}
		if enemyRef.Friend != nil {
			if _, err = g.enemyReferenceForCombat(enemyRef.Friend.MonsterType); err != nil {
				return fmt.Errorf("%w: friend: %w", ErrInvalidCampingCombat, err)
			}
		}
	}

	return nil
}

func (g *GameState) prepareCampingParty(
	campingScene *camping.CampingScene,
	campingState *CampingState,
	combatMap *references.CombatMapReference,
) (*CombatState, error) {
	if campingState == nil || !campingState.specialRingsInitialized {
		return nil, fmt.Errorf("%w: camping party rings are not initialized", ErrInvalidCampingCombat)
	}
	combat := NewCombatState(combatMap, CombatEntryNorth, &g.PartyState)
	g.configureCombatTurnQueue(combat)
	occupied := make(map[references.Position]struct{}, party_state.MaxActiveParty)
	for partyIndex, active := range combat.PartyActive {
		if !active {
			continue
		}
		position := campingScene.GetPartyMemberPosition(partyIndex)
		if !isCombatMapPosition(position) {
			return nil, fmt.Errorf(
				"%w: party position %d is outside combat map: %v",
				ErrInvalidCampingCombat,
				partyIndex,
				position,
			)
		}
		if _, exists := occupied[position]; exists {
			return nil, fmt.Errorf(
				"%w: duplicate party position: %v",
				ErrInvalidCampingCombat,
				position,
			)
		}
		occupied[position] = struct{}{}
		combat.PartyPositions[partyIndex] = position
	}
	for positionIndex, position := range combatMap.MonsterPositions {
		if !isCombatMapPosition(position) {
			return nil, fmt.Errorf(
				"%w: monster position %d is outside combat map: %v",
				ErrInvalidCampingCombat,
				positionIndex,
				position,
			)
		}
	}
	inheritCampingPartyRings(combat, campingState)

	return combat, nil
}

func (g *GameState) populateOutdoorCampingCombat(
	combat *CombatState,
	ambusher references.MonsterType,
) (CampingAmbushResult, error) {
	enemyRef, err := g.enemyReferenceForCombat(ambusher)
	if err != nil {
		return CampingAmbushResult{}, fmt.Errorf("%w: %w", ErrInvalidCampingCombat, err)
	}

	positions := combat.CombatMapRef.MonsterPositions
	for positionIndex := range len(positions) - 1 {
		randomIndex := g.GetRNG().Intn(len(positions))
		positions[positionIndex], positions[randomIndex] = positions[randomIndex], positions[positionIndex]
	}
	// A guard who wandered onto a spawn slot keeps it; ambushers use the rest.
	free := make([]references.Position, 0, len(positions))
	for _, position := range positions {
		if !campingPartyOccupies(combat, position) {
			free = append(free, position)
		}
	}
	count := min(g.resolveCampingAmbushCount(enemyRef.MaxPerMap), len(free))
	result := CampingAmbushResult{MonsterType: ambusher}
	for monsterIndex := range count {
		monsterType := ambusher
		if monsterIndex > 0 && enemyRef.Friend != nil &&
			references.ShouldUseFriend(monsterIndex, count, g.AsRandomProvider()) {
			monsterType = enemyRef.Friend.MonsterType
		}
		position := free[monsterIndex]

		monsterRef := enemyRef
		if monsterType != ambusher {
			monsterRef, err = g.enemyReferenceForCombat(monsterType)
			if err != nil {
				panic("prevalidated camping friend reference became invalid")
			}
			result.FriendCount++
		} else {
			result.PrimaryCount++
		}
		combat.AddMonsterToQueue(NewCombatMonster(
			monsterType,
			monsterRef,
			position,
			g.AsRandomProvider(),
		))
	}
	_, queuedMonsters := combat.TurnQueue.CountRemaining()
	if queuedMonsters != combat.MonsterManager.Count() {
		panic("camping monster manager and turn queue are out of sync")
	}

	return result, nil
}

func (g *GameState) resolveCampingAmbushCount(maximum int) int {
	maximum = min(maximum, references.MaxMonstersInCombat)
	if maximum < 1 {
		panic("prevalidated camping monster has no valid group size")
	}
	if maximum == 1 || maximum == 8 || maximum == references.MaxMonstersInCombat {
		return maximum
	}

	count := g.RollDice(maximum)
	if g.hasOpeningCampaignEncounterReduction() {
		count = g.RollDice(count)
	}

	return count
}

func (g *GameState) queueCampingCombat(
	combat *CombatState,
	territory references.CombatMapTerritory,
) error {
	if combat == nil || combat.CampingOrigin == nil {
		return fmt.Errorf("%w: combat state and origin are required", ErrInvalidCampingCombat)
	}
	if combat.OverworldOrigin != nil || combat.DungeonRoomOrigin != nil || combat.DungeonWandererOrigin != nil {
		return fmt.Errorf("%w: combat state has another encounter owner", ErrInvalidCampingCombat)
	}
	if territory != references.BritanniaCombatTerritory && territory != references.DungeonCombatTerritory {
		return fmt.Errorf("%w: invalid combat territory %d", ErrInvalidCampingCombat, territory)
	}
	if g.pendingCampingCombat != nil {
		return fmt.Errorf("%w: camping combat is already pending", ErrInvalidCampingCombat)
	}
	if g.CombatScene != nil {
		return ErrCombatEncounterAlreadyActive
	}

	g.pendingCampingCombat = &pendingCampingCombat{combat: combat, territory: territory}

	return nil
}

func (g *GameState) resolvePendingCampingCombat() error {
	if g.pendingCampingCombat == nil {
		return nil
	}
	if err := g.validateCombatEncounterStartState(); err != nil {
		return err
	}
	pending := g.pendingCampingCombat
	origin := pending.combat.CampingOrigin
	if origin == nil || g.MapState.PlayerLocation != origin.ReturnLocation {
		return fmt.Errorf("%w: camping workflow did not restore its return location", ErrInvalidCampingCombat)
	}

	g.pendingCampingCombat = nil
	// REDUX DEVIATION: original encounter selection enters camp combat synchronously. Redux commits
	// only after the camping workflow's Finally releases map and input ownership.
	// See ref:CAMP-0002.
	g.commitCombatTransition(pending.combat, pending.territory)

	return nil
}

func (g *GameState) resolveCampingCombatOrigin(origin *CampingCombatOrigin) {
	if origin == nil {
		return
	}
	if g.MapState.PlayerLocation != origin.ReturnLocation {
		panic("camping combat returned to an unexpected origin")
	}

	g.restorePreCombatActivePlayer(origin.ActivePlayerIndex)
	if origin.dungeonCommand.id != 0 {
		if g.MapState.PlayerLocation.Location.GetMapType() != references.DungeonMapType {
			panic("dungeon camping combat did not return to a dungeon")
		}
		g.CompleteDungeonCommandCycle(origin.dungeonCommand, DungeonFullEndPlayer)

		return
	}
	if origin.completeSurfaceCycle && !g.PartyState.IsAllPartyDead() {
		g.completeSurfaceCycle(references.LargeMapType, DefaultLargeMapMinutesPerTurn)

		return
	}
	g.checkForPartyWipe()
}

func (g *GameState) deferSurfaceCommandCycleForCampingCombat(originMapType references.GeneralMapType) bool {
	if g.pendingCampingCombat == nil {
		return false
	}
	if originMapType != references.LargeMapType {
		panic("outdoor camping completion requires a large-map origin")
	}
	origin := g.pendingCampingCombat.combat.CampingOrigin
	if origin == nil || origin.dungeonCommand.id != 0 {
		return false
	}
	origin.completeSurfaceCycle = true

	return true
}

func campingPartyOccupies(combat *CombatState, position references.Position) bool {
	for partyIndex, active := range combat.PartyActive {
		if active && combat.PartyPositions[partyIndex] == position {
			return true
		}
	}

	return false
}
