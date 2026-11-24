package game_state

import (
	"errors"
	"fmt"
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

var (
	// ErrInvalidInnRequest indicates stale, tampered, or unsupported reviewed input.
	ErrInvalidInnRequest = errors.New("invalid inn request")
	// ErrInnRestUnavailable indicates that the reviewed room or payment is unavailable.
	ErrInnRestUnavailable = errors.New("inn rest unavailable")
	// ErrInnLeaveUnavailable indicates that the reviewed companion cannot be left.
	ErrInnLeaveUnavailable = errors.New("inn leave unavailable")
	// ErrInnPickupUnavailable indicates that the reviewed guest cannot be picked up.
	ErrInnPickupUnavailable = errors.New("inn pickup unavailable")
)

const maximumInnMonths = 25

// InnWarning identifies a consequential but confirmable Inn outcome.
type InnWarning byte

const (
	// InnWarningNone indicates no consequential warning.
	InnWarningNone InnWarning = iota
	// InnWarningPoisonDeath warns that poison becomes death during restoration.
	InnWarningPoisonDeath
	// InnWarningLastLivingMember warns that Leave removes the final living member.
	InnWarningLastLivingMember
	// InnWarningMinimumMonthCharge warns that zero accrued months still bills one month.
	InnWarningMinimumMonthCharge
	// InnWarningInsufficientFundsActivatesGuards warns that confirmation is consequential.
	InnWarningInsufficientFundsActivatesGuards
)

// InnRestBlockReason identifies why a nightly stay cannot currently commit.
type InnRestBlockReason byte

const (
	// InnRestBlockNone indicates that vacancy permits Rest.
	InnRestBlockNone InnRestBlockReason = iota
	// InnRestBlockNoVacancy indicates that all guest rooms are occupied.
	InnRestBlockNoVacancy
)

// InnLeaveBlockReason identifies one active member's current leave gate.
type InnLeaveBlockReason byte

const (
	// InnLeaveBlockNone indicates that the member can be left.
	InnLeaveBlockNone InnLeaveBlockReason = iota
	// InnLeaveBlockAvatar preserves the Avatar in slot zero.
	InnLeaveBlockAvatar
	// InnLeaveBlockDead prevents leaving a dead active member.
	InnLeaveBlockDead
	// InnLeaveBlockMinimumParty prevents reducing a one-member party.
	InnLeaveBlockMinimumParty
	// InnLeaveBlockNoVacancy indicates that all guest rooms are occupied.
	InnLeaveBlockNoVacancy
)

// InnPickupBlockReason identifies one stored guest's current pickup gate.
type InnPickupBlockReason byte

const (
	// InnPickupBlockNone indicates that the guest can be picked up or can trigger guards for nonpayment.
	InnPickupBlockNone InnPickupBlockReason = iota
	// InnPickupBlockDead implements the Redux decision to keep dead guests stored.
	InnPickupBlockDead
	// InnPickupBlockPartyFull indicates that the active prefix already contains six records.
	InnPickupBlockPartyFull
)

// InnPickupOutcome distinguishes a completed pickup from the faithful terminal guard refusal.
type InnPickupOutcome byte

const (
	// InnPickupCompleted indicates payment, transfer, and restoration succeeded.
	InnPickupCompleted InnPickupOutcome = iota
	// InnPickupRefusedWithGuards is the typed terminal insufficient-funds result.
	InnPickupRefusedWithGuards
)

// InnCharacterIdentity remains stable when Inn operations move a complete
// character record to another roster index.
type InnCharacterIdentity struct {
	Name   string
	Gender party_state.CharacterGender
	Class  party_state.CharacterClass
}

// InnCharacterProjection describes exact character state before and after one service.
type InnCharacterProjection struct {
	Identity          InnCharacterIdentity
	RosterIndexBefore int
	RosterIndexAfter  int
	StatusBefore      party_state.CharacterStatus
	StatusAfter       party_state.CharacterStatus
	HPBefore          uint16
	HPAfter           uint16
	MaxHP             uint16
	MPBefore          byte
	MPAfter           byte
	PartyMapBefore    party_state.PartyMap
	PartyMapAfter     party_state.PartyMap
	MonthsBefore      byte
	MonthsAfter       byte
	Warnings          []InnWarning
}

// InnRestQuote is the trusted nightly charge and complete overnight projection.
type InnRestQuote struct {
	Charge            uint16
	Affordable        bool
	BlockReason       InnRestBlockReason
	GoldBefore        uint16
	GoldAfterPayment  uint16
	SleepLocation     references.PlayerLocation
	WakeLocation      references.PlayerLocation
	DateTimeBefore    datetime.UltimaDate
	DateTimeAfter     datetime.UltimaDate
	TimeStepsMinutes  []int
	MemberProjections []InnCharacterProjection
	Warnings          []InnWarning
}

// InnLeaveRow is one active member's trusted leave eligibility and projection.
type InnLeaveRow struct {
	Identity       InnCharacterIdentity
	RosterIndex    int
	Status         party_state.CharacterStatus
	Eligible       bool
	BlockReason    InnLeaveBlockReason
	Projection     InnCharacterProjection
	Warnings       []InnWarning
	PartySizeAfter int
	OccupancyAfter int
}

// InnGuestRow is one current-Inn guest in complete-record roster order.
type InnGuestRow struct {
	Identity           InnCharacterIdentity
	RosterIndex        int
	Status             party_state.CharacterStatus
	MonthsAtInn        byte
	BilledMonths       int
	MonthlyRate        uint16
	Charge             uint16
	Affordable         bool
	Confirmable        bool
	BlockReason        InnPickupBlockReason
	Projection         InnCharacterProjection
	Warnings           []InnWarning
	GoldBefore         uint16
	GoldAfterPayment   uint16
	PartySizeAfter     int
	RoomOccupancyAfter int
}

// InnShopState is a trusted detached snapshot of one Inn session. The captured
// Intelligence scalar is deliberately private; presentation consumes only the
// finalized service charges.
type InnShopState struct {
	Merchant              references.MerchantDefinition
	PricingCharacterIndex int
	PricingCharacterName  string
	Gold                  uint16
	RoomOccupancy         int
	RoomCapacity          int
	PartySize             int
	PartyCapacity         int
	MonthlyRate           uint16
	RosterFingerprint     string
	Rest                  InnRestQuote
	LeaveRows             []InnLeaveRow
	GuestRows             []InnGuestRow
	pricingIntelligence   int
}

// InnShopReview is a pure production snapshot with canonical sprite identity.
type InnShopReview = MerchantShopReview[InnShopState]

// InnRestRequest carries comparison-only nightly review authority.
type InnRestRequest struct {
	ReviewedMerchantID            references.MerchantID
	ReviewedPricingCharacterIndex int
	ReviewedRosterFingerprint     string
	ReviewedCharge                uint16
	ReviewedGoldBefore            uint16
	ReviewedDateTimeBefore        datetime.UltimaDate
	ReviewedDateTimeAfter         datetime.UltimaDate
}

// InnLeaveRequest identifies one reviewed active complete record.
type InnLeaveRequest struct {
	ReviewedMerchantID            references.MerchantID
	ReviewedPricingCharacterIndex int
	ReviewedRosterFingerprint     string
	Character                     InnCharacterIdentity
	ReviewedCharacterIndex        int
}

// InnPickupRequest identifies one reviewed stored complete record and exact bill.
type InnPickupRequest struct {
	ReviewedMerchantID            references.MerchantID
	ReviewedPricingCharacterIndex int
	ReviewedRosterFingerprint     string
	Character                     InnCharacterIdentity
	ReviewedCharacterIndex        int
	ReviewedCharge                uint16
	ReviewedGoldBefore            uint16
}

// InnRegenerationResult reports one original check_ring success during Inn sleep.
type InnRegenerationResult struct {
	Character   InnCharacterIdentity
	RosterIndex int
	HPBefore    uint16
	HPAfter     uint16
}

// InnRestResult reports one complete authoritative overnight stay.
type InnRestResult struct {
	MerchantID            references.MerchantID
	PricingCharacter      InnCharacterIdentity
	PricingCharacterIndex int
	Charge                uint16
	GoldBefore            uint16
	GoldAfterPayment      uint16
	Theft                 uint16
	GoldAfterTheft        uint16
	FoodBefore            uint16
	FoodAfter             uint16
	PositionBefore        references.PlayerLocation
	SleepLocation         references.PlayerLocation
	WakeLocation          references.PlayerLocation
	DateTimeBefore        datetime.UltimaDate
	DateTimeAfter         datetime.UltimaDate
	ActiveSpellBefore     DurationEffect
	ActiveSpellAfter      DurationEffect
	TimeStepsMinutes      []int
	Regeneration          []InnRegenerationResult
	Members               []InnCharacterProjection
	RosterBefore          PartyRosterSnapshot
	RosterAfter           PartyRosterSnapshot
}

// InnLeaveResult reports one complete zero-cost stable roster transfer.
type InnLeaveResult struct {
	MerchantID           references.MerchantID
	Character            InnCharacterIdentity
	CharacterIndexBefore int
	CharacterIndexAfter  int
	GoldBefore           uint16
	GoldAfter            uint16
	PartySizeBefore      int
	PartySizeAfter       int
	RoomOccupancyBefore  int
	RoomOccupancyAfter   int
	Warnings             []InnWarning
	RosterBefore         PartyRosterSnapshot
	RosterAfter          PartyRosterSnapshot
	State                InnShopState
}

// InnPickupResult reports either a complete paid pickup or the terminal guard refusal.
type InnPickupResult struct {
	Outcome              InnPickupOutcome
	MerchantID           references.MerchantID
	Character            InnCharacterIdentity
	CharacterIndexBefore int
	CharacterIndexAfter  int
	MonthsAtInn          byte
	BilledMonths         int
	MonthlyRate          uint16
	ListedCharge         uint16
	GoldBefore           uint16
	GoldAfterPayment     uint16
	Theft                uint16
	GoldAfterTheft       uint16
	FinalGold            uint16
	GuardActivation      GuardActivationResult
	Projection           InnCharacterProjection
	RosterBefore         PartyRosterSnapshot
	RosterAfter          PartyRosterSnapshot
	State                InnShopState
}

// GetInnShopState returns the complete current Inn service state while retaining
// the session's originally captured pricing Intelligence.
func (g *GameState) GetInnShopState(session MerchantSession) (InnShopState, error) {
	buyer, buyerIndex, merchant, err := g.validateInnMerchantSession(session)
	if err != nil {
		return InnShopState{}, err
	}

	return g.buildInnShopState(merchant, buyer, buyerIndex, session.buyerIntelligence)
}

// ReviewInnShopByMerchantID builds production projections without issuing a
// session, invoking callbacks, consuming RNG, advancing time, or mutating state.
func (g *GameState) ReviewInnShopByMerchantID(id references.MerchantID) (InnShopReview, error) {
	return reviewMerchantShop(g, id, references.MerchantFamilyInn, ErrInvalidInnRequest, "an Inn", func(
		merchant references.MerchantDefinition, buyer *party_state.PlayerCharacter, buyerIndex int,
	) (InnShopState, error) {
		return g.buildInnShopState(merchant, buyer, buyerIndex, int(buyer.Intelligence))
	})
}

// RestAtInn revalidates and commits one complete original overnight transaction.
func (g *GameState) RestAtInn(session MerchantSession, request InnRestRequest) (InnRestResult, error) {
	state, err := g.GetInnShopState(session)
	if err != nil {
		return InnRestResult{}, err
	}
	if request.ReviewedMerchantID != state.Merchant.ID ||
		request.ReviewedPricingCharacterIndex != state.PricingCharacterIndex ||
		request.ReviewedRosterFingerprint == "" || request.ReviewedRosterFingerprint != state.RosterFingerprint ||
		request.ReviewedCharge != state.Rest.Charge || request.ReviewedGoldBefore != state.Gold ||
		request.ReviewedDateTimeBefore != state.Rest.DateTimeBefore ||
		request.ReviewedDateTimeAfter != state.Rest.DateTimeAfter {
		return InnRestResult{}, fmt.Errorf("%w: reviewed merchant, buyer, roster, charge, gold, or time changed", ErrInvalidInnRequest)
	}
	if state.Rest.BlockReason != InnRestBlockNone {
		return InnRestResult{}, fmt.Errorf("%w: room block reason %d", ErrInnRestUnavailable, state.Rest.BlockReason)
	}
	if !state.Rest.Affordable {
		return InnRestResult{}, fmt.Errorf("%w: need %d gold, have %d", ErrInnRestUnavailable, state.Rest.Charge, state.Gold)
	}

	beforeRoster := g.CurrentPartyRosterSnapshot()
	result := InnRestResult{
		MerchantID:            state.Merchant.ID,
		PricingCharacter:      innCharacterIdentity(session.buyer),
		PricingCharacterIndex: state.PricingCharacterIndex,
		Charge:                state.Rest.Charge,
		GoldBefore:            state.Gold,
		GoldAfterPayment:      state.Gold,
		GoldAfterTheft:        state.Gold,
		FoodBefore:            g.PartyState.Inventory.Provisions.Food.Get(),
		PositionBefore:        g.MapState.PlayerLocation,
		SleepLocation:         state.Rest.SleepLocation,
		WakeLocation:          state.Rest.WakeLocation,
		DateTimeBefore:        g.DateTime,
		ActiveSpellBefore:     g.ActiveDurationEffect,
		TimeStepsMinutes:      append([]int(nil), state.Rest.TimeStepsMinutes...),
		Members:               append([]InnCharacterProjection(nil), state.Rest.MemberProjections...),
		RosterBefore:          beforeRoster,
	}
	if !g.PartyState.Inventory.Gold.DecrementBy(state.Rest.Charge) {
		panic("validated Inn rest could not deduct gold")
	}
	result.GoldAfterPayment = g.PartyState.Inventory.Gold.Get()
	result.Theft = g.ApplyFaulineiShopTheft()
	result.GoldAfterTheft = g.PartyState.Inventory.Gold.Get()

	g.MapState.PlayerLocation = state.Rest.SleepLocation
	for index := range state.PartySize {
		character := g.PartyState.GetCharacter(index)
		if character.IsGood() {
			character.SetStatus(party_state.Sleep)
		}
	}
	// REDUX DEVIATION: CAMP-0007: Inn services clears active effect and duration counter on
	// entering the Inn conversation. Redux clears them only after paid Rest is
	// committed so Leave, Pick Up, cancellation, and refusal preserve the effect.
	// See ref:CAMP-0007, ref:MERCH-0010.
	g.ClearDurationEffect()

	for range 12 {
		g.advanceInnRestMinutes(5)
	}
	for g.DateTime.Hour != 6 {
		result.Regeneration = append(result.Regeneration, g.innRegenerationResults(g.ProcessRegenerationRingSweep())...)
		g.advanceInnRestMinutes(9)
	}
	g.lastProcessedHour = int(g.DateTime.Hour)
	g.CurrentNPCAIController.HardResetNPCs(true)
	changes := g.restoreInnMorningParty(state.PartySize)
	g.publishTransientStatusChanges(changes)
	g.MapState.PlayerLocation = state.Rest.WakeLocation

	result.DateTimeAfter = g.DateTime
	result.ActiveSpellAfter = g.ActiveDurationEffect
	result.FoodAfter = g.PartyState.Inventory.Provisions.Food.Get()
	result.RosterAfter = g.CurrentPartyRosterSnapshot()
	result.Members = innProjectionResults(result.Members, result.RosterAfter)

	return result, nil
}

// LeaveCompanionAtInn commits one zero-cost stable complete-record transfer.
func (g *GameState) LeaveCompanionAtInn(
	session MerchantSession,
	request InnLeaveRequest,
) (InnLeaveResult, error) {
	state, err := g.GetInnShopState(session)
	if err != nil {
		return InnLeaveResult{}, err
	}
	if request.ReviewedMerchantID != state.Merchant.ID ||
		request.ReviewedPricingCharacterIndex != state.PricingCharacterIndex ||
		request.ReviewedRosterFingerprint == "" || request.ReviewedRosterFingerprint != state.RosterFingerprint {
		return InnLeaveResult{}, fmt.Errorf("%w: reviewed merchant, buyer, or roster changed", ErrInvalidInnRequest)
	}
	row, ok := innLeaveRowAt(state.LeaveRows, request.ReviewedCharacterIndex, request.Character)
	if !ok {
		return InnLeaveResult{}, fmt.Errorf("%w: reviewed active character changed", ErrInvalidInnRequest)
	}
	if !row.Eligible {
		return InnLeaveResult{}, fmt.Errorf("%w: leave block reason %d", ErrInnLeaveUnavailable, row.BlockReason)
	}

	result := InnLeaveResult{
		MerchantID:           state.Merchant.ID,
		Character:            row.Identity,
		CharacterIndexBefore: row.RosterIndex,
		CharacterIndexAfter:  party_state.NRosterSlots - 1,
		GoldBefore:           state.Gold,
		GoldAfter:            state.Gold,
		PartySizeBefore:      state.PartySize,
		PartySizeAfter:       state.PartySize - 1,
		RoomOccupancyBefore:  state.RoomOccupancy,
		RoomOccupancyAfter:   state.RoomOccupancy + 1,
		Warnings:             append([]InnWarning(nil), row.Warnings...),
		RosterBefore:         g.CurrentPartyRosterSnapshot(),
	}
	if g.activePlayerIndex == row.RosterIndex {
		g.activePlayerIndex = -1
	} else if g.activePlayerIndex > row.RosterIndex {
		g.activePlayerIndex--
	}
	g.PartyState.LeaveCharacterAtInn(row.RosterIndex, state.Merchant.Location)
	result.RosterAfter = g.CurrentPartyRosterSnapshot()
	result.State, err = g.GetInnShopState(session)
	if err != nil {
		panic("Inn session became invalid after its own leave transfer: " + err.Error())
	}

	return result, nil
}

// PickUpInnGuest commits one paid stable transfer or faithfully activates
// guards once and returns a typed terminal refusal when gold is insufficient.
func (g *GameState) PickUpInnGuest(
	session MerchantSession,
	request InnPickupRequest,
) (InnPickupResult, error) {
	state, err := g.GetInnShopState(session)
	if err != nil {
		return InnPickupResult{}, err
	}
	if request.ReviewedMerchantID != state.Merchant.ID ||
		request.ReviewedPricingCharacterIndex != state.PricingCharacterIndex ||
		request.ReviewedRosterFingerprint == "" || request.ReviewedRosterFingerprint != state.RosterFingerprint ||
		request.ReviewedGoldBefore != state.Gold {
		return InnPickupResult{}, fmt.Errorf("%w: reviewed merchant, buyer, roster, or gold changed", ErrInvalidInnRequest)
	}
	row, ok := innGuestRowAt(state.GuestRows, request.ReviewedCharacterIndex, request.Character)
	if !ok || request.ReviewedCharge != row.Charge {
		return InnPickupResult{}, fmt.Errorf("%w: reviewed guest or charge changed", ErrInvalidInnRequest)
	}
	if row.BlockReason != InnPickupBlockNone {
		return InnPickupResult{}, fmt.Errorf("%w: pickup block reason %d", ErrInnPickupUnavailable, row.BlockReason)
	}

	beforeRoster := g.CurrentPartyRosterSnapshot()
	result := InnPickupResult{
		Outcome:              InnPickupCompleted,
		MerchantID:           state.Merchant.ID,
		Character:            row.Identity,
		CharacterIndexBefore: row.RosterIndex,
		CharacterIndexAfter:  row.RosterIndex,
		MonthsAtInn:          row.MonthsAtInn,
		BilledMonths:         row.BilledMonths,
		MonthlyRate:          row.MonthlyRate,
		ListedCharge:         row.Charge,
		GoldBefore:           state.Gold,
		GoldAfterPayment:     state.Gold,
		GoldAfterTheft:       state.Gold,
		FinalGold:            state.Gold,
		Projection:           innProjectionFromSnapshot(beforeRoster.Characters[row.RosterIndex]),
		RosterBefore:         beforeRoster,
		RosterAfter:          beforeRoster,
		State:                state,
	}
	if !row.Affordable {
		result.Outcome = InnPickupRefusedWithGuards
		result.GuardActivation, err = g.ActivateGuards()
		if err != nil {
			return result, fmt.Errorf("activating guards for unpaid Inn bill: %w", err)
		}

		return result, nil
	}

	if !g.PartyState.Inventory.Gold.DecrementBy(row.Charge) {
		panic("validated Inn pickup could not deduct gold")
	}
	result.GoldAfterPayment = g.PartyState.Inventory.Gold.Get()
	result.Theft = g.ApplyFaulineiShopTheft()
	result.GoldAfterTheft = g.PartyState.Inventory.Gold.Get()
	result.CharacterIndexAfter = row.Projection.RosterIndexAfter
	result.Projection = row.Projection
	inserted := g.PartyState.PickUpCharacterFromInn(row.RosterIndex)
	if inserted != row.Projection.RosterIndexAfter {
		panic("validated Inn pickup inserted guest at unexpected active index")
	}
	changes := g.restoreInnPickupCharacter(g.PartyState.GetCharacter(inserted))
	g.publishTransientStatusChanges(changes)
	result.FinalGold = g.PartyState.Inventory.Gold.Get()
	result.RosterAfter = g.CurrentPartyRosterSnapshot()
	result.Projection = innProjectionResult(result.Projection, result.RosterAfter.Characters[inserted])
	result.State, err = g.GetInnShopState(session)
	if err != nil {
		panic("Inn session became invalid after its own pickup transfer: " + err.Error())
	}

	return result, nil
}

func (g *GameState) buildInnShopState(
	merchant references.MerchantDefinition,
	buyer *party_state.PlayerCharacter,
	buyerIndex int,
	pricingIntelligence int,
) (InnShopState, error) {
	inn, ok := g.GameReferences.MerchantReferences.Inn(merchant.ID)
	if !ok {
		return InnShopState{}, fmt.Errorf("%w: no Inn row for merchant %q", ErrInvalidInnRequest, merchant.ID)
	}
	partySize, err := g.validateContiguousActiveParty()
	if err != nil {
		return InnShopState{}, fmt.Errorf("%w: invalid packed party: %w", ErrInvalidInnRequest, err)
	}
	roster := g.CurrentPartyRosterSnapshot()
	state := InnShopState{
		Merchant:              merchant,
		PricingCharacterIndex: buyerIndex,
		PricingCharacterName:  buyer.GetNameAsString(),
		Gold:                  g.PartyState.Inventory.Gold.Get(),
		RoomCapacity:          inn.GuestCapacity,
		PartySize:             partySize,
		PartyCapacity:         party_state.MaxActiveParty,
		RosterFingerprint:     roster.Fingerprint,
		LeaveRows:             make([]InnLeaveRow, 0, partySize),
		GuestRows:             make([]InnGuestRow, 0, inn.GuestCapacity),
		pricingIntelligence:   pricingIntelligence,
	}
	for index := range party_state.NRosterSlots {
		if g.PartyState.HasCharacter(index) &&
			g.PartyState.GetCharacter(index).PartyMap == party_state.PartyMap(merchant.Location) {
			state.RoomOccupancy++
		}
	}
	state.Rest, err = g.buildInnRestQuote(state, inn, roster)
	if err != nil {
		return InnShopState{}, err
	}
	living := 0
	for index := range partySize {
		if !g.PartyState.GetCharacter(index).IsDead() {
			living++
		}
	}
	for index := range partySize {
		state.LeaveRows = append(state.LeaveRows, g.buildInnLeaveRow(state, roster.Characters[index], living))
	}
	monthlyRate := CalculateMerchantBuyPrice(inn.BasePrice*10, state.pricingIntelligence)
	if monthlyRate <= 0 || monthlyRate > math.MaxUint16/maximumInnMonths {
		return InnShopState{}, fmt.Errorf("%w: calculated monthly rate %d is invalid", ErrInvalidInnRequest, monthlyRate)
	}
	state.MonthlyRate = uint16(monthlyRate)
	for index := partySize; index < party_state.NRosterSlots; index++ {
		if !g.PartyState.HasCharacter(index) {
			continue
		}
		character := g.PartyState.GetCharacter(index)
		if character.PartyMap != party_state.PartyMap(merchant.Location) {
			continue
		}
		state.GuestRows = append(state.GuestRows, g.buildInnGuestRow(state, roster.Characters[index], monthlyRate))
	}

	return state, nil
}

func (g *GameState) buildInnRestQuote(
	state InnShopState,
	inn references.InnReference,
	roster PartyRosterSnapshot,
) (InnRestQuote, error) {
	price := CalculateMerchantBuyPrice(inn.BasePrice*state.PartySize, state.pricingIntelligence)
	if price <= 0 || price > math.MaxUint16 {
		return InnRestQuote{}, fmt.Errorf("%w: calculated nightly price %d is invalid", ErrInvalidInnRequest, price)
	}
	quote := InnRestQuote{
		Charge:            uint16(price),
		Affordable:        int(state.Gold) >= price,
		GoldBefore:        state.Gold,
		GoldAfterPayment:  state.Gold,
		SleepLocation:     inn.SleepLocation,
		WakeLocation:      inn.WakeLocation,
		DateTimeBefore:    g.DateTime,
		MemberProjections: make([]InnCharacterProjection, state.PartySize),
	}
	if quote.Affordable {
		quote.GoldAfterPayment -= quote.Charge
	}
	if state.RoomOccupancy >= state.RoomCapacity {
		quote.BlockReason = InnRestBlockNoVacancy
	}
	quote.TimeStepsMinutes, quote.DateTimeAfter = projectInnRestTime(g.DateTime)
	monthBoundaries := britannianMonthOrdinal(quote.DateTimeAfter) - britannianMonthOrdinal(quote.DateTimeBefore)
	for index := range state.PartySize {
		quote.MemberProjections[index] = projectInnRestCharacter(roster.Characters[index])
		quote.MemberProjections[index].MonthsAfter = projectInnMonths(
			quote.MemberProjections[index].MonthsAfter,
			monthBoundaries,
		)
		quote.Warnings = appendUniqueInnWarnings(quote.Warnings, quote.MemberProjections[index].Warnings...)
	}

	return quote, nil
}

func projectInnMonths(months byte, boundaries int) byte {
	for range max(boundaries, 0) {
		if months >= maximumInnMonths {
			break
		}
		months++
	}

	return months
}

func (g *GameState) buildInnLeaveRow(
	state InnShopState,
	character PartyRosterCharacterSnapshot,
	living int,
) InnLeaveRow {
	identity := innSnapshotIdentity(character)
	projection := innProjectionFromSnapshot(character)
	projection.RosterIndexAfter = party_state.NRosterSlots - 1
	projection.PartyMapAfter = party_state.PartyMap(state.Merchant.Location)
	projection.MonthsAfter = 0
	row := InnLeaveRow{
		Identity:       identity,
		RosterIndex:    character.RosterIndex,
		Status:         character.Status,
		Eligible:       true,
		Projection:     projection,
		PartySizeAfter: state.PartySize - 1,
		OccupancyAfter: state.RoomOccupancy + 1,
	}
	switch {
	case character.RosterIndex == party_state.AvatarCharacterIndex:
		row.Eligible, row.BlockReason = false, InnLeaveBlockAvatar
	case character.Status == party_state.Dead:
		row.Eligible, row.BlockReason = false, InnLeaveBlockDead
	case state.PartySize <= 1:
		row.Eligible, row.BlockReason = false, InnLeaveBlockMinimumParty
	case state.RoomOccupancy >= state.RoomCapacity:
		row.Eligible, row.BlockReason = false, InnLeaveBlockNoVacancy
	case living == 1:
		row.Warnings = []InnWarning{InnWarningLastLivingMember}
		row.Projection.Warnings = append([]InnWarning(nil), row.Warnings...)
	}
	if !row.Eligible {
		row.Projection = innProjectionFromSnapshot(character)
		row.PartySizeAfter = state.PartySize
		row.OccupancyAfter = state.RoomOccupancy
	}

	return row
}

func (g *GameState) buildInnGuestRow(
	state InnShopState,
	character PartyRosterCharacterSnapshot,
	monthlyRate int,
) InnGuestRow {
	billedMonths := max(int(character.MonthsAtInn), 1)
	charge := monthlyRate * billedMonths
	projection := projectInnPickupCharacter(character, state.PartySize)
	row := InnGuestRow{
		Identity:           innSnapshotIdentity(character),
		RosterIndex:        character.RosterIndex,
		Status:             character.Status,
		MonthsAtInn:        character.MonthsAtInn,
		BilledMonths:       billedMonths,
		MonthlyRate:        uint16(monthlyRate),
		Charge:             uint16(charge),
		Affordable:         int(state.Gold) >= charge,
		Confirmable:        true,
		Projection:         projection,
		GoldBefore:         state.Gold,
		GoldAfterPayment:   state.Gold,
		PartySizeAfter:     state.PartySize + 1,
		RoomOccupancyAfter: state.RoomOccupancy - 1,
	}
	if row.Affordable {
		row.GoldAfterPayment -= row.Charge
	}
	switch {
	case character.Status == party_state.Dead:
		// REDUX DEVIATION: MERCH-0011: original pickup does not refuse a Dead guest and
		// restores full HP while it stays Dead. Redux lists the guest but cannot collect
		// it, avoiding that result. See ref:MERCH-0011.
		row.BlockReason, row.Confirmable = InnPickupBlockDead, false
	case state.PartySize >= party_state.MaxActiveParty:
		row.BlockReason, row.Confirmable = InnPickupBlockPartyFull, false
	}
	if row.BlockReason != InnPickupBlockNone {
		row.Projection = innProjectionFromSnapshot(character)
		row.PartySizeAfter = state.PartySize
		row.RoomOccupancyAfter = state.RoomOccupancy
	}
	if character.MonthsAtInn == 0 {
		row.Warnings = append(row.Warnings, InnWarningMinimumMonthCharge)
	}
	row.Warnings = appendUniqueInnWarnings(row.Warnings, projection.Warnings...)
	if row.BlockReason == InnPickupBlockNone && !row.Affordable {
		row.Warnings = appendUniqueInnWarnings(row.Warnings, InnWarningInsufficientFundsActivatesGuards)
	}

	return row
}

func projectInnRestTime(start datetime.UltimaDate) ([]int, datetime.UltimaDate) {
	steps := make([]int, 0, 128)
	projected := start
	for range 12 {
		steps = append(steps, 5)
		projected.Advance(5)
	}
	for projected.Hour != 6 {
		steps = append(steps, 9)
		projected.Advance(9)
	}

	return steps, projected
}

func projectInnRestCharacter(character PartyRosterCharacterSnapshot) InnCharacterProjection {
	projection := innProjectionFromSnapshot(character)
	if character.Status == party_state.Dead {
		return projection
	}
	projection.HPAfter = character.MaximumHP
	projection.MPAfter = restoredInnMP(character.Class, character.Intelligence, character.CurrentMP)
	switch character.Status {
	case party_state.Poisoned:
		projection.StatusAfter = party_state.Dead
		projection.HPAfter = 0
		projection.Warnings = []InnWarning{InnWarningPoisonDeath}
	case party_state.Sleep, party_state.Good:
		projection.StatusAfter = party_state.Good
	case party_state.Charmed:
		// Charmed is restored but deliberately remains Charmed.
	case party_state.Dead:
		panic("dead Inn Rest projection passed the early return")
	}

	return projection
}

func projectInnPickupCharacter(
	character PartyRosterCharacterSnapshot,
	partySize int,
) InnCharacterProjection {
	projection := innProjectionFromSnapshot(character)
	projection.RosterIndexAfter = partySize
	projection.PartyMapAfter = party_state.PartyMapActive
	if character.Status == party_state.Poisoned {
		projection.StatusAfter = party_state.Dead
		projection.HPAfter = 0
		projection.Warnings = []InnWarning{InnWarningPoisonDeath}

		return projection
	}
	if character.Status != party_state.Dead {
		projection.HPAfter = character.MaximumHP
		projection.MPAfter = restoredInnMP(character.Class, character.Intelligence, character.CurrentMP)
	}

	return projection
}

func restoredInnMP(class party_state.CharacterClass, intelligence, current byte) byte {
	switch class {
	case party_state.Avatar, party_state.Wizard:
		return intelligence
	case party_state.Bard:
		return intelligence / 2
	case party_state.Fighter:
		return current
	default:
		return current
	}
}

func innProjectionFromSnapshot(character PartyRosterCharacterSnapshot) InnCharacterProjection {
	return InnCharacterProjection{
		Identity:          innSnapshotIdentity(character),
		RosterIndexBefore: character.RosterIndex,
		RosterIndexAfter:  character.RosterIndex,
		StatusBefore:      character.Status,
		StatusAfter:       character.Status,
		HPBefore:          character.CurrentHP,
		HPAfter:           character.CurrentHP,
		MaxHP:             character.MaximumHP,
		MPBefore:          character.CurrentMP,
		MPAfter:           character.CurrentMP,
		PartyMapBefore:    character.PartyMap,
		PartyMapAfter:     character.PartyMap,
		MonthsBefore:      character.MonthsAtInn,
		MonthsAfter:       character.MonthsAtInn,
	}
}

func innCharacterIdentity(character *party_state.PlayerCharacter) InnCharacterIdentity {
	return InnCharacterIdentity{Name: character.GetNameAsString(), Gender: character.Gender, Class: character.Class}
}

func innSnapshotIdentity(character PartyRosterCharacterSnapshot) InnCharacterIdentity {
	return InnCharacterIdentity{Name: character.Name, Gender: character.Gender, Class: character.Class}
}

func innLeaveRowAt(rows []InnLeaveRow, index int, identity InnCharacterIdentity) (InnLeaveRow, bool) {
	for _, row := range rows {
		if row.RosterIndex == index && row.Identity == identity {
			return row, true
		}
	}

	return InnLeaveRow{}, false
}

func innGuestRowAt(rows []InnGuestRow, index int, identity InnCharacterIdentity) (InnGuestRow, bool) {
	for _, row := range rows {
		if row.RosterIndex == index && row.Identity == identity {
			return row, true
		}
	}

	return InnGuestRow{}, false
}

func appendUniqueInnWarnings(warnings []InnWarning, additions ...InnWarning) []InnWarning {
	for _, addition := range additions {
		if addition == InnWarningNone {
			continue
		}
		found := false
		for _, warning := range warnings {
			if warning == addition {
				found = true

				break
			}
		}
		if !found {
			warnings = append(warnings, addition)
		}
	}

	return warnings
}

func (g *GameState) innRegenerationResults(sweep []RegenerationRingResult) []InnRegenerationResult {
	results := make([]InnRegenerationResult, 0, len(sweep))
	for _, regeneration := range sweep {
		if !regeneration.RollSucceeded {
			continue
		}
		results = append(results, InnRegenerationResult{
			Character:   innCharacterIdentity(g.PartyState.GetCharacter(regeneration.CharacterIndex)),
			RosterIndex: regeneration.CharacterIndex,
			HPBefore:    regeneration.HPBefore,
			HPAfter:     regeneration.HPAfter,
		})
	}

	return results
}

func (g *GameState) restoreInnMorningParty(partySize int) transientStatusChanges {
	changes := transientStatusChanges{}
	for index := range partySize {
		character := g.PartyState.GetCharacter(index)
		status := character.GetStatus()
		if status == party_state.Dead {
			continue
		}
		hpBefore := character.GetCurrentHp()
		character.SetHp(character.GetMaxHp())
		character.RestoreMpByClass()
		switch status {
		case party_state.Poisoned:
			character.SetHp(0)
			character.SetStatus(party_state.Dead)
		case party_state.Sleep:
			character.SetStatus(party_state.Good)
		case party_state.Good, party_state.Charmed:
			character.SetStatus(status)
		case party_state.Dead:
			panic("dead Inn guest passed the morning early return")
		}
		changes.observeHealing(hpBefore, character)
	}

	return changes
}

func (g *GameState) restoreInnPickupCharacter(character *party_state.PlayerCharacter) transientStatusChanges {
	changes := transientStatusChanges{}
	status := character.GetStatus()
	if status == party_state.Poisoned {
		character.SetHp(0)
		character.SetStatus(party_state.Dead)

		return changes
	}
	hpBefore := character.GetCurrentHp()
	character.SetHp(character.GetMaxHp())
	character.RestoreMpByClass()
	character.SetStatus(status)
	changes.observeHealing(hpBefore, character)

	return changes
}

func innProjectionResults(
	projections []InnCharacterProjection,
	after PartyRosterSnapshot,
) []InnCharacterProjection {
	results := append([]InnCharacterProjection(nil), projections...)
	for index := range results {
		results[index] = innProjectionResult(results[index], after.Characters[results[index].RosterIndexAfter])
	}

	return results
}

func innProjectionResult(
	projection InnCharacterProjection,
	after PartyRosterCharacterSnapshot,
) InnCharacterProjection {
	projection.StatusAfter = after.Status
	projection.HPAfter = after.CurrentHP
	projection.MPAfter = after.CurrentMP
	projection.PartyMapAfter = after.PartyMap
	projection.MonthsAfter = after.MonthsAtInn

	return projection
}
