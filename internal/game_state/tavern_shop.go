package game_state

import (
	"errors"
	"fmt"
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

var (
	// ErrInvalidTavernRequest indicates stale, tampered, or unsupported reviewed input.
	ErrInvalidTavernRequest = errors.New("invalid tavern request")
	// ErrTavernInformationLocked indicates that the active session has not established a customer.
	ErrTavernInformationLocked = errors.New("tavern information requires a customer")
	// ErrTavernInformationTopicUnknown identifies blank or unmatched information input.
	ErrTavernInformationTopicUnknown = errors.New("unknown tavern information topic")
)

const (
	// TavernOrdinaryDrinkWineIndex distinguishes ordinary drinks from the canonical wine list.
	TavernOrdinaryDrinkWineIndex = -1
	maximumTavernProvisionPacks  = 99
)

// TavernMealOutcome identifies a completed meal or terminal payment refusal.
type TavernMealOutcome byte

const (
	// TavernMealOutcomeUnknown is the zero value before a terminal decision is assigned.
	TavernMealOutcomeUnknown TavernMealOutcome = iota
	// TavernMealCompleted indicates payment, theft, food, and customer effects committed.
	TavernMealCompleted
	// TavernMealInsufficientFunds indicates a terminal refusal with no effects.
	TavernMealInsufficientFunds
)

// TavernDrinkApproval distinguishes an ordinary purchase from the warned Continue decision.
type TavernDrinkApproval byte

const (
	// TavernDrinkApprovalPurchase is valid when no fourth-drink warning applies.
	TavernDrinkApprovalPurchase TavernDrinkApproval = iota
	// TavernDrinkApprovalContinue explicitly accepts a fourth drink despite the warning.
	TavernDrinkApprovalContinue
	// TavernDrinkApprovalCancel abandons the selected drink without effects.
	TavernDrinkApprovalCancel
)

// TavernDrinkOutcome identifies all terminal drink decisions.
type TavernDrinkOutcome byte

const (
	// TavernDrinkOutcomeUnknown is the zero value before a terminal decision is assigned.
	TavernDrinkOutcomeUnknown TavernDrinkOutcome = iota
	// TavernDrinkPurchased indicates all drink effects committed.
	TavernDrinkPurchased
	// TavernDrinkCancelled indicates explicit cancellation without effects.
	TavernDrinkCancelled
	// TavernDrinkInsufficientFunds indicates a terminal refusal without effects.
	TavernDrinkInsufficientFunds
)

// TavernStopOutcome identifies acceptance of the no-payment warning branch.
type TavernStopOutcome byte

const (
	// TavernStopOutcomeUnknown is the zero value before Stop is accepted.
	TavernStopOutcomeUnknown TavernStopOutcome = iota
	// TavernStopAccepted indicates customer eligibility was established without payment.
	TavernStopAccepted
)

// TavernProvisionOutcome identifies faithful per-pack completion boundaries.
type TavernProvisionOutcome byte

const (
	// TavernProvisionOutcomeUnknown is the zero value before a terminal boundary is assigned.
	TavernProvisionOutcomeUnknown TavernProvisionOutcome = iota
	// TavernProvisionCompleted indicates every requested pack committed.
	TavernProvisionCompleted
	// TavernProvisionPartial indicates at least one pack committed before later nonpayment.
	TavernProvisionPartial
	// TavernProvisionSaturated indicates fulfillment stopped when food reached 9999.
	TavernProvisionSaturated
	// TavernProvisionCharityEjection indicates low-food charity followed by ejection.
	TavernProvisionCharityEjection
	// TavernProvisionEjection indicates unaffordable provisions with no need for charity.
	TavernProvisionEjection
)

// TavernInformationOutcome identifies paid lore completion or terminal nonpayment.
type TavernInformationOutcome byte

const (
	// TavernInformationOutcomeUnknown is the zero value before a terminal decision is assigned.
	TavernInformationOutcomeUnknown TavernInformationOutcome = iota
	// TavernInformationCompleted indicates payment and response selection committed.
	TavernInformationCompleted
	// TavernInformationInsufficientFunds indicates terminal nonpayment without effects.
	TavernInformationInsufficientFunds
)

// TavernMealQuote is the fixed per-diner meal charge against current authority.
type TavernMealQuote struct {
	Meal             references.TavernMeal
	DinerCount       int
	Charge           uint16
	Affordable       bool
	GoldBefore       uint16
	GoldAfterPayment uint16
	FoodBefore       uint16
}

// TavernDrinkQuote is one ordinary drink or canonical wine against current authority.
type TavernDrinkQuote struct {
	Drink            references.TavernDrink
	WineIndex        int
	Wine             references.TavernWine
	DinerCount       int
	Charge           uint16
	Affordable       bool
	Warning          bool
	StopAvailable    bool
	GoldBefore       uint16
	GoldAfterPayment uint16
	Drunk            int
	Smashed          int
	Karma            int
}

// TavernStopQuote describes the no-payment warning branch available at exactly drunk three.
type TavernStopQuote struct {
	MerchantID       references.MerchantID
	Drunk            int
	CustomerEligible bool
}

// TavernProvisionQuote is the canonical per-pack price captured for one session.
type TavernProvisionQuote struct {
	Provision          references.TavernProvision
	PricePerPack       int
	GoldBefore         uint16
	FoodBefore         uint16
	MaximumUsefulPacks int
	Approvals          []TavernProvisionApproval
}

// TavernProvisionApproval is one deterministic pre-commit projection. Charity
// remains explicitly uncertain because its 1-2 food grant consumes gameplay RNG.
type TavernProvisionApproval struct {
	PackCount          int
	PurchasedPacks     int
	Outcome            TavernProvisionOutcome
	TotalCost          int
	GoldAfterPurchases uint16
	FoodAfter          uint16
	ActualFood         uint16
	CharityPossible    bool
}

// HasValidApprovals reports whether every detached projection exactly matches
// this quote's authoritative deterministic provision rules.
func (q TavernProvisionQuote) HasValidApprovals() bool {
	if !q.Provision.Available || q.Provision.PackQuantity == 0 || q.PricePerPack <= 0 ||
		q.FoodBefore > party_state.ItemQuantityLargeMax {
		return false
	}
	expectedMaximum := maximumUsefulTavernProvisionPacks(q)
	if q.MaximumUsefulPacks != expectedMaximum || len(q.Approvals) != expectedMaximum {
		return false
	}
	for packCount := 1; packCount <= expectedMaximum; packCount++ {
		if q.Approvals[packCount-1] != projectTavernProvisionPurchase(q, packCount) {
			return false
		}
	}

	return true
}

// TavernInformationQuote is one matched fixed-price topic.
type TavernInformationQuote struct {
	Topic            references.TavernInformationTopic
	Price            int
	Affordable       bool
	GoldBefore       uint16
	GoldAfterPayment uint16
}

// TavernShopState is a trusted detached snapshot of one Tavern session.
type TavernShopState struct {
	Merchant              references.MerchantDefinition
	PricingCharacterIndex int
	PricingCharacterName  string
	PricingIntelligence   int
	Gold                  uint16
	Food                  uint16
	DinerCount            int
	CustomerEligible      bool
	Drunk                 int
	Smashed               int
	Karma                 int
	Menu                  references.TavernMenu
	Meal                  TavernMealQuote
	Drink                 TavernDrinkQuote
	Wines                 []TavernDrinkQuote
	Provision             TavernProvisionQuote
}

// TavernShopReview is a pure production snapshot with canonical sprite identity.
type TavernShopReview = MerchantShopReview[TavernShopState]

// TavernMealRequest carries comparison-only reviewed meal authority.
type TavernMealRequest struct {
	ReviewedMerchantID references.MerchantID
	ReviewedMeal       references.TavernMeal
	ReviewedDinerCount int
	ReviewedCharge     uint16
	ReviewedGoldBefore uint16
	ReviewedFoodBefore uint16
}

// TavernDrinkRequest carries comparison-only reviewed drink authority and approval.
type TavernDrinkRequest struct {
	ReviewedMerchantID references.MerchantID
	ReviewedDrink      references.TavernDrink
	ReviewedWineIndex  int
	ReviewedWine       references.TavernWine
	ReviewedDinerCount int
	ReviewedCharge     uint16
	ReviewedGoldBefore uint16
	ReviewedDrunk      int
	ReviewedSmashed    int
	ReviewedKarma      int
	Approval           TavernDrinkApproval
}

// TavernStopRequest carries comparison-only warning state.
type TavernStopRequest struct {
	ReviewedMerchantID references.MerchantID
	ReviewedDrunk      int
}

// TavernProvisionRequest carries comparison-only pack price and inventory authority.
type TavernProvisionRequest struct {
	ReviewedMerchantID references.MerchantID
	ReviewedProvision  references.TavernProvision
	ReviewedPrice      int
	ReviewedGoldBefore uint16
	ReviewedFoodBefore uint16
	PackCount          int
}

// TavernInformationRequest carries only the reviewed merchant/topic/price/gold comparison.
type TavernInformationRequest struct {
	ReviewedMerchantID references.MerchantID
	ReviewedTopic      references.TavernInformationTopic
	ReviewedPrice      int
	ReviewedGoldBefore uint16
}

// TavernMealTableVisual tells presentation which temporary table-food sprite to show.
type TavernMealTableVisual struct {
	Show        bool
	Position    references.Position
	SpriteIndex indexes.SpriteIndex
}

// TavernMealResult reports one complete meal decision and transient presentation signal.
type TavernMealResult struct {
	Outcome              TavernMealOutcome
	MerchantID           references.MerchantID
	Meal                 references.TavernMeal
	DinerCount           int
	Charge               uint16
	GoldBefore           uint16
	GoldAfterPayment     uint16
	Theft                uint16
	GoldAfterTheft       uint16
	FoodBefore           uint16
	FoodAfter            uint16
	ActualFood           uint16
	CustomerBefore       bool
	CustomerAfter        bool
	TransientTableVisual TavernMealTableVisual
}

// TavernDrinkResult reports all economic, intoxication, karma, and eligibility fields.
type TavernDrinkResult struct {
	Outcome          TavernDrinkOutcome
	MerchantID       references.MerchantID
	Drink            references.TavernDrink
	WineIndex        int
	Wine             references.TavernWine
	DinerCount       int
	Charge           uint16
	Warning          bool
	GoldBefore       uint16
	GoldAfterPayment uint16
	Theft            uint16
	GoldAfterTheft   uint16
	DrunkBefore      int
	DrunkAfter       int
	SmashedBefore    int
	SmashedAfter     int
	KarmaBefore      int
	KarmaAfter       int
	CustomerBefore   bool
	CustomerAfter    bool
}

// TavernStopResult reports the no-payment fourth-drink warning branch.
type TavernStopResult struct {
	Outcome        TavernStopOutcome
	MerchantID     references.MerchantID
	GoldBefore     uint16
	GoldAfter      uint16
	DrunkBefore    int
	DrunkAfter     int
	SmashedBefore  int
	SmashedAfter   int
	KarmaBefore    int
	KarmaAfter     int
	CustomerBefore bool
	CustomerAfter  bool
}

// TavernProvisionResult reports faithful per-pack purchases, charity, and ejection.
type TavernProvisionResult struct {
	Outcome            TavernProvisionOutcome
	MerchantID         references.MerchantID
	Provision          references.TavernProvision
	PricePerPack       int
	RequestedPacks     int
	PurchasedPacks     int
	OfferedFood        uint16
	ActualFood         uint16
	TotalCost          int
	GoldBefore         uint16
	GoldAfterPurchases uint16
	Theft              uint16
	GoldAfterTheft     uint16
	FoodBefore         uint16
	FoodAfter          uint16
	CustomerBefore     bool
	CustomerAfter      bool
}

// TavernInformationResult reports one paid response selection.
type TavernInformationResult struct {
	Outcome          TavernInformationOutcome
	MerchantID       references.MerchantID
	Topic            references.TavernInformationTopic
	Price            int
	GoldBefore       uint16
	GoldAfterPayment uint16
	Informant        string
	Location         string
	CustomerBefore   bool
	CustomerAfter    bool
}

// GetTavernShopState returns the complete current Tavern state using captured Intelligence.
func (g *GameState) GetTavernShopState(session MerchantSession) (TavernShopState, error) {
	buyer, merchant, err := g.validateMerchantSession(session, references.MerchantFamilyTavern)
	if err != nil {
		return TavernShopState{}, err
	}

	return g.buildTavernShopState(
		merchant,
		buyer,
		session.buyerIndex,
		session.buyerIntelligence,
		g.tavernCustomerSessionSequence == session.sequence,
	)
}

// ReviewTavernShopByMerchantID builds production quotes without a session, mutation, callbacks, or RNG.
func (g *GameState) ReviewTavernShopByMerchantID(id references.MerchantID) (TavernShopReview, error) {
	return reviewMerchantShop(g, id, references.MerchantFamilyTavern, ErrInvalidTavernRequest, "a Tavern", func(
		merchant references.MerchantDefinition, buyer *party_state.PlayerCharacter, buyerIndex int,
	) (TavernShopState, error) {
		return g.buildTavernShopState(merchant, buyer, buyerIndex, int(buyer.Intelligence), false)
	})
}

// PurchaseTavernMeal revalidates and commits one meal order.
func (g *GameState) PurchaseTavernMeal(
	session MerchantSession,
	request TavernMealRequest,
) (TavernMealResult, error) {
	state, err := g.GetTavernShopState(session)
	if err != nil {
		return TavernMealResult{}, err
	}
	quote := state.Meal
	if request.ReviewedMerchantID != state.Merchant.ID || request.ReviewedMeal != quote.Meal ||
		request.ReviewedDinerCount != quote.DinerCount || request.ReviewedCharge != quote.Charge ||
		request.ReviewedGoldBefore != quote.GoldBefore || request.ReviewedFoodBefore != quote.FoodBefore {
		return TavernMealResult{}, fmt.Errorf(
			"%w: reviewed merchant, meal, diners, charge, gold, or food changed",
			ErrInvalidTavernRequest,
		)
	}

	result := TavernMealResult{
		Outcome:          TavernMealInsufficientFunds,
		MerchantID:       state.Merchant.ID,
		Meal:             quote.Meal,
		DinerCount:       quote.DinerCount,
		Charge:           quote.Charge,
		GoldBefore:       quote.GoldBefore,
		GoldAfterPayment: quote.GoldBefore,
		GoldAfterTheft:   quote.GoldBefore,
		FoodBefore:       quote.FoodBefore,
		FoodAfter:        quote.FoodBefore,
		CustomerBefore:   state.CustomerEligible,
		CustomerAfter:    state.CustomerEligible,
	}
	if !quote.Affordable {
		return result, nil
	}
	if !g.PartyState.Inventory.Gold.DecrementBy(quote.Charge) {
		panic("validated Tavern meal could not deduct gold")
	}
	result.GoldAfterPayment = g.PartyState.Inventory.Gold.Get()
	result.Theft = g.ApplyFaulineiShopTheft()
	result.GoldAfterTheft = g.PartyState.Inventory.Gold.Get()
	g.PartyState.Inventory.Provisions.Food.IncrementBy(uint16(quote.DinerCount))
	result.FoodAfter = g.PartyState.Inventory.Provisions.Food.Get()
	result.ActualFood = result.FoodAfter - result.FoodBefore
	// REDUX DEVIATION: meal payment and plating permanently replaces an adjacent TABLE
	// tile with FOOD_DOWN or FOOD_UP. Redux returns the same decided visual as a
	// transient presentation signal so native map state remains unchanged.
	// See ref:MERCH-0013.
	result.TransientTableVisual = g.tavernMealTableVisual()
	g.tavernCustomerSessionSequence = session.sequence
	result.CustomerAfter = true
	result.Outcome = TavernMealCompleted

	return result, nil
}

// PurchaseTavernDrink revalidates and commits an ordinary drink or wine decision.
func (g *GameState) PurchaseTavernDrink(
	session MerchantSession,
	request TavernDrinkRequest,
) (TavernDrinkResult, error) {
	state, err := g.GetTavernShopState(session)
	if err != nil {
		return TavernDrinkResult{}, err
	}
	var quote TavernDrinkQuote
	switch {
	case request.ReviewedWineIndex == TavernOrdinaryDrinkWineIndex:
		if state.Menu.Drink.IsWineList {
			return TavernDrinkResult{}, fmt.Errorf("%w: ordinary drink is unavailable", ErrInvalidTavernRequest)
		}
		quote = state.Drink
	case !state.Menu.Drink.IsWineList || request.ReviewedWineIndex < 0 ||
		request.ReviewedWineIndex >= len(state.Wines):
		return TavernDrinkResult{}, fmt.Errorf(
			"%w: wine index %d is unavailable",
			ErrInvalidTavernRequest,
			request.ReviewedWineIndex,
		)
	default:
		quote = state.Wines[request.ReviewedWineIndex]
	}
	if request.ReviewedMerchantID != state.Merchant.ID || request.ReviewedDrink != quote.Drink ||
		request.ReviewedWineIndex != quote.WineIndex || request.ReviewedWine != quote.Wine ||
		request.ReviewedDinerCount != quote.DinerCount || request.ReviewedCharge != quote.Charge ||
		request.ReviewedGoldBefore != quote.GoldBefore || request.ReviewedDrunk != quote.Drunk ||
		request.ReviewedSmashed != quote.Smashed || request.ReviewedKarma != quote.Karma {
		return TavernDrinkResult{}, fmt.Errorf(
			"%w: reviewed merchant, drink, price, gold, or intoxication changed",
			ErrInvalidTavernRequest,
		)
	}
	if request.Approval > TavernDrinkApprovalCancel {
		return TavernDrinkResult{}, fmt.Errorf("%w: unsupported drink approval %d", ErrInvalidTavernRequest, request.Approval)
	}
	if quote.Warning && request.Approval == TavernDrinkApprovalPurchase {
		return TavernDrinkResult{}, fmt.Errorf("%w: warned purchase requires explicit Continue", ErrInvalidTavernRequest)
	}
	if !quote.Warning && request.Approval == TavernDrinkApprovalContinue {
		return TavernDrinkResult{}, fmt.Errorf("%w: Continue is valid only for the warning", ErrInvalidTavernRequest)
	}

	result := newTavernDrinkResult(state, quote)
	if request.Approval == TavernDrinkApprovalCancel {
		result.Outcome = TavernDrinkCancelled

		return result, nil
	}
	if !quote.Affordable {
		result.Outcome = TavernDrinkInsufficientFunds

		return result, nil
	}

	// REDUX DEVIATION: KARMA-0014: alcohol purchase sets smashed and reduces karma
	// before attempting the warned purchase. Redux first revalidates the canonical
	// request and affordability, then commits payment -> warning effects ->
	// Faulinei -> drunk so cancellation and failed payment are fully atomic.
	// See ref:KARMA-0014, ref:MERCH-0013.
	if !g.PartyState.Inventory.Gold.DecrementBy(quote.Charge) {
		panic("validated Tavern drink could not deduct gold")
	}
	result.GoldAfterPayment = g.PartyState.Inventory.Gold.Get()
	if quote.Warning {
		g.smashed = 25
		g.AdjustPartyKarma(-1)
	}
	result.Theft = g.ApplyFaulineiShopTheft()
	result.GoldAfterTheft = g.PartyState.Inventory.Gold.Get()
	g.drunk++
	g.tavernCustomerSessionSequence = session.sequence
	result.Outcome = TavernDrinkPurchased
	result.DrunkAfter = g.drunk
	result.SmashedAfter = g.smashed
	result.KarmaAfter = g.PartyState.Karma.Value
	result.CustomerAfter = true

	return result, nil
}

// QuoteTavernStop returns the no-payment warning branch available at exactly drunk three.
func (g *GameState) QuoteTavernStop(session MerchantSession) (TavernStopQuote, error) {
	state, err := g.GetTavernShopState(session)
	if err != nil {
		return TavernStopQuote{}, err
	}
	if state.Drunk != 3 {
		return TavernStopQuote{}, fmt.Errorf("%w: Stop requires drunk 3, got %d", ErrInvalidTavernRequest, state.Drunk)
	}

	return TavernStopQuote{
		MerchantID: state.Merchant.ID, Drunk: state.Drunk, CustomerEligible: state.CustomerEligible,
	}, nil
}

// StopTavernDrinking accepts the warning without payment or intoxication changes.
func (g *GameState) StopTavernDrinking(
	session MerchantSession,
	request TavernStopRequest,
) (TavernStopResult, error) {
	quote, err := g.QuoteTavernStop(session)
	if err != nil {
		return TavernStopResult{}, err
	}
	if request.ReviewedMerchantID != quote.MerchantID || request.ReviewedDrunk != quote.Drunk {
		return TavernStopResult{}, fmt.Errorf("%w: reviewed Stop state changed", ErrInvalidTavernRequest)
	}
	gold := g.PartyState.Inventory.Gold.Get()
	result := TavernStopResult{
		Outcome:        TavernStopAccepted,
		MerchantID:     quote.MerchantID,
		GoldBefore:     gold,
		GoldAfter:      gold,
		DrunkBefore:    g.drunk,
		DrunkAfter:     g.drunk,
		SmashedBefore:  g.smashed,
		SmashedAfter:   g.smashed,
		KarmaBefore:    g.PartyState.Karma.Value,
		KarmaAfter:     g.PartyState.Karma.Value,
		CustomerBefore: g.tavernCustomerSessionSequence == session.sequence,
		CustomerAfter:  true,
	}
	g.tavernCustomerSessionSequence = session.sequence

	return result, nil
}

// PurchaseTavernProvisions preserves the original independently committed per-pack loop.
func (g *GameState) PurchaseTavernProvisions(
	session MerchantSession,
	request TavernProvisionRequest,
) (TavernProvisionResult, error) {
	state, err := g.GetTavernShopState(session)
	if err != nil {
		return TavernProvisionResult{}, err
	}
	if !state.Menu.Provision.Available {
		return TavernProvisionResult{}, fmt.Errorf("%w: provisions are unavailable", ErrInvalidTavernRequest)
	}
	quote := state.Provision
	if request.PackCount < 1 || request.PackCount > maximumTavernProvisionPacks {
		return TavernProvisionResult{}, fmt.Errorf(
			"%w: provision pack count %d must be between 1 and %d",
			ErrInvalidTavernRequest,
			request.PackCount,
			maximumTavernProvisionPacks,
		)
	}
	if request.ReviewedMerchantID != state.Merchant.ID || request.ReviewedProvision != quote.Provision ||
		request.ReviewedPrice != quote.PricePerPack || request.ReviewedGoldBefore != quote.GoldBefore ||
		request.ReviewedFoodBefore != quote.FoodBefore {
		return TavernProvisionResult{}, fmt.Errorf(
			"%w: reviewed merchant, provision, price, gold, or food changed",
			ErrInvalidTavernRequest,
		)
	}

	result := TavernProvisionResult{
		MerchantID:         state.Merchant.ID,
		Provision:          quote.Provision,
		PricePerPack:       quote.PricePerPack,
		RequestedPacks:     request.PackCount,
		GoldBefore:         quote.GoldBefore,
		GoldAfterPurchases: quote.GoldBefore,
		GoldAfterTheft:     quote.GoldBefore,
		FoodBefore:         quote.FoodBefore,
		FoodAfter:          quote.FoodBefore,
		CustomerBefore:     state.CustomerEligible,
		CustomerAfter:      state.CustomerEligible,
	}
	price := uint16(quote.PricePerPack)
	for range request.PackCount {
		if !g.PartyState.Inventory.Gold.DecrementBy(price) {
			if result.PurchasedPacks > 0 {
				result.Outcome = TavernProvisionPartial
				result.GoldAfterTheft = result.GoldAfterPurchases
				g.tavernCustomerSessionSequence = session.sequence
				result.CustomerAfter = true

				return result, nil
			}
			if g.PartyState.Inventory.Provisions.Food.Get() < 3 {
				charity := uint16(g.RandomIntInRange(0, 1) + 1)
				g.PartyState.Inventory.Provisions.Food.IncrementBy(charity)
				result.FoodAfter = g.PartyState.Inventory.Provisions.Food.Get()
				result.ActualFood = result.FoodAfter - result.FoodBefore
				result.Outcome = TavernProvisionCharityEjection

				return result, nil
			}
			result.Outcome = TavernProvisionEjection

			return result, nil
		}

		result.PurchasedPacks++
		result.OfferedFood += quote.Provision.PackQuantity
		result.TotalCost += quote.PricePerPack
		result.GoldAfterPurchases = g.PartyState.Inventory.Gold.Get()
		g.PartyState.Inventory.Provisions.Food.IncrementBy(quote.Provision.PackQuantity)
		result.FoodAfter = g.PartyState.Inventory.Provisions.Food.Get()
		result.ActualFood = result.FoodAfter - result.FoodBefore
		if result.FoodAfter == party_state.ItemQuantityLargeMax {
			result.Outcome = TavernProvisionSaturated

			break
		}
	}
	if result.Outcome == TavernProvisionOutcomeUnknown {
		result.Outcome = TavernProvisionCompleted
	}
	result.Theft = g.ApplyFaulineiShopTheft()
	result.GoldAfterTheft = g.PartyState.Inventory.Gold.Get()
	g.tavernCustomerSessionSequence = session.sequence
	result.CustomerAfter = true

	return result, nil
}

// QuoteTavernInformation matches one authoritative topic for an eligible customer.
func (g *GameState) QuoteTavernInformation(
	session MerchantSession,
	input string,
) (TavernInformationQuote, error) {
	state, err := g.GetTavernShopState(session)
	if err != nil {
		return TavernInformationQuote{}, err
	}

	return g.quoteTavernInformation(state, input)
}

// ReviewTavernInformationByMerchantID matches one information quote without a
// session, mutation, callbacks, or RNG so Merchant Tester Preview can exercise
// the production topic and approval path.
func (g *GameState) ReviewTavernInformationByMerchantID(
	id references.MerchantID,
	input string,
) (TavernInformationQuote, error) {
	review, err := g.ReviewTavernShopByMerchantID(id)
	if err != nil {
		return TavernInformationQuote{}, err
	}
	review.State.CustomerEligible = true

	return g.quoteTavernInformation(review.State, input)
}

func (g *GameState) quoteTavernInformation(
	state TavernShopState,
	input string,
) (TavernInformationQuote, error) {
	if !state.CustomerEligible {
		return TavernInformationQuote{}, ErrTavernInformationLocked
	}
	topic, ok := g.GameReferences.MerchantReferences.MatchTavernInformationTopic(input)
	if !ok {
		return TavernInformationQuote{}, fmt.Errorf("%w: no canonical topic matches input", ErrTavernInformationTopicUnknown)
	}
	quote := TavernInformationQuote{
		Topic:            topic,
		Price:            topic.Price,
		Affordable:       int(state.Gold) >= topic.Price,
		GoldBefore:       state.Gold,
		GoldAfterPayment: state.Gold,
	}
	if quote.Affordable {
		quote.GoldAfterPayment -= uint16(topic.Price)
	}

	return quote, nil
}

// PurchaseTavernInformation commits one fixed-price response without Faulinei.
func (g *GameState) PurchaseTavernInformation(
	session MerchantSession,
	request TavernInformationRequest,
) (TavernInformationResult, error) {
	state, err := g.GetTavernShopState(session)
	if err != nil {
		return TavernInformationResult{}, err
	}
	if !state.CustomerEligible {
		return TavernInformationResult{}, ErrTavernInformationLocked
	}
	canonicalTopic, ok := g.GameReferences.MerchantReferences.MatchTavernInformationTopic(request.ReviewedTopic.Key)
	if !ok || canonicalTopic != request.ReviewedTopic || request.ReviewedMerchantID != state.Merchant.ID ||
		request.ReviewedPrice != canonicalTopic.Price || request.ReviewedGoldBefore != state.Gold {
		return TavernInformationResult{}, fmt.Errorf(
			"%w: reviewed merchant, topic, price, or gold changed",
			ErrInvalidTavernRequest,
		)
	}

	result := TavernInformationResult{
		Outcome:          TavernInformationInsufficientFunds,
		MerchantID:       state.Merchant.ID,
		Topic:            canonicalTopic,
		Price:            canonicalTopic.Price,
		GoldBefore:       state.Gold,
		GoldAfterPayment: state.Gold,
		Informant:        canonicalTopic.Informant,
		Location:         canonicalTopic.Location,
		CustomerBefore:   state.CustomerEligible,
		CustomerAfter:    state.CustomerEligible,
	}
	if int(state.Gold) < canonicalTopic.Price {
		return result, nil
	}
	price := uint16(canonicalTopic.Price)
	if !g.PartyState.Inventory.Gold.DecrementBy(price) {
		panic("validated Tavern information purchase could not deduct gold")
	}
	result.GoldAfterPayment = g.PartyState.Inventory.Gold.Get()
	g.tavernCustomerSessionSequence = session.sequence
	result.CustomerAfter = true
	result.Outcome = TavernInformationCompleted

	return result, nil
}

func (g *GameState) buildTavernShopState(
	merchant references.MerchantDefinition,
	buyer *party_state.PlayerCharacter,
	buyerIndex int,
	pricingIntelligence int,
	customerEligible bool,
) (TavernShopState, error) {
	menu, ok := g.GameReferences.MerchantReferences.TavernMenu(merchant.ID)
	if !ok {
		return TavernShopState{}, fmt.Errorf("%w: no Tavern menu for merchant %q", ErrInvalidTavernRequest, merchant.ID)
	}
	if menu.Meal.BasePrice <= 0 || menu.Meal.Name == "" || menu.Drink.Name == "" {
		return TavernShopState{}, fmt.Errorf("%w: incomplete Tavern menu for merchant %q", ErrInvalidTavernRequest, merchant.ID)
	}

	state := TavernShopState{
		Merchant:              merchant,
		PricingCharacterIndex: buyerIndex,
		PricingCharacterName:  buyer.GetNameAsString(),
		PricingIntelligence:   pricingIntelligence,
		Gold:                  g.PartyState.Inventory.Gold.Get(),
		Food:                  g.PartyState.Inventory.Provisions.Food.Get(),
		DinerCount:            g.tavernDinerCount(),
		CustomerEligible:      customerEligible,
		Drunk:                 g.drunk,
		Smashed:               g.smashed,
		Karma:                 g.PartyState.Karma.Value,
		Menu:                  menu,
	}
	charge := menu.Meal.BasePrice * state.DinerCount
	if charge <= 0 || charge > math.MaxUint16 {
		return TavernShopState{}, fmt.Errorf("%w: calculated meal charge %d is invalid", ErrInvalidTavernRequest, charge)
	}
	state.Meal = TavernMealQuote{
		Meal:             menu.Meal,
		DinerCount:       state.DinerCount,
		Charge:           uint16(charge),
		Affordable:       int(state.Gold) >= charge,
		GoldBefore:       state.Gold,
		GoldAfterPayment: state.Gold,
		FoodBefore:       state.Food,
	}
	if state.Meal.Affordable {
		state.Meal.GoldAfterPayment -= state.Meal.Charge
	}
	if menu.Drink.IsWineList {
		wines := g.GameReferences.MerchantReferences.TavernWines()
		state.Wines = make([]TavernDrinkQuote, len(wines))
		for index, wine := range wines {
			state.Wines[index] = g.buildTavernDrinkQuote(state, index, wine)
		}
	} else {
		if menu.Drink.UnitPrice <= 0 {
			return TavernShopState{}, fmt.Errorf("%w: ordinary drink price is invalid", ErrInvalidTavernRequest)
		}
		state.Drink = g.buildTavernDrinkQuote(state, TavernOrdinaryDrinkWineIndex, references.TavernWine{})
	}
	state.Provision = TavernProvisionQuote{
		Provision:  menu.Provision,
		GoldBefore: state.Gold,
		FoodBefore: state.Food,
	}
	if menu.Provision.Available {
		price := CalculateMerchantBuyPrice(menu.Provision.BasePrice, pricingIntelligence)
		if menu.Provision.PackQuantity == 0 || menu.Provision.BasePrice <= 0 || price <= 0 || price > math.MaxUint16 {
			return TavernShopState{}, fmt.Errorf("%w: calculated provision price %d is invalid", ErrInvalidTavernRequest, price)
		}
		state.Provision.PricePerPack = price
		state.Provision.MaximumUsefulPacks = maximumUsefulTavernProvisionPacks(state.Provision)
		state.Provision.Approvals = make([]TavernProvisionApproval, state.Provision.MaximumUsefulPacks)
		for packCount := 1; packCount <= state.Provision.MaximumUsefulPacks; packCount++ {
			state.Provision.Approvals[packCount-1] = projectTavernProvisionPurchase(state.Provision, packCount)
		}
	}

	return state, nil
}

func maximumUsefulTavernProvisionPacks(quote TavernProvisionQuote) int {
	headroom := int(party_state.ItemQuantityLargeMax - quote.FoodBefore)
	packQuantity := int(quote.Provision.PackQuantity)
	if headroom == 0 {
		return 1
	}

	return min(maximumTavernProvisionPacks, (headroom+packQuantity-1)/packQuantity)
}

func projectTavernProvisionPurchase(quote TavernProvisionQuote, packCount int) TavernProvisionApproval {
	approval := TavernProvisionApproval{
		PackCount:          packCount,
		GoldAfterPurchases: quote.GoldBefore,
		FoodAfter:          quote.FoodBefore,
	}
	affordablePacks := int(quote.GoldBefore) / quote.PricePerPack
	if affordablePacks == 0 {
		approval.CharityPossible = quote.FoodBefore < 3
		if approval.CharityPossible {
			approval.Outcome = TavernProvisionCharityEjection
		} else {
			approval.Outcome = TavernProvisionEjection
		}

		return approval
	}

	approval.PurchasedPacks = min(packCount, affordablePacks, maximumUsefulTavernProvisionPacks(quote))
	approval.TotalCost = approval.PurchasedPacks * quote.PricePerPack
	approval.GoldAfterPurchases -= uint16(approval.TotalCost)
	offeredFood := approval.PurchasedPacks * int(quote.Provision.PackQuantity)
	approval.FoodAfter = uint16(min(
		int(party_state.ItemQuantityLargeMax),
		int(quote.FoodBefore)+offeredFood,
	))
	approval.ActualFood = approval.FoodAfter - quote.FoodBefore
	switch {
	case approval.FoodAfter == party_state.ItemQuantityLargeMax:
		approval.Outcome = TavernProvisionSaturated
	case approval.PurchasedPacks < packCount:
		approval.Outcome = TavernProvisionPartial
	default:
		approval.Outcome = TavernProvisionCompleted
	}

	return approval
}

func (g *GameState) buildTavernDrinkQuote(
	state TavernShopState,
	wineIndex int,
	wine references.TavernWine,
) TavernDrinkQuote {
	charge := state.Menu.Drink.UnitPrice * state.DinerCount
	if wineIndex != TavernOrdinaryDrinkWineIndex {
		charge = wine.Price
	}
	quote := TavernDrinkQuote{
		Drink:            state.Menu.Drink,
		WineIndex:        wineIndex,
		Wine:             wine,
		DinerCount:       state.DinerCount,
		Charge:           uint16(charge),
		Affordable:       int(state.Gold) >= charge,
		Warning:          state.Drunk == 3,
		StopAvailable:    state.Drunk == 3,
		GoldBefore:       state.Gold,
		GoldAfterPayment: state.Gold,
		Drunk:            state.Drunk,
		Smashed:          state.Smashed,
		Karma:            state.Karma,
	}
	if quote.Affordable {
		quote.GoldAfterPayment -= quote.Charge
	}

	return quote
}

func newTavernDrinkResult(state TavernShopState, quote TavernDrinkQuote) TavernDrinkResult {
	return TavernDrinkResult{
		MerchantID:       state.Merchant.ID,
		Drink:            quote.Drink,
		WineIndex:        quote.WineIndex,
		Wine:             quote.Wine,
		DinerCount:       quote.DinerCount,
		Charge:           quote.Charge,
		Warning:          quote.Warning,
		GoldBefore:       quote.GoldBefore,
		GoldAfterPayment: quote.GoldBefore,
		GoldAfterTheft:   quote.GoldBefore,
		DrunkBefore:      state.Drunk,
		DrunkAfter:       state.Drunk,
		SmashedBefore:    state.Smashed,
		SmashedAfter:     state.Smashed,
		KarmaBefore:      state.Karma,
		KarmaAfter:       state.Karma,
		CustomerBefore:   state.CustomerEligible,
		CustomerAfter:    state.CustomerEligible,
	}
}

func (g *GameState) tavernDinerCount() int {
	diners := 0
	for index := range party_state.MaxActiveParty {
		character := g.PartyState.GetCharacter(index)
		if character.PartyMap != party_state.PartyMapActive {
			break
		}
		if character.GetStatus() != party_state.Dead {
			diners++
		}
	}

	return diners
}

func (g *GameState) tavernMealTableVisual() TavernMealTableVisual {
	layeredMap := g.GetLayeredMapByCurrentLocation()
	if layeredMap == nil {
		return TavernMealTableVisual{}
	}
	playerPosition := g.MapState.PlayerLocation.Position
	north := playerPosition.GetPositionUp()
	if tile := layeredMap.GetTileTopMapOnlyTile(north); tile != nil && tile.Is(indexes.TableMiddle) {
		return TavernMealTableVisual{Show: true, Position: *north, SpriteIndex: indexes.TableFoodBottom}
	}
	south := playerPosition.GetPositionDown()
	if tile := layeredMap.GetTileTopMapOnlyTile(south); tile != nil && tile.Is(indexes.TableMiddle) {
		return TavernMealTableVisual{Show: true, Position: *south, SpriteIndex: indexes.TableFoodTop}
	}

	return TavernMealTableVisual{}
}
