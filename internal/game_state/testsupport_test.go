package game_state

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/ai"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/test/helpers"
)

// ValidateAndConsumePartyTargetSpellResources commits a prepared character
// spell after final target confirmation, with the same validation as other routes.
func (g *GameState) ValidateAndConsumePartyTargetSpellResources(request CastRequest) SpellValidationResult {
	profile := references.GetSpellTargetingProfile(request.Spell, g.GetCurrentSpellContext())
	if profile.Selection != references.TargetPartyMember {
		panic("party-target resource commit called for another targeting mode")
	}

	return g.ValidateAndConsumeSpellResources(request)
}

// ActionGetCombatMap delegates collection and reports whether combat handled the request.
func (g *GameState) ActionGetCombatMap(direction references.Direction) bool {
	return g.GetCombatItem(direction).Handled
}

// ProcessBedRestProvisionHour consumes a meal or applies one starvation consequence.
func (g *GameState) ProcessBedRestProvisionHour(hour int) BedRestProvisionResult {
	return g.processBedRestProvisionHour(hour, g.cachePlayerUpdateStatuses().Diners)
}

// ActionJimmySmallMap attempts a town unlock with the eligible selected character.
func (g *GameState) ActionJimmySmallMap(direction references.Direction) bool {
	if !g.PartyState.Inventory.Provisions.Keys.HasSome() {
		return g.actionJimmySurface(direction, nil)
	}
	selectedCharacter := g.SelectCharacterForJimmy()
	if selectedCharacter == nil {
		g.SystemCallbacks.Message.AddRowStr("No characters available!")

		return false
	}

	return g.ActionJimmySmallMapWithCharacter(direction, selectedCharacter)
}

// ActionJimmyLargeMap attempts an outdoor unlock with the eligible selected character.
func (g *GameState) ActionJimmyLargeMap(direction references.Direction) bool {
	if !g.PartyState.Inventory.Provisions.Keys.HasSome() {
		return g.actionJimmySurface(direction, nil)
	}
	selectedCharacter := g.SelectCharacterForJimmy()
	if selectedCharacter == nil {
		g.SystemCallbacks.Message.AddRowStr("No characters available!")

		return false
	}

	return g.actionJimmySurface(direction, selectedCharacter)
}

// ActionJimmyCombatMap reports whether the target was handled, including failed attempts.
func (g *GameState) ActionJimmyCombatMap(direction references.Direction) bool {
	handled, _ := g.ActionJimmyCombatMapWithDisarm(direction)

	return handled
}

func (g *GameState) ActionReadySmallMap() bool {
	g.SystemCallbacks.Message.ShowCommandPrompt("Ready...")
	request, outcome := g.BeginReadyEquipment(false)
	if outcome == ReadyBeginDialog {
		g.SystemCallbacks.Flow.Items.ShowReadyDialog(request)
	}

	return outcome != ReadyBeginRejected
}

// CommitSkullKeyUnlock atomically revalidates and unlocks the reviewed door.
func (g *GameState) CommitSkullKeyUnlock(review SkullKeyUnlockReview) SkullKeyUnlockResult {
	return g.commitSkullKeyUnlock(review, true)
}

func (g *GameState) ActionViewSmallMap() bool {
	if !g.ActionViewSurface() {
		return false
	}

	// Trigger the presentation-only gem view workflow.
	g.SystemCallbacks.Flow.Observation.ShowGemViewInteraction(nil)

	return true
}

func (g *GameState) ActionViewLargeMap() bool {
	if !g.ActionViewSurface() {
		return false
	}

	// Trigger the presentation-only gem view workflow.
	g.SystemCallbacks.Flow.Observation.ShowGemViewInteraction(nil)

	return true
}

// GetAllShadowlordPositions returns a map of town locations to their assigned Shadowlords.
// Only includes towns that have a living Shadowlord assigned.
func (g *GameState) GetAllShadowlordPositions() map[references.Location]references.Shadowlord {
	positions := make(map[references.Location]references.Shadowlord)

	for loc := ShadowlordTownMinLocation; loc <= ShadowlordTownMaxLocation; loc++ {
		lord := g.GetShadowlordForLocation(references.Location(loc))
		if lord != references.NoShadowlord {
			positions[references.Location(loc)] = lord
		}
	}

	return positions
}

