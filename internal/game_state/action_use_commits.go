package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

const (
	potionRandomEffectRollMax = 16
	potionRandomPotionMax     = 8
)

// HMSCapeUseResult identifies review and commit outcomes for the global preference.
type HMSCapeUseResult int

//nolint:revive // The typed outcomes are the complete service and presentation contract.
const (
	HMSCapeUseNoPlans HMSCapeUseResult = iota
	HMSCapeUseReady
	HMSCapeUseStale
	HMSCapeUseEnabled
	HMSCapeUseDisabled
)

// HMSCapeUseReview captures the durable state shown by one confirmation.
type HMSCapeUseReview struct {
	Result   HMSCapeUseResult
	Quantity uint16
	Enabled  bool
}

// HMSCapeUseCommit authorizes one confirmed preference change.
type HMSCapeUseCommit struct {
	state     *GameState
	review    HMSCapeUseReview
	location  references.PlayerLocation
	owner     CombatUseOwner
	committed bool
}

// Enables reports the preference value that confirmation will commit.
func (c *HMSCapeUseCommit) Enables() bool {
	if c == nil {
		panic("nil HMS Cape use commit")
	}

	return !c.review.Enabled
}

// ReviewHMSCapeUse projects plan ownership and the current global preference.
func (g *GameState) ReviewHMSCapeUse() HMSCapeUseReview {
	review := HMSCapeUseReview{
		Result:   HMSCapeUseNoPlans,
		Quantity: g.PartyState.Inventory.QuestItems.Get(references.HMSCape),
		Enabled:  g.IsHMSCapeActivated(),
	}
	if review.Quantity > 0 {
		review.Result = HMSCapeUseReady
	}

	return review
}

// BeginHMSCapeUse captures a one-shot confirmation without mutation or RNG.
func (g *GameState) BeginHMSCapeUse() (*HMSCapeUseCommit, HMSCapeUseReview) {
	review := g.ReviewHMSCapeUse()
	if review.Result != HMSCapeUseReady {
		return nil, review
	}

	return &HMSCapeUseCommit{
		state: g, review: review, location: g.MapState.PlayerLocation, owner: g.CaptureCombatUseOwner(),
	}, review
}

// CommitHMSCapeUse revalidates and applies one confirmed global preference change.
func (g *GameState) CommitHMSCapeUse(commit *HMSCapeUseCommit) HMSCapeUseResult {
	if commit == nil || commit.state != g || commit.committed {
		panic("invalid HMS Cape use commit")
	}
	commit.committed = true
	if g.ReviewHMSCapeUse() != commit.review || g.MapState.PlayerLocation != commit.location ||
		!g.isRetainedUseContext(commit.owner) {
		return HMSCapeUseStale
	}
	g.SetHMSCapeActivated(!commit.review.Enabled)
	g.CommitCastUseAttempt()
	if commit.review.Enabled {
		return HMSCapeUseDisabled
	}

	return HMSCapeUseEnabled
}

// PotionUseReview is the authoritative availability projection for one potion use.
type PotionUseReview struct {
	Potion          references.Potion
	Context         UsageContext
	Quantity        uint16
	Ready           bool
	EffectAvailable bool
}

// PotionUseCommit captures the state reviewed before one potion is consumed.
type PotionUseCommit struct {
	state          *GameState
	review         PotionUseReview
	combatIdentity CombatTurnIdentity
	owner          CombatUseOwner
	location       references.PlayerLocation
	committed      bool
}

// Potion returns the reviewed inventory identity without committing its use.
func (c *PotionUseCommit) Potion() references.Potion {
	if c == nil {
		panic("nil potion use commit")
	}

	return c.review.Potion
}

// PotionUseOutcome authorizes the original post-target random roll and effect.
// CommitPotionEffect consumes it at most once.
type PotionUseOutcome struct {
	FinalPotion references.Potion
	Mislabelled bool

	state           *GameState
	context         UsageContext
	combatIdentity  CombatTurnIdentity
	owner           CombatUseOwner
	effectCommitted bool
}

