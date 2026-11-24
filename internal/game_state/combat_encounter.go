package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// ErrUnsupportedOverworldCombatContact indicates that a special or empty
// large-map contact cannot enter tactical combat.
var ErrUnsupportedOverworldCombatContact = errors.New("unsupported overworld combat contact")

// ErrInvalidOverworldEncounter indicates that source enemy data cannot form an encounter.
var ErrInvalidOverworldEncounter = errors.New("invalid overworld encounter")

// OverworldContactType classifies a large-map enemy interaction before combat setup.
type OverworldContactType int

const (
	// OverworldContactNone means the map unit does not trigger hostile contact.
	OverworldContactNone OverworldContactType = iota
	// OverworldContactLandMonster enters terrain-based tactical combat.
	OverworldContactLandMonster
	// OverworldContactWaterMonster enters a sea or shore tactical map.
	OverworldContactWaterMonster
	// OverworldContactPirateShip enters a ship or shore tactical map.
	OverworldContactPirateShip
	// OverworldContactWhirlpool retains special whirlpool handling.
	OverworldContactWhirlpool
	// OverworldContactSandtrap applies direct damage instead of tactical combat.
	OverworldContactSandtrap
)

// OverworldEncounterOrigin retains the stable source and large-map context
// needed to resolve and restore a normal-play combat encounter.
type OverworldEncounterOrigin struct {
	SourceEnemy       *map_units.NPCEnemy
	ContactType       OverworldContactType
	CombatMap         references.BritanniaCombatMap
	World             references.World
	Position          references.Position
	Floor             references.FloorNumber
	VehicleType       references.VehicleType
	ActivePlayerIndex int
}

// RandomProvider is an adapter to make GameState implement the RandomProvider interface.
type RandomProvider struct {
	g *GameState
}

// Intn implements the RandomProvider interface, returning a value in [0, n).
// Note: This must NOT use RollDice which returns [1, n] (dice semantics).
func (r *RandomProvider) Intn(n int) int {
	return r.g.rng.Intn(n)
}

// AsRandomProvider returns a RandomProvider adapter for GameState.
//
//nolint:ireturn // Interface return is intentional for dependency injection
func (g *GameState) AsRandomProvider() references.RandomProvider {
	return &RandomProvider{g: g}
}

// ClassifyOverworldEnemyContact determines how a visible large-map enemy behaves on contact.
func ClassifyOverworldEnemyContact(enemy *map_units.NPCEnemy) OverworldContactType {
	if enemy == nil || enemy.IsEmptyMapUnit() {
		return OverworldContactNone
	}

	if specialContact, classified := classifySpecialOverworldContact(enemy); classified {
		return specialContact
	}
	if isPiratePredicate()(enemy) {
		return OverworldContactPirateShip
	}
	if enemy.EnemyReference.IsWaterEnemy || enemy.EnemyReference.MonsterType.IsWaterMonster() {
		return OverworldContactWaterMonster
	}

	return OverworldContactLandMonster
}

func classifySpecialOverworldContact(enemy *map_units.NPCEnemy) (OverworldContactType, bool) {
	monsterType := enemy.EnemyReference.MonsterType
	if monsterType == references.MonsterWhirlpool || isWhirlpoolPredicate()(enemy) {
		return OverworldContactWhirlpool, true
	}
	if monsterType == references.MonsterSandtrap || enemy.EnemyReference.IsSandEnemy {
		return OverworldContactSandtrap, true
	}
	if monsterType.IsEnvironmentalHazard() || !enemy.EnemyReference.ActivelyAttacks {
		return OverworldContactNone, true
	}

	return OverworldContactNone, false
}

// SelectOverworldCombatMap reproduces encounter selection map selection.
// See ref:CBT-0003.
func SelectOverworldCombatMap(
	contactType OverworldContactType,
	vehicleType references.VehicleType,
	enemyTerrain *references.Tile,
) (references.BritanniaCombatMap, error) {
	isFrigate := vehicleType == references.FrigateVehicle

	switch contactType {
	case OverworldContactPirateShip:
		if isFrigate {
			return references.ShipShipCombatMap, nil
		}

		return references.LandShipCombatMap, nil
	case OverworldContactWaterMonster:
		if isFrigate {
			return references.ShipSeaCombatMap, nil
		}

		return references.LandSeaCombatMap, nil
	case OverworldContactLandMonster:
		if isOverworldWaterTerrain(enemyTerrain) {
			if isFrigate {
				return references.ShipSeaCombatMap, nil
			}

			return references.LandSeaCombatMap, nil
		}
		if isFrigate {
			return references.ShipShoreCombatMap, nil
		}

		return selectOverworldTerrainCombatMap(enemyTerrain), nil
	case OverworldContactNone, OverworldContactWhirlpool, OverworldContactSandtrap:
		return 0, ErrUnsupportedOverworldCombatContact
	}

	return 0, ErrUnsupportedOverworldCombatContact
}