// GetTotalSearchLocations returns the total number of search locations in the system
func (aslm *AuthenticSearchLocationManager) GetTotalSearchLocations() int {
	return len(references.GetAllSearchLocations())
}

// GetFoundLocationsByMap returns a map of found locations grouped by map location
func (aslm *AuthenticSearchLocationManager) GetFoundLocationsByMap() map[references.Location][]int {
	result := make(map[references.Location][]int)
	allLocations := references.GetAllSearchLocations()

	for _, loc := range allLocations {
		if aslm.isAlreadyFound(loc.Index) {
			mapLoc := references.Location(loc.MapNumber)
			result[mapLoc] = append(result[mapLoc], loc.Index)
		}
	}

	return result
}

// CastCombatInFlamHur applies the original flame-wind cone. Each occupied tile
// takes direct random damage from 0 through 30 without an armor reduction.
// original spell projection rules and original command rules establish the inclusive source contract;
// the external RNG implementation is unavailable for binary confirmation.
// See ref:SPELL-0003.
func (g *GameState) CastCombatInFlamHur(casterIdx int, direction references.Direction) []references.Position {
	return g.castCommittedCombatCone(references.InFlamHur, casterIdx, direction)
}

// CastCombatInVasGravCorp applies the original energy-vortex cone. Targets
// first receive an intelligence save; spell-immune and immortal monsters
// cannot be killed. A failed save deals the original instant-death value.
func (g *GameState) CastCombatInVasGravCorp(casterIdx int, direction references.Direction) []references.Position {
	return g.castCommittedCombatCone(references.InVasGravCorp, casterIdx, direction)
}

// CastCombatInVasPorYlem applies the original earthquake to hostile combatants
// whose DEX does not exceed their individual D30 roll.
func (g *GameState) CastCombatInVasPorYlem(casterIdx int) []references.Position {
	return g.CastCombatEarthquakeResult(casterIdx).processedPositions()
}

// TurnDelay returns the turn delay (36 - DEX for initiative order).
func (m *CombatMonster) TurnDelay() int { return m.turnDelay }

// SetHasActed marks whether the monster has acted this round.
func (m *CombatMonster) SetHasActed(acted bool) {
	m.hasActed = acted
}

// GetActiveHostileMonsters returns active monsters loyal to the opposing side.
func (m *CombatMonsterManager) GetActiveHostileMonsters() []*CombatMonster {
	hostile := make([]*CombatMonster, 0)
	for _, monster := range m.monsters {
		if monster.IsActive() && !monster.IsDead() && monster.IsHostile() {
			hostile = append(hostile, monster)
		}
	}

	return hostile
}

// GetActivePartyLoyalMonsters returns active monsters that support the party.
func (m *CombatMonsterManager) GetActivePartyLoyalMonsters() []*CombatMonster {
	partyLoyal := make([]*CombatMonster, 0)
	for _, monster := range m.monsters {
		if monster.IsActive() && !monster.IsDead() && monster.IsPartyLoyal() {
			partyLoyal = append(partyLoyal, monster)
		}
	}

	return partyLoyal
}

// CountActivePartyLoyal returns the number of active monsters that support the party.
func (m *CombatMonsterManager) CountActivePartyLoyal() int {
	count := 0
	for _, monster := range m.monsters {
		if monster.IsActive() && !monster.IsDead() && monster.IsPartyLoyal() {
			count++
		}
	}

	return count
}

// GetMonstersAtPosition returns every active monster at pos in stable manager order.
func (m *CombatMonsterManager) GetMonstersAtPosition(pos references.Position) []*CombatMonster {
	monsters := make([]*CombatMonster, 0)
	for _, monster := range m.monsters {
		if monster.IsActive() && !monster.IsDead() && monster.Position() == pos {
			monsters = append(monsters, monster)
		}
	}

	return monsters
}

// GetMonsterIndicesAtPosition returns stable manager indices for every active monster at pos.
func (m *CombatMonsterManager) GetMonsterIndicesAtPosition(pos references.Position) []int {
	indices := make([]int, 0)
	for index, monster := range m.monsters {
		if monster.IsActive() && !monster.IsDead() && monster.Position() == pos {
			indices = append(indices, index)
		}
	}

	return indices
}

// GetEntry returns the queue entry for a specific combatant.
// Returns nil if not found.
// GetEntry returns the normal-slot entry retained for existing PC/monster callers.
func (q *CombatTurnQueue) GetEntry(entityIndex int, isPC bool) *TurnQueueEntry {
	return q.GetEntryByIdentity(CombatTurnIdentity{IsPartyMember: isPC, EntityIndex: entityIndex})
}