// PotionEffectResult identifies the authoritative effect applied after potion consumption.
type PotionEffectResult int

//nolint:revive // The typed outcome names are the complete effect documentation used by presentation switches.
const (
	PotionEffectNoEffect PotionEffectResult = iota
	PotionEffectAwakened
	PotionEffectHealed
	PotionEffectCured
	PotionEffectPoisoned
	PotionEffectSlept
	PotionEffectRat
	PotionEffectInvisible
	PotionEffectXRay
	PotionEffectNoNotice
)

// ReviewPotionUse projects the same inventory and context rules used by BeginPotionUse.
func (g *GameState) ReviewPotionUse(potion references.Potion, context UsageContext) PotionUseReview {
	review := PotionUseReview{
		Potion:          potion,
		Context:         context,
		EffectAvailable: true,
	}
	if potion < references.Blue || potion > references.White {
		return review
	}
	review.Quantity = g.PartyState.Inventory.Potions.Get(potion)
	if review.Quantity == 0 ||
		(context != UsageField && context != UsageCombat && context != UsageDungeon) {
		return review
	}
	switch context {
	case UsageCombat:
		if g.CombatScene == nil || !g.CombatScene.GetCurrentTurnIdentity().IsPartyMember {
			return review
		}
	case UsageDungeon:
		if g.CombatScene != nil || g.MapState.PlayerLocation.Location.GetMapType() != references.DungeonMapType {
			return review
		}
	case UsageField:
		if g.CombatScene != nil || g.MapState.PlayerLocation.Location.GetMapType() == references.DungeonMapType {
			return review
		}
	case UsageAnytime:
		return review
	}

	review.Ready = true
	review.EffectAvailable = (context == UsageCombat || (potion != references.Purple && potion != references.Black)) &&
		(context == UsageField || potion != references.White)

	return review
}

// BeginPotionUse captures a one-shot commit without consuming inventory or RNG.
func (g *GameState) BeginPotionUse(potion references.Potion, context UsageContext) (*PotionUseCommit, PotionUseReview) {
	review := g.ReviewPotionUse(potion, context)
	if !review.Ready {
		return nil, review
	}
	commit := &PotionUseCommit{
		state: g, review: review, location: g.MapState.PlayerLocation, owner: g.CaptureCombatUseOwner(),
	}
	if context == UsageCombat {
		commit.combatIdentity = g.CombatScene.GetCurrentTurnIdentity()
	}

	return commit, review
}

// CommitPotionUse consumes one potion at final confirmation. The original
// mislabelling rolls remain deferred until CommitPotionEffect after commitment.
// REDUX DEVIATION: original item-use rules consume before recipient selection; Redux cancellation
// and prerequisite rejection are free in every context, not effect failures.
// A stale or reused commit has no inventory or RNG effects.
// See ref:CONSUME-0001.
func (g *GameState) CommitPotionUse(commit *PotionUseCommit) (*PotionUseOutcome, bool) {
	if commit == nil || commit.state != g || commit.committed {
		panic("invalid potion use commit")
	}
	commit.committed = true
	current := g.ReviewPotionUse(commit.review.Potion, commit.review.Context)
	if current != commit.review || g.MapState.PlayerLocation != commit.location ||
		!g.isRetainedUseContext(commit.owner) {
		return nil, false
	}
	if !g.PartyState.Inventory.Potions.DecrementByOne(commit.review.Potion) {
		panic("reviewed potion could not be consumed")
	}
	// REDUX DEVIATION: potion use plays item activation, not a separate
	// drinking sound. Redux reserves consumption feedback even for a no-notice effect.
	// See ref:CONSUME-0001.
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundPotionDrink)
	g.CommitCastUseAttempt()

	return &PotionUseOutcome{
		FinalPotion:    commit.review.Potion,
		state:          g,
		context:        commit.review.Context,
		combatIdentity: commit.combatIdentity,
		owner:          commit.owner,
	}, true
}