func selectOverworldTerrainCombatMap(terrain *references.Tile) references.BritanniaCombatMap {
	if terrain == nil {
		return references.GladeCombatMap
	}

	//nolint:exhaustive // Only terrain indexes with specialized combat maps need cases.
	switch terrain.Index {
	case indexes.Swamp:
		return references.SwampCombatMap
	case indexes.Forest1, indexes.Forest2:
		return references.TreedCombatMap
	case indexes.Desert1, indexes.LeftDesert2, indexes.RightDesert2:
		return references.DesertCombatMap
	case indexes.Forest3, indexes.Forest4:
		return references.CleanTreeCombatMap
	case indexes.Hills1, indexes.SmallMountains, indexes.TallMountains,
		indexes.LeftHills2, indexes.RightHills2:
		return references.MountainsCombatMap
	}

	if terrain.IsBridge() {
		return references.BigBridgeCombatMap
	}

	return references.GladeCombatMap
}

func isOverworldWaterTerrain(terrain *references.Tile) bool {
	if terrain == nil || terrain.IsBridge() {
		return false
	}
	if terrain.IsWater() {
		return true
	}

	return terrain.Index >= indexes.WaterStream1 && terrain.Index <= indexes.WaterStream14
}

func (g *GameState) overworldEncounterMonsterCount(
	monsterType references.MonsterType,
	enemyRef *references.EnemyReference,
) (int, error) {
	switch enemyRef.MaxPerMap {
	case references.SoloMonsterCount:
		return references.SoloMonsterCount, nil
	case references.GuardGroupCount:
		return references.GuardGroupCount, nil
	case references.SwarmMonsterCount:
		return references.SwarmMonsterCount, nil
	}

	maximum := min(enemyRef.MaxPerMap, references.MaxMonstersPerMap)
	if maximum < 1 {
		return 0, fmt.Errorf("%w: monster %s has maximum group size %d",
			ErrInvalidOverworldEncounter, monsterType, enemyRef.MaxPerMap)
	}

	count := g.RollDice(maximum)
	if g.hasOpeningCampaignEncounterReduction() {
		count = g.RollDice(count)
	}

	return count, nil
}

func (g *GameState) hasOpeningCampaignEncounterReduction() bool {
	return g.openingCampaignEncounterReduction
}

func (g *GameState) enemyReferenceForCombat(
	monsterType references.MonsterType,
) (*references.EnemyReference, error) {
	if !monsterType.IsValid() || g.GameReferences == nil || g.GameReferences.EnemyReferences == nil {
		return nil, fmt.Errorf("%w: monster type %d", ErrInvalidOverworldEncounter, monsterType)
	}

	enemyReferences := *g.GameReferences.EnemyReferences
	if int(monsterType) >= len(enemyReferences) {
		return nil, fmt.Errorf("%w: monster type %d has no enemy reference",
			ErrInvalidOverworldEncounter, monsterType)
	}

	return &enemyReferences[monsterType], nil
}