// ActiveEntryCount returns the number of entries that are still active.
func (q *CombatTurnQueue) ActiveEntryCount() int {
	count := 0
	for _, entry := range q.entries {
		if entry.status == turnQueueActive {
			count++
		}
	}

	return count
}

// TriggerEarthquakeManually commits earthquake damage without the random eligibility roll.
func (e *EarthquakeSystem) TriggerEarthquakeManually(partyState *party_state.PartyState) EarthquakeResult {
	return e.triggerEarthquake(partyState)
}

// ProcessWindOnTurnAdvancement should be called during turn processing for automatic wind changes
func (g *GameState) ProcessWindOnTurnAdvancement() {
	if g.windSystem == nil {
		// System corruption: wind system should be initialized in GameState constructor
		panic("ProcessWindOnTurnAdvancement: windSystem is nil")
	}

	g.windSystem.UpdateWindTick()
}

// SetPartyMemberInvisible updates encounter-local party invisibility.
func (combatState *CombatState) SetPartyMemberInvisible(memberIndex int, invisible bool) {
	if memberIndex < 0 || memberIndex >= party_state.MaxActiveParty {
		panic("combat party invisibility index out of range")
	}
	combatState.PartyInvisible[memberIndex] = invisible
}

// QuoteHorsePurchase returns the current canonical price and delivery fingerprint.
func (g *GameState) QuoteHorsePurchase(session MerchantSession) (HorsePurchaseQuote, error) {
	state, err := g.GetHorseShopState(session)
	if err != nil {
		return HorsePurchaseQuote{}, err
	}

	return state.Quote, nil
}

// QuoteInnPickup returns one current stored-guest bill and projection.
func (g *GameState) QuoteInnPickup(
	session MerchantSession,
	character InnCharacterIdentity,
) (InnGuestRow, error) {
	state, err := g.GetInnShopState(session)
	if err != nil {
		return InnGuestRow{}, err
	}
	for _, row := range state.GuestRows {
		if row.Identity == character {
			return row, nil
		}
	}

	return InnGuestRow{}, fmt.Errorf("%w: guest %q is unavailable", ErrInvalidInnRequest, character.Name)
}

// MerchantID returns the merchant's stable canonical identity.
func (s MerchantSession) MerchantID() references.MerchantID { return s.merchant.ID }

// PricingCharacterName returns the captured pricing character's stable display identity.
func (s MerchantSession) PricingCharacterName() string {
	return s.buyerName
}

// BeginMerchantSessionByID resolves a stable merchant identity to the exact
// loaded NPC, then delegates all production validation to BeginMerchantSession.
func (g *GameState) BeginMerchantSessionByID(id references.MerchantID) (MerchantSession, error) {
	_, npc, err := g.resolveLoadedMerchantByID(id)
	if err != nil {
		return MerchantSession{}, err
	}

	return g.BeginMerchantSession(npc)
}

// PrepareMoongateTravel checks moongate status and prepares for travel.
// Returns a captured travel route. Does NOT start any event queues.
// The caller is responsible for orchestrating the visual sequence.
func (g *GameState) PrepareMoongateTravel() MoongateTravelResult {
	result := g.ReviewMoongateTravel()
	if result.Success || result.FailedButActivated {
		g.ApplyMoongateTravelEffects(result.Success)
	}

	return result
}

// GetMoonstoneIndexForMoonPhase returns the moonstone index for current moon phase - delegation to MoonstoneSystem
func (g *GameState) GetMoonstoneIndexForMoonPhase(phase int) int {
	if g.moonstoneSystem == nil {
		g.moonstoneSystem = NewMoonstoneSystem()
	}

	return g.moonstoneSystem.GetMoonstoneIndexForMoonPhase(phase)
}

// GetMoongateVisibilityLevel converts the authoritative stage to 0.0 through 1.0.
func (g *GameState) GetMoongateVisibilityLevel() float32 {
	return float32(g.moongateStage) / float32(MoongateStageFull)
}