// CommitPotionUseForTarget rejects a structurally invalid target before consuming.
func (g *GameState) CommitPotionUseForTarget(commit *PotionUseCommit, targetIndex int) (*PotionUseOutcome, bool) {
	if !g.PartyState.HasCharacter(targetIndex) || !g.PartyState.GetCharacter(targetIndex).IsInParty() {
		return nil, false
	}

	return g.CommitPotionUse(commit)
}

// CommitPotionEffect applies the selected target effect without owning presentation.
func (g *GameState) CommitPotionEffect(outcome *PotionUseOutcome, targetIndex int) PotionEffectResult {
	if outcome == nil || outcome.state != g || outcome.effectCommitted {
		panic("invalid potion effect commit")
	}
	outcome.effectCommitted = true
	if outcome.context == UsageCombat {
		if !g.IsCurrentCombatUseOwner(outcome.owner) {
			return PotionEffectNoEffect
		}
		targetIndex = outcome.combatIdentity.EntityIndex
	} else if g.CombatScene != nil {
		return PotionEffectNoEffect
	}
	character := g.PartyState.GetCharacter(targetIndex)
	if character == nil {
		return PotionEffectNoEffect
	}

	finalPotion := outcome.FinalPotion
	switch g.RandomIntInRange(0, potionRandomEffectRollMax-1) {
	case 0:
		finalPotion = references.Orange
	case 1:
		finalPotion = references.Potion(g.RandomIntInRange(0, potionRandomPotionMax-1))
	}
	// REDUX DEVIATION: CONSUME-0002: the original emits no mislabel warning. Redux flags a
	// resolved color that differs from the label so the player is told. See ref:CONSUME-0002.
	outcome.Mislabelled = finalPotion != outcome.FinalPotion
	outcome.FinalPotion = finalPotion

	switch outcome.FinalPotion {
	case references.Blue:
		if outcome.context != UsageCombat {
			if g.AwakenCharacter(targetIndex) {
				return PotionEffectAwakened
			}

			break
		}
		if !character.IsAsleep() {
			break
		}
		// Original item-use rules clears shared status first, but tactical wake-up applies only to
		// the retained drinking entry when its local sleep mask is set.
		// See ref:CONSUME-0001.
		character.SetStatus(party_state.Good)
		if g.CombatScene.isCombatPCSleeping(outcome.combatIdentity) {
			g.setCombatPCSleeping(outcome.combatIdentity, false)

			return PotionEffectAwakened
		}
	case references.Yellow:
		if character.IsDead() {
			break
		}
		// Original spell-effect rules healing rolls even when the living target is already at max HP.
		// See ref:CONSUME-0001.
		hpBefore := character.GetCurrentHp()
		character.AddHp(uint16(g.RollOriginalD30())) // Healing calls transformed d30.
		// See ref:RNG-0001.
		changes := transientStatusChanges{}
		changes.observeHealing(hpBefore, character)
		g.publishTransientStatusChanges(changes)

		return PotionEffectHealed
	case references.Red:
		if character.IsPoisoned() {
			statusBefore := character.GetStatus()
			character.SetStatus(party_state.Good)
			changes := transientStatusChanges{}
			changes.observePoison(statusBefore, character)
			g.publishTransientStatusChanges(changes)

			return PotionEffectCured
		}
	case references.Green:
		if character.IsGood() {
			statusBefore := character.GetStatus()
			character.SetStatus(party_state.Poisoned)
			changes := transientStatusChanges{}
			changes.observePoison(statusBefore, character)
			g.publishTransientStatusChanges(changes)

			return PotionEffectPoisoned
		}
	case references.Orange:
		if character.IsGood() {
			if outcome.context == UsageCombat {
				// REDUX DEVIATION: CONSUME-0002: the original passes the roster index to a
				// queue-indexed sleep operation that can hit another entry. Redux sleeps the
				// exact drinking PC or clone. See ref:CONSUME-0002.
				g.setCombatPCSleeping(outcome.combatIdentity, true)
			} else {
				character.SetStatus(party_state.Sleep)
			}

			return PotionEffectSlept
		}
	case references.Purple:
		if outcome.context != UsageCombat {
			return PotionEffectNoNotice
		}
		if g.TransformActivePartyMemberIntoRat(targetIndex) {
			return PotionEffectRat
		}
	case references.Black:
		if outcome.context != UsageCombat {
			return PotionEffectNoNotice
		}
		if g.CastCombatSanctLor(targetIndex) {
			// Black replaces the base appearance, unlike Sanct Lor's rendered-shape
			// change. It therefore ends Purple's standing-poison protection. ref:COMBAT-0007.
			g.CombatScene.TurnQueue.GetEntryByIdentity(outcome.combatIdentity).ratAppearance = false

			return PotionEffectInvisible
		}
	case references.White:
		if outcome.context == UsageField {
			return PotionEffectXRay
		}

		return PotionEffectNoNotice
	default:
		panic("unknown potion effect")
	}

	return PotionEffectNoEffect
}