// StartOverworldCombatEncounter builds and enters a tactical encounter for a
// visible enemy already owned by the current large-map controller.
func (g *GameState) StartOverworldCombatEncounter(enemy *map_units.NPCEnemy) error {
	if g.CombatScene != nil || g.IsInCombat() {
		return ErrCombatEncounterAlreadyActive
	}
	origin, err := g.buildOverworldEncounterOrigin(enemy)
	if err != nil {
		return err
	}

	combatMap, err := references.LoadCombatMap(
		int(origin.CombatMap), references.BritanniaCombatTerritory, g.GameConfig,
	)
	if err != nil {
		return fmt.Errorf("starting overworld combat: %w", err)
	}
	if err = g.validateOverworldEncounterStaticData(enemy, combatMap); err != nil {
		return fmt.Errorf("validating overworld combat: %w", err)
	}
	primary := enemy.EnemyReference.MonsterType
	if origin.ContactType == OverworldContactPirateShip {
		primary = references.MonsterBard
	}
	primaryRef, err := g.enemyReferenceForCombat(primary)
	if err != nil {
		panic("prevalidated overworld primary reference became invalid")
	}
	combat := NewCombatState(combatMap, CombatEntryNorth, &g.PartyState)
	g.configureCombatTurnQueue(combat)
	g.initializeCombatPartyRings(combat)
	count, err := g.overworldEncounterMonsterCount(primary, primaryRef)
	if err != nil {
		panic("prevalidated overworld group size became invalid")
	}
	// Natural encounters retain every original CBT slot, including coincident
	// positions. Share createentry ordering with the general composer without
	// its developer-selected capacity filtering or a second set of friend rolls.
	g.populatePreparedCombatEncounter(CombatEncounterRequest{
		MonsterType: primary, FriendsEnabled: true,
	}, &preparedCombatEncounter{
		primaryRef: primaryRef, friendType: primaryRef.GetFriendType(),
		combatState: combat, positions: combatMap.MonsterPositions[:],
	}, count)
	combat.OverworldOrigin = origin
	g.commitCombatTransition(combat, references.BritanniaCombatTerritory)
	if phase := g.outdoorActorContinuation; phase != nil && phase.source == enemy {
		phase.encounterID = combat.EncounterID
	}

	g.SystemCallbacks.Message.AddRowStr(enemy.EnemyReference.NamePlural)
	g.SystemCallbacks.Message.AddRowStr("Attacked!")
	logging.Infof(logging.CategoryGameState,
		"Started overworld combat: source=%s map=%d monsters=%d world=%d position=%v",
		enemy.EnemyReference.Name, origin.CombatMap, count, origin.World, origin.Position)

	return nil
}

func (g *GameState) validateOverworldEncounterStaticData(
	enemy *map_units.NPCEnemy,
	combatMap *references.CombatMapReference,
) error {
	primaryMonster := enemy.EnemyReference.MonsterType
	if ClassifyOverworldEnemyContact(enemy) == OverworldContactPirateShip {
		primaryMonster = references.MonsterBard
	}
	enemyRef, err := g.enemyReferenceForCombat(primaryMonster)
	if err != nil {
		return err
	}
	if enemyRef.Friend != nil {
		if _, err = g.enemyReferenceForCombat(enemyRef.Friend.MonsterType); err != nil {
			return err
		}
	}
	if enemyRef.MaxPerMap != references.SoloMonsterCount &&
		enemyRef.MaxPerMap != references.GuardGroupCount &&
		enemyRef.MaxPerMap != references.SwarmMonsterCount &&
		min(enemyRef.MaxPerMap, references.MaxMonstersPerMap) < 1 {
		return fmt.Errorf("%w: monster %s has maximum group size %d",
			ErrInvalidOverworldEncounter, primaryMonster, enemyRef.MaxPerMap)
	}

	for monsterIndex, position := range combatMap.MonsterPositions {
		if position.X < 0 || position.X >= references.XCombatMapTiles ||
			position.Y < 0 || position.Y >= references.YCombatMapTiles {
			return fmt.Errorf("%w: monster position %d is outside combat map: %v",
				ErrInvalidOverworldEncounter, monsterIndex, position)
		}
	}

	return nil
}

func (g *GameState) buildOverworldEncounterOrigin(enemy *map_units.NPCEnemy) (*OverworldEncounterOrigin, error) {
	if enemy == nil {
		return nil, fmt.Errorf("%w: source enemy is nil", ErrInvalidOverworldEncounter)
	}
	if g.MapState.PlayerLocation.Location.GetMapType() != references.LargeMapType {
		return nil, fmt.Errorf("%w: player is not on a large map", ErrInvalidOverworldEncounter)
	}

	mapUnits := g.GetCurrentLargeMapNPCAIController().GetNpcs()
	visibleEnemy := mapUnits.GetVisibleEnemyAtPositionAndFloorOrNil(enemy.Pos(), enemy.Floor())
	if visibleEnemy == nil || visibleEnemy != enemy || enemy.Floor() != g.MapState.PlayerLocation.Floor {
		return nil, fmt.Errorf("%w: source enemy is not active on the current map",
			ErrInvalidOverworldEncounter)
	}

	contactType := ClassifyOverworldEnemyContact(enemy)
	terrain := g.GetCurrentLayeredMap().GetTileByLayer(map_state.MapLayer, enemy.PosPtr())
	vehicleType := g.PartyVehicle.GetVehicleDetails().VehicleType
	combatMap, err := SelectOverworldCombatMap(contactType, vehicleType, terrain)
	if err != nil {
		return nil, err
	}

	return &OverworldEncounterOrigin{
		SourceEnemy:       enemy,
		ContactType:       contactType,
		CombatMap:         combatMap,
		World:             references.World(g.MapState.PlayerLocation.Floor),
		Position:          enemy.Pos(),
		Floor:             enemy.Floor(),
		VehicleType:       vehicleType,
		ActivePlayerIndex: g.GetActivePlayerIndex(),
	}, nil
}