// HasActiveMoongateAtPosition checks stage and placed moonstone source state.
func (g *GameState) HasActiveMoongateAtPosition(pos *references.Position) bool {
	if g.moonstoneSystem == nil {
		g.moonstoneSystem = NewMoonstoneSystem()
	}

	location := g.MapState.PlayerLocation

	return g.moongateStage > MoongateStageHidden &&
		g.moonstoneSystem.CanCreateMoongateAt(location.Location, location.Floor, *pos)
}

// FirePirateCannon fires a cannon from pirate ship to player position
// See ref:SHIP-0005.
func (g *GameState) FirePirateCannon(pirate *map_units.NPCEnemy, targetPos references.Position) {
	result := g.resolvePirateCannon(pirate, targetPos)
	g.pirateAttackQueue = append(g.pirateAttackQueue, pirateAttackRequest{
		result: result,
	})
	g.startPirateAttackWorkflow()
}

// ExecuteSearch performs a search operation with clear priority ordering
// The ORDER of method calls defines the search priority
func (ssm *SearchSystemManager) ExecuteSearch(direction references.Direction) SearchResponse {
	return ssm.ExecuteSearchWithCharacter(direction, ssm.gameState.defaultSurfaceSearchCharacterIndex())
}

// QuoteShipPurchase returns one current canonical vessel quote.
func (g *GameState) QuoteShipPurchase(
	session MerchantSession,
	vehicleType references.VehicleType,
) (ShipPurchaseQuote, error) {
	state, err := g.GetShipShopState(session)
	if err != nil {
		return ShipPurchaseQuote{}, err
	}

	switch vehicleType {
	case references.FrigateVehicle:
		return state.FrigateQuote, nil
	case references.SkiffVehicle:
		return state.SkiffQuote, nil
	case references.NoPartyVehicle, references.CarpetVehicle, references.HorseVehicle, references.NPC:
		return ShipPurchaseQuote{}, fmt.Errorf("%w: unsupported vehicle type %d", ErrInvalidShipPurchase, vehicleType)
	default:
		return ShipPurchaseQuote{}, fmt.Errorf("%w: unsupported vehicle type %d", ErrInvalidShipPurchase, vehicleType)
	}
}

// SkiffDeepWaterDamageSequence lets a workflow own a qualifying hazard inline.
// An owner that must await damage uses ThenSequence, never a child workflow plus
// a wait for its name; that remains safe even for high/critical-priority callers.
// The owner must clear transient visuals in Finally via ClearSkiffDeepWaterEffects.
func (g *GameState) SkiffDeepWaterDamageSequence(position references.Position) event_queue.Sequence {
	return func(queue *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		if !g.isSkiffDeepWaterDamageEligible(&position) {
			return queue
		}

		return queue.ThenSequence(g.skiffDeepWaterDamageSequence(func() {}))
	}
}

// QuoteTavernMeal returns the current trusted meal charge.
func (g *GameState) QuoteTavernMeal(session MerchantSession) (TavernMealQuote, error) {
	state, err := g.GetTavernShopState(session)
	if err != nil {
		return TavernMealQuote{}, err
	}

	return state.Meal, nil
}

// QuoteTavernDrink returns the current ordinary drink quote.
func (g *GameState) QuoteTavernDrink(session MerchantSession) (TavernDrinkQuote, error) {
	state, err := g.GetTavernShopState(session)
	if err != nil {
		return TavernDrinkQuote{}, err
	}
	if state.Menu.Drink.IsWineList {
		return TavernDrinkQuote{}, fmt.Errorf("%w: merchant %q offers a wine list", ErrInvalidTavernRequest, state.Merchant.ID)
	}

	return state.Drink, nil
}

// QuoteTavernWine returns one fixed-price canonical wine quote.
func (g *GameState) QuoteTavernWine(session MerchantSession, wineIndex int) (TavernDrinkQuote, error) {
	state, err := g.GetTavernShopState(session)
	if err != nil {
		return TavernDrinkQuote{}, err
	}
	if !state.Menu.Drink.IsWineList || wineIndex < 0 || wineIndex >= len(state.Wines) {
		return TavernDrinkQuote{}, fmt.Errorf("%w: wine index %d is unavailable", ErrInvalidTavernRequest, wineIndex)
	}

	return state.Wines[wineIndex], nil
}

// QuoteTavernProvisions returns the active menu's canonical per-pack quote.
func (g *GameState) QuoteTavernProvisions(session MerchantSession) (TavernProvisionQuote, error) {
	state, err := g.GetTavernShopState(session)
	if err != nil {
		return TavernProvisionQuote{}, err
	}
	if !state.Menu.Provision.Available {
		return TavernProvisionQuote{}, fmt.Errorf("%w: provisions are unavailable", ErrInvalidTavernRequest)
	}

	return state.Provision, nil
}