// CancelPotionEffect invalidates an unresolved post-consumption effect without using RNG.
func (g *GameState) CancelPotionEffect(outcome *PotionUseOutcome) bool {
	if outcome == nil || outcome.state != g {
		panic("invalid potion effect cancellation")
	}
	if outcome.effectCommitted {
		return false
	}
	outcome.effectCommitted = true

	return true
}

// CarpetBoardingResult is both the review disposition and commit result.
type CarpetBoardingResult int

//nolint:revive // The typed result names distinguish review failures from commit outcomes.
const (
	CarpetBoardingReady CarpetBoardingResult = iota
	CarpetBoardingNoInventory
	CarpetBoardingNotHere
	CarpetBoardingExitShipFirst
	CarpetBoardingOnlyOnFoot
	CarpetBoardingOccupied
	CarpetBoardingNoMapCapacity
	CarpetBoardingStale
	CarpetBoardingSuccess
)

// CarpetBoardingReview captures the authoritative state shown by Use projection.
type CarpetBoardingReview struct {
	Result      CarpetBoardingResult
	Location    references.PlayerLocation
	Quantity    uint16
	VehicleType references.VehicleType
	Tile        indexes.SpriteIndex
}

// CarpetBoardingCommit authorizes one atomic inventory-to-vehicle transition.
type CarpetBoardingCommit struct {
	state     *GameState
	review    CarpetBoardingReview
	units     *map_units.MapUnits
	committed bool
}

// ReviewCarpetBoarding projects all fallible carpet placement conditions.
func (g *GameState) ReviewCarpetBoarding() CarpetBoardingReview {
	review := CarpetBoardingReview{
		Result:      CarpetBoardingNoInventory,
		Location:    g.MapState.PlayerLocation,
		Quantity:    g.PartyState.Inventory.QuestItems.Get(references.Carpet),
		VehicleType: g.PartyVehicle.GetVehicleDetails().VehicleType,
	}
	if review.Quantity == 0 {
		return review
	}
	mapType := review.Location.Location.GetMapType()
	// Original item-use rules permit towns and both large-map layers; level is not gated.
	// See ref:CONSUME-0001.
	if mapType != references.SmallMapType && mapType != references.LargeMapType {
		review.Result = CarpetBoardingNotHere

		return review
	}
	tile := g.GetLayeredMapByCurrentLocation().GetTopTile(&review.Location.Position)
	if tile == nil || tile.IsMountain() {
		review.Result = CarpetBoardingNotHere

		return review
	}
	review.Tile = tile.Index
	if review.VehicleType == references.FrigateVehicle || review.VehicleType == references.SkiffVehicle {
		review.Result = CarpetBoardingExitShipFirst

		return review
	}
	if review.VehicleType != references.NoPartyVehicle {
		review.Result = CarpetBoardingOnlyOnFoot

		return review
	}
	if g.CurrentNPCAIController == nil || g.CurrentNPCAIController.GetNpcs() == nil {
		review.Result = CarpetBoardingNoMapCapacity

		return review
	}
	units := g.CurrentNPCAIController.GetNpcs()
	if units.GetMapUnitAtPositionAndFloorOrNil(review.Location.Position, review.Location.Floor) != nil {
		review.Result = CarpetBoardingOccupied

		return review
	}
	if !units.HasCapacity() {
		review.Result = CarpetBoardingNoMapCapacity

		return review
	}
	review.Result = CarpetBoardingReady

	return review
}