func isResolvableOverworldContact(contactType OverworldContactType) bool {
	return contactType == OverworldContactLandMonster ||
		contactType == OverworldContactWaterMonster ||
		contactType == OverworldContactPirateShip ||
		contactType == OverworldContactSandtrap
}

// QueueOverworldEncounterAt records a tactical enemy occupying the target tile.
// The pending encounter starts after the current large-map turn cost is applied.
func (g *GameState) QueueOverworldEncounterAt(position references.Position) bool {
	if g.MapState.PlayerLocation.Location.GetMapType() != references.LargeMapType {
		return false
	}
	if !positionsAreOrthogonallyAdjacentWrapped(g.MapState.PlayerLocation.Position, position) {
		return false
	}

	enemy := g.GetCurrentLargeMapNPCAIController().GetNpcs().
		GetVisibleEnemyAtPositionAndFloorOrNil(position, g.MapState.PlayerLocation.Floor)
	contactType := ClassifyOverworldEnemyContact(enemy)
	if contactType != OverworldContactLandMonster &&
		contactType != OverworldContactWaterMonster &&
		contactType != OverworldContactPirateShip &&
		contactType != OverworldContactSandtrap {
		return false
	}

	if g.usesDirectDamageForOverworldContact(contactType) {
		g.pendingDirectMonsterContact = contactType
	} else {
		g.pendingOverworldEncounter = enemy
	}

	return true
}

func (g *GameState) startPendingOverworldEncounter() error {
	if g.pendingOverworldEncounter == nil {
		return nil
	}

	enemy := g.pendingOverworldEncounter
	g.pendingOverworldEncounter = nil
	if enemy.Floor() != g.MapState.PlayerLocation.Floor ||
		!positionsAreOrthogonallyAdjacentWrapped(g.MapState.PlayerLocation.Position, enemy.Pos()) {
		g.discardOutdoorActorContinuation()
		g.activeSlowMovement = nil
		g.abortShipRepairAfterUnresolvedCombat()

		return nil
	}

	err := g.StartOverworldCombatEncounter(enemy)
	if err != nil {
		g.discardOutdoorActorContinuation()
		g.activeSlowMovement = nil
		g.abortShipRepairAfterUnresolvedCombat()
	}

	return err
}

// HasPendingOverworldContact reports whether deferred direct damage, ordinary
// overworld combat, or a source-less special encounter should resolve after
// active gameplay workflows complete.
func (g *GameState) HasPendingOverworldContact() bool {
	return g.pendingDirectMonsterContact != OverworldContactNone ||
		g.pendingOverworldEncounter != nil || g.pendingCombatEncounter != nil ||
		g.pendingSmallMapEncounter != nil ||
		g.pendingCampingCombat != nil ||
		(g.outdoorActorContinuation != nil && g.CombatScene == nil) ||
		(g.activeSlowMovement != nil && g.CombatScene == nil)
}

// ShouldDeferOverworldContact respects interaction ownership, not background
// animation. In particular, Time Stop can freeze moongate regrowth indefinitely
// until ordinary player updates are allowed to expire the spell.
func (g *GameState) ShouldDeferOverworldContact() bool {
	return g.EventQueueManager.HasInputBlockingWorkflows() ||
		g.EventQueueManager.HasWorkflow("automatic_sailing")
}

// ResolvePendingOverworldContact applies deferred direct damage or enters combat.
func (g *GameState) ResolvePendingOverworldContact() error {
	if g.CombatScene != nil || g.ShouldDeferOverworldContact() {
		return nil
	}
	if g.activeSlowMovement != nil {
		return g.resumeSlowMovement()
	}

	return g.resolvePendingOverworldContact()
}

func (g *GameState) resolvePendingOverworldContact() error {
	if g.PartyState.IsAllPartyDead() {
		g.pendingDirectMonsterContact = OverworldContactNone
		g.pendingOverworldEncounter = nil
		g.pendingCombatEncounter = nil
		g.pendingSmallMapEncounter = nil
		g.pendingCampingCombat = nil
		g.cancelOutdoorActorPhase()

		return nil
	}
	g.resumeOutdoorActorPhase()
	if g.ShouldDeferOverworldContact() {
		return nil
	}
	if g.pendingCampingCombat != nil {
		return g.resolvePendingCampingCombat()
	}
	if g.pendingSmallMapEncounter != nil {
		origin := g.pendingSmallMapEncounter
		g.pendingSmallMapEncounter = nil

		_, err := g.startSmallMapCombatEncounter(origin)

		return err
	}
	if g.processPendingDirectMonsterDamage() {
		return nil
	}
	if g.pendingCombatEncounter != nil {
		request := *g.pendingCombatEncounter
		g.pendingCombatEncounter = nil
		_, err := g.StartCombatEncounter(request)

		return err
	}

	return g.startPendingOverworldEncounter()
}