// checkOutdoorActorAttacks runs the production per-actor attack check for each
// visible enemy on the player's floor in the controller's descending slot order,
// without the movement and monster generation a full outdoor turn adds.
func checkOutdoorActorAttacks(g *GameState) bool {
	enemies := g.CurrentNPCAIController.GetNpcs().GetEnemiesMatching(func(*map_units.NPCEnemy) bool { return true })
	slices.SortFunc(enemies, func(left, right *map_units.NPCEnemy) int {
		return right.MapUnitDetails().NPCNum - left.MapUnitDetails().NPCNum
	})
	acted := false
	for _, enemy := range enemies {
		if !enemy.IsVisible() || enemy.Floor() != g.MapState.PlayerLocation.Floor {
			continue
		}
		switch g.checkOutdoorActorAttack(enemy) {
		case ai.OutdoorActorNoAttack:
		case ai.OutdoorActorAttacked:
			acted = true
		case ai.OutdoorActorDeferred:
			return true
		}
	}

	return acted
}

// attackCombatDirection drives the production Attack command at the square
// next to the active PC; see attackCombatSquare.
func attackCombatDirection(t *testing.T, g *GameState, direction references.Direction) bool {
	t.Helper()
	identity, active := g.activeCombatPCIdentity()
	require.True(t, active, "attack needs an active PC")
	origin := g.CombatScene.GetCombatantPosition(identity)
	target := direction.GetNewPositionInDirection(&origin)
	require.NotNil(t, target)

	return attackCombatSquare(t, g, *target)
}

// attackCombatSquare drives the production Attack command as a player would:
// it aims each melee source at target and confirms it. Ranged sources, and
// sources left after the target is gone, are skipped like an Escape press.
// It reports whether the Attack command began.
func attackCombatSquare(t *testing.T, g *GameState, target references.Position) bool {
	t.Helper()
	if !g.BeginCombatAttack() {
		return false
	}
	confirmed := false
	for g.CombatScene.AttackTargeting != nil {
		targeting := g.CombatScene.AttackTargeting
		source := targeting.CurrentSource()
		occupied := g.CombatScene.MonsterManager.GetMonstersAtPosition(target)
		if source.Equipment.GetAttackRange() > 0 || (confirmed && len(occupied) == 0) {
			g.CancelCombatAttackSource()

			continue
		}
		for targeting.Cursor != target {
			step := func(from, to references.Coordinate) int {
				return min(max(int(to)-int(from), -1), 1)
			}
			require.True(t, g.MoveCombatAttackCursor(step(targeting.Cursor.X, target.X), step(targeting.Cursor.Y, target.Y)),
				"attack cursor cannot reach %v", target)
		}
		g.ConfirmCombatAttackTarget()
		confirmed = true
	}

	return true
}

// testCampingMap loads the original camp layout that production camping requires.
func testCampingMap(t *testing.T) *references.CombatMapReference {
	t.Helper()
	campingMap, err := references.LoadCampingMap(helpers.LoadTestConfiguration(t))
	require.NoError(t, err)

	return campingMap
}

// wordForDungeon returns the Word of Power that opens dungeon, or "" for a
// location without one.
func wordForDungeon(dungeon references.Location) string {
	for _, candidate := range canonicalDungeonWords {
		if candidate.dungeon == dungeon {
			return candidate.word
		}
	}

	return ""
}

// countEqual returns how many values equal want.
func countEqual[T comparable](values []T, want T) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}

	return count
}

// newBritainCombatTestState builds a seeded Britain state and enters the
// Britannia combat territory from the north.
func newBritainCombatTestState(t *testing.T, seed uint64) (*GameState, *MockSystemCallbacks) {
	t.Helper()
	gs, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(seed).
		Build()
	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))

	return gs, mockCallbacks
}

// roundTripSaveSnapshot captures state as a native save snapshot and returns
// the state rebuilt from it.
func roundTripSaveSnapshot(t *testing.T, state *GameState) *GameState {
	t.Helper()
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	hydrated, err := NewGameStateFromSaveSnapshot(
		snapshot, state.GameConfig, state.GameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen,
	)
	require.NoError(t, err)

	return hydrated
}