// BeginCarpetBoarding captures one placement commit without mutation.
func (g *GameState) BeginCarpetBoarding() (*CarpetBoardingCommit, CarpetBoardingReview) {
	review := g.ReviewCarpetBoarding()
	if review.Result != CarpetBoardingReady {
		return nil, review
	}

	return &CarpetBoardingCommit{
		state:  g,
		review: review,
		units:  g.CurrentNPCAIController.GetNpcs(),
	}, review
}

// CommitCarpetBoarding consumes, places, and boards one carpet as one core operation.
func (g *GameState) CommitCarpetBoarding(commit *CarpetBoardingCommit) CarpetBoardingResult {
	return g.commitCarpetBoarding(commit, true)
}

// CommitCarpetBoardingForSurfaceUse leaves ordinary cycle completion to the owning Use workflow.
func (g *GameState) CommitCarpetBoardingForSurfaceUse(commit *CarpetBoardingCommit) CarpetBoardingResult {
	return g.commitCarpetBoarding(commit, false)
}

func (g *GameState) commitCarpetBoarding(
	commit *CarpetBoardingCommit,
	advanceOrdinaryMinutes bool,
) CarpetBoardingResult {
	if commit == nil || commit.state != g || commit.committed {
		panic("invalid carpet boarding commit")
	}
	commit.committed = true
	if g.ReviewCarpetBoarding() != commit.review ||
		g.CurrentNPCAIController.GetNpcs() != commit.units {
		return CarpetBoardingStale
	}
	g.CommitCastUseAttempt()
	carpetDirection := references.Right
	if g.RandomIntInRange(0, 1) == 1 {
		carpetDirection = references.Left
	}
	if !g.PartyState.Inventory.QuestItems.DecrementByOne(references.Carpet) {
		panic("reviewed carpet could not be consumed")
	}

	carpetReference := *references.NewNPCReferenceForVehicle(
		references.CarpetVehicle,
		commit.review.Location.Position,
		commit.review.Location.Floor,
	)
	carpetReference.Schedule.OverrideAllPositions(
		byte(commit.review.Location.Position.X),
		byte(commit.review.Location.Position.Y),
	)
	carpet := *map_units.NewNPCFriendlyVehicle(references.CarpetVehicle, carpetReference)
	carpet.GetVehicleDetails().SetPartyVehicleDirection(carpetDirection)
	if !commit.units.AddVehicle(carpet) {
		panic("reviewed carpet placement failed")
	}
	if g.BoardVehicle(carpet) != BoardVehicleResultSuccess {
		panic("reviewed carpet boarding failed")
	}
	if advanceOrdinaryMinutes {
		g.AdvanceOrdinarySurfaceCommandMinutes(1)
	}

	return CarpetBoardingSuccess
}

// SkullKeyUseResult identifies availability and consumption outcomes.
type SkullKeyUseResult int

//nolint:revive // The typed result names distinguish availability, stale state, and consumption.
const (
	SkullKeyUseReady SkullKeyUseResult = iota
	SkullKeyUseNoInventory
	SkullKeyUseNotHere
	SkullKeyUseStale
	SkullKeyUseConsumed
)