func (g *GameState) usesDirectDamageForOverworldContact(contactType OverworldContactType) bool {
	if contactType == OverworldContactSandtrap {
		return true
	}

	vehicleType := g.PartyVehicle.GetVehicleDetails().VehicleType
	if vehicleType != references.SkiffVehicle && vehicleType != references.CarpetVehicle {
		return false
	}

	terrain := g.GetCurrentLayeredMap().GetTileByLayer(
		map_state.MapLayer,
		&g.MapState.PlayerLocation.Position,
	)

	return terrain != nil && terrain.Index < indexes.Swamp
}

func (g *GameState) processPendingDirectMonsterDamage() bool {
	if g.pendingDirectMonsterContact == OverworldContactNone {
		return false
	}
	contactType := g.pendingDirectMonsterContact
	g.pendingDirectMonsterContact = OverworldContactNone

	if contactType != OverworldContactSandtrap {
		g.SystemCallbacks.Message.AddRowStr("Attacked!")
	}
	hit := g.ResolveSourceHit()
	g.presentSourceHitOutcome(hit)
	for _, damage := range hit.MemberDamage {
		if damage.Died {
			character := g.PartyState.GetCharacter(damage.MemberIndex)
			g.SystemCallbacks.Message.AddRowStr("Alas! " + character.GetNameAsString() + " has died!")
		}
	}
	if hit.PartyWiped {
		g.checkForPartyWipe()
	}

	return true
}

func positionsAreOrthogonallyAdjacentWrapped(first, second references.Position) bool {
	xDistance := wrappedCoordinateDistance(first.X, second.X, references.XLargeMapTiles)
	yDistance := wrappedCoordinateDistance(first.Y, second.Y, references.YLargeMapTiles)

	return xDistance+yDistance == 1
}

func wrappedCoordinateDistance(
	first references.Coordinate,
	second references.Coordinate,
	mapSize references.Coordinate,
) int {
	distance := int(first) - int(second)
	if distance < 0 {
		distance = -distance
	}

	return min(distance, int(mapSize)-distance)
}

func (g *GameState) resolveOverworldEncounterOrigin(
	origin *OverworldEncounterOrigin,
	reason CombatEndReason,
) {
	controller := g.LargeMapNPCAIController[origin.World]
	if controller == nil {
		panic("overworld encounter origin has no large-map controller")
	}

	mapUnits := controller.GetNpcs()
	if !mapUnits.RemoveEnemy(origin.SourceEnemy) {
		panic("overworld encounter source enemy was already removed")
	}

	if reason == CombatVictory && origin.ContactType == OverworldContactPirateShip {
		direction := pirateDirection(origin.SourceEnemy)
		frigate := map_units.NewNPCFriendlyVehiceNewRef(
			references.FrigateVehicle,
			origin.Position,
			origin.Floor,
		)
		if !mapUnits.AddVehicle(*frigate) {
			panic("unable to add salvaged pirate frigate")
		}
		salvagedFrigate := mapUnits.GetVehicleAtPositionAndFloorOrNil(origin.Position, origin.Floor)
		if salvagedFrigate == nil {
			panic("salvaged pirate frigate was not added at the encounter position")
		}
		salvagedFrigate.SetDirection(direction)
	}

	g.restorePreCombatActivePlayer(origin.ActivePlayerIndex)
	controller.FreshenExistingNPCsOnMap()
}

func (g *GameState) restorePreCombatActivePlayer(activePlayerIndex int) {
	if activePlayerIndex < 0 {
		g.ClearActivePlayer()

		return
	}

	character := g.PartyState.GetCharacter(activePlayerIndex)
	if character.IsDead() || character.IsAsleep() {
		g.ClearActivePlayer()

		return
	}

	if !g.SetActivePlayer(activePlayerIndex) {
		panic("unable to restore pre-combat active player")
	}
}

func pirateDirection(enemy *map_units.NPCEnemy) references.Direction {
	if enemy == nil || enemy.Variant() != map_units.EnemyVariantPirate {
		return references.Right
	}

	return enemy.Facing()
}