// SkullKeyUseReview captures the prerequisites shown before direction input.
type SkullKeyUseReview struct {
	Result   SkullKeyUseResult
	Location references.PlayerLocation
	Quantity uint16
}

// SkullKeyUseCommit authorizes one post-confirmation consumption.
type SkullKeyUseCommit struct {
	state           *GameState
	review          SkullKeyUseReview
	owner           CombatUseOwner
	committed       bool
	consumed        bool
	unlockCommitted bool
}

// ReviewSkullKeyUse projects the same location and inventory rules used by BeginSkullKeyUse.
func (g *GameState) ReviewSkullKeyUse() SkullKeyUseReview {
	review := SkullKeyUseReview{
		Result:   SkullKeyUseNoInventory,
		Location: g.MapState.PlayerLocation,
		Quantity: g.PartyState.Inventory.Provisions.SkullKeys.Get(),
	}
	if review.Quantity == 0 {
		return review
	}
	mapType := review.Location.Location.GetMapType()
	if mapType == references.CombatMapType && !g.IsCurrentCombatUseOwner(g.CaptureCombatUseOwner()) {
		review.Result = SkullKeyUseNotHere

		return review
	}
	if mapType != references.SmallMapType &&
		(mapType != references.LargeMapType || !g.MapState.IsOverworld()) &&
		mapType != references.CombatMapType {
		review.Result = SkullKeyUseNotHere

		return review
	}
	review.Result = SkullKeyUseReady

	return review
}

// BeginSkullKeyUse captures one key-consumption commit without mutation.
func (g *GameState) BeginSkullKeyUse() (*SkullKeyUseCommit, SkullKeyUseReview) {
	review := g.ReviewSkullKeyUse()
	if review.Result == SkullKeyUseNoInventory {
		return nil, review
	}

	return &SkullKeyUseCommit{state: g, review: review, owner: g.CaptureCombatUseOwner()}, review
}

// CommitSkullKeyUse consumes one key after final direction confirmation.
// REDUX DEVIATION: original item-use rules consume before magical unlocking; Redux cancellation is free.
// See ref:PROV-0006.
func (g *GameState) CommitSkullKeyUse(commit *SkullKeyUseCommit) SkullKeyUseResult {
	if commit == nil || commit.state != g || commit.committed {
		panic("invalid Skull Key use commit")
	}
	commit.committed = true
	if g.ReviewSkullKeyUse() != commit.review || !g.isRetainedUseContext(commit.owner) {
		return SkullKeyUseStale
	}
	if commit.review.Result == SkullKeyUseNotHere {
		return SkullKeyUseNotHere
	}
	if !g.PartyState.Inventory.Provisions.SkullKeys.DecrementByOne() {
		panic("reviewed Skull Key could not be consumed")
	}
	commit.consumed = true
	g.CommitCastUseAttempt()

	return SkullKeyUseConsumed
}

// SkullKeyUnlockResult identifies one directional unlock result.
type SkullKeyUnlockResult int

//nolint:revive // The typed result names distinguish review, stale state, and mutation outcomes.
const (
	SkullKeyUnlockNoEffect SkullKeyUnlockResult = iota
	SkullKeyUnlockReady
	SkullKeyUnlockStale
	SkullKeyUnlockSuccess
)

// SkullKeyUnlockReview captures one adjacent tile before mutation.
type SkullKeyUnlockReview struct {
	Result       SkullKeyUnlockResult
	Location     references.PlayerLocation
	Target       references.Position
	LockedDoor   indexes.SpriteIndex
	UnlockedDoor indexes.SpriteIndex
	useCommit    *SkullKeyUseCommit
}

// ReviewSkullKeyUnlock resolves the adjacent door for one consumed key without mutation.
func (g *GameState) ReviewSkullKeyUnlock(
	useCommit *SkullKeyUseCommit,
	direction references.Direction,
) SkullKeyUnlockReview {
	review := SkullKeyUnlockReview{
		Result:    SkullKeyUnlockNoEffect,
		Location:  g.MapState.PlayerLocation,
		useCommit: useCommit,
	}
	if useCommit == nil || useCommit.state != g || !useCommit.consumed || useCommit.unlockCommitted {
		return review
	}
	target := direction.GetNewPositionInDirection(&review.Location.Position)
	if useCommit.owner.combat != nil {
		if !g.IsCurrentCombatUseOwner(useCommit.owner) {
			review.Result = SkullKeyUnlockStale

			return review
		}
		target = direction.GetNewPositionInDirection(&useCommit.owner.origin)
	}
	if target == nil {
		return review
	}
	review.Target = *target
	if useCommit.owner.combat != nil {
		if target.X < 0 || target.Y < 0 ||
			target.X >= references.XCombatMapTiles || target.Y >= references.YCombatMapTiles {
			return review
		}
		review.LockedDoor = g.CombatScene.CombatMapRef.GetTileNumber(*target)
	} else {
		tile := g.GetLayeredMapByCurrentLocation().GetTopTile(target)
		if tile == nil {
			return review
		}
		review.LockedDoor = tile.Index
	}
	//nolint:exhaustive // Every non-door sprite intentionally remains a no-effect result.
	switch review.LockedDoor {
	case indexes.MagicLockDoor:
		review.UnlockedDoor = indexes.RegularDoor
	case indexes.MagicLockDoorWithView:
		review.UnlockedDoor = indexes.RegularDoorView
	default:
		return review
	}
	review.Result = SkullKeyUnlockReady

	return review
}

// CommitSkullKeyUnlockForSurfaceUse leaves ordinary cycle completion to the owning Use workflow.
func (g *GameState) CommitSkullKeyUnlockForSurfaceUse(review SkullKeyUnlockReview) SkullKeyUnlockResult {
	return g.commitSkullKeyUnlock(review, false)
}

func (g *GameState) commitSkullKeyUnlock(
	review SkullKeyUnlockReview,
	advanceOrdinaryMinutes bool,
) SkullKeyUnlockResult {
	if review.useCommit == nil || review.useCommit.state != g ||
		!review.useCommit.consumed || review.useCommit.unlockCommitted {
		return SkullKeyUnlockNoEffect
	}
	if review.Result != SkullKeyUnlockReady {
		review.useCommit.unlockCommitted = true

		return SkullKeyUnlockNoEffect
	}
	origin := review.Location.Position
	if review.useCommit.owner.combat != nil {
		origin = review.useCommit.owner.origin
	}
	current := g.ReviewSkullKeyUnlock(review.useCommit, directionBetween(origin, review.Target))
	review.useCommit.unlockCommitted = true
	if g.MapState.PlayerLocation != review.Location || current != review {
		return SkullKeyUnlockStale
	}
	if review.useCommit.owner.combat != nil {
		g.setCombatTerrain(review.Target, review.UnlockedDoor)
	} else {
		g.GetLayeredMapByCurrentLocation().SetTileByLayer(
			map_state.MapOverrideLayer,
			&review.Target,
			review.UnlockedDoor,
		)
		if advanceOrdinaryMinutes {
			g.AdvanceOrdinarySurfaceCommandMinutes(1)
		}
	}

	return SkullKeyUnlockSuccess
}

// CancelSkullKeyUnlock invalidates an unresolved post-consumption direction attempt.
func (g *GameState) CancelSkullKeyUnlock(commit *SkullKeyUseCommit) bool {
	if commit == nil || commit.state != g || !commit.consumed || commit.unlockCommitted {
		return false
	}
	commit.unlockCommitted = true

	return true
}

func directionBetween(origin references.Position, target references.Position) references.Direction {
	switch {
	case target == origin.Up():
		return references.Up
	case target == origin.Down():
		return references.Down
	case target == origin.Left():
		return references.Left
	case target == origin.Right():
		return references.Right
	default:
		return references.NoneDirection
	}
}
