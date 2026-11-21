package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

var (
	// ErrInvalidHealerRequest indicates stale or unsupported service/recipient input.
	ErrInvalidHealerRequest = errors.New("invalid healer request")
	// ErrHealerServiceUnneeded indicates that the selected recipient is ineligible.
	ErrHealerServiceUnneeded = errors.New("healer service is not needed")
	// ErrHealerInsufficientFunds indicates that non-charitable care cannot be afforded.
	ErrHealerInsufficientFunds = errors.New("insufficient funds for healer service")
)

// HealerAllRecipientsIndex identifies the Redux batch target in a healer request.
const HealerAllRecipientsIndex = -1

// HealerChargeKind identifies how a currently quoted treatment will be funded.
type HealerChargeKind byte

const (
	// HealerChargePaid deducts the fixed source price and invokes eligible Faulinei theft.
	HealerChargePaid HealerChargeKind = iota
	// HealerChargeFree is Minoc Cure or Heal with no payment or theft.
	HealerChargeFree
	// HealerChargeCharitable is unaffordable Skara Brae Cure or Heal with no payment or theft.
	HealerChargeCharitable
)

// HealerEligibility identifies the original service-specific recipient gate.
type HealerEligibility byte

const (
	// HealerEligible permits the selected service for the recipient.
	HealerEligible HealerEligibility = iota
	// HealerNotPoisoned rejects Cure for a recipient who is not poisoned.
	HealerNotPoisoned
	// HealerNotInjured rejects Heal for a recipient already at maximum HP.
	HealerNotInjured
	// HealerDeadCannotHeal rejects Heal for a dead recipient.
	HealerDeadCannotHeal
	// HealerLivingCannotResurrect rejects Resurrect for a living recipient.
	HealerLivingCannotResurrect
)

// IsEligible reports whether the treatment can be committed for this recipient.
func (e HealerEligibility) IsEligible() bool { return e == HealerEligible }

// HealerServiceQuote describes one fixed-price service against current party gold.
type HealerServiceQuote struct {
	Service     references.HealerService
	SourcePrice uint16
	Charge      uint16
	ChargeKind  HealerChargeKind
	Affordable  bool
}

// HealerAllServiceQuote describes the currently offered active-roster batch.
type HealerAllServiceQuote struct {
	Service        references.HealerService
	SourcePrice    uint16
	Charge         uint16
	ChargeKind     HealerChargeKind
	Affordable     bool
	RecipientCount int
}

// HealerTreatmentProjection describes one service's authoritative projected effect.
type HealerTreatmentProjection struct {
	Eligibility HealerEligibility
	StatusAfter party_state.CharacterStatus
	HPAfter     uint16
	MaxHPAfter  uint16
	MPAfter     byte
	ExpAfter    uint16
	LevelAfter  byte
}

// HealerRecipientState is one active-party member and all three service projections.
type HealerRecipientState struct {
	CharacterIndex int
	Name           string
	Status         party_state.CharacterStatus
	HP             uint16
	MaxHP          uint16
	MP             byte
	Exp            uint16
	Level          byte
	Treatments     [3]HealerTreatmentProjection
}

// HealerShopState is a trusted snapshot of one healer session.
type HealerShopState struct {
	Merchant              references.MerchantDefinition
	PricingCharacterIndex int
	PricingCharacterName  string
	Gold                  uint16
	Services              [3]HealerServiceQuote
	AllServices           [3]HealerAllServiceQuote
	Recipients            []HealerRecipientState
}

// HealerShopReview is a read-only production snapshot with canonical sprite identity.
type HealerShopReview = MerchantShopReview[HealerShopState]

// HealerServiceRequest identifies one service and active-party recipient.
type HealerServiceRequest struct {
	Service        references.HealerService
	CharacterIndex int
}

// HealerTreatmentQuote combines the current service charge and recipient projection.
type HealerTreatmentQuote struct {
	Service    HealerServiceQuote
	Recipient  HealerRecipientState
	Projection HealerTreatmentProjection
}

// HealerAllTreatmentQuote combines the offered charge with selected recipients in roster order.
type HealerAllTreatmentQuote struct {
	Service    HealerAllServiceQuote
	Recipients []HealerRecipientState
}

// HealerServiceResult reports one complete authoritative treatment.
type HealerServiceResult struct {
	Service          references.HealerService
	CharacterIndex   int
	CharacterName    string
	ChargeKind       HealerChargeKind
	SourcePrice      uint16
	Charge           uint16
	GoldBefore       uint16
	GoldAfterPayment uint16
	Theft            uint16
	GoldAfterTheft   uint16
	StatusBefore     party_state.CharacterStatus
	StatusAfter      party_state.CharacterStatus
	HPBefore         uint16
	HPAfter          uint16
	MaxHPBefore      uint16
	MaxHPAfter       uint16
	MPBefore         byte
	MPAfter          byte
	ExpBefore        uint16
	ExpAfter         uint16
	LevelBefore      byte
	LevelAfter       byte
}

// HealerAllServiceResult reports completed treatments, including an interrupted paid batch.
type HealerAllServiceResult struct {
	Service          references.HealerService
	CharacterIndexes []int
	ChargeKind       HealerChargeKind
	SourcePrice      uint16
	Charge           uint16
	GoldBefore       uint16
	GoldAfterPayment uint16
	Theft            uint16
	GoldAfterTheft   uint16
}

// GetHealerShopState returns current service charges and active-party projections.
func (g *GameState) GetHealerShopState(session MerchantSession) (HealerShopState, error) {
	buyer, merchant, err := g.validateMerchantSession(session, references.MerchantFamilyHealer)
	if err != nil {
		return HealerShopState{}, err
	}

	return g.buildHealerShopState(merchant, buyer, session.buyerIndex)
}

// ReviewHealerShopByMerchantID builds production treatment projections without
// issuing a session, invoking callbacks, consuming RNG, or consulting loaded NPC state.
func (g *GameState) ReviewHealerShopByMerchantID(id references.MerchantID) (HealerShopReview, error) {
	return reviewMerchantShop(g, id, references.MerchantFamilyHealer, ErrInvalidHealerRequest, "a healer", g.buildHealerShopState)
}

// QuoteHealerTreatment returns a current trusted charge and recipient projection.
func (g *GameState) QuoteHealerTreatment(
	session MerchantSession,
	request HealerServiceRequest,
) (HealerTreatmentQuote, error) {
	_, merchant, err := g.validateMerchantSession(session, references.MerchantFamilyHealer)
	if err != nil {
		return HealerTreatmentQuote{}, err
	}

	return g.quoteHealerTreatment(merchant, request)
}

// ProvideHealerService validates and commits one original healer treatment.
func (g *GameState) ProvideHealerService(
	session MerchantSession,
	request HealerServiceRequest,
) (HealerServiceResult, error) {
	quote, err := g.QuoteHealerTreatment(session, request)
	if err != nil {
		return HealerServiceResult{}, err
	}
	if !quote.Projection.Eligibility.IsEligible() {
		return HealerServiceResult{}, fmt.Errorf(
			"%w: %s for %s",
			ErrHealerServiceUnneeded,
			request.Service,
			quote.Recipient.Name,
		)
	}
	if !quote.Service.Affordable {
		return HealerServiceResult{}, fmt.Errorf(
			"%w: need %d gold, have %d",
			ErrHealerInsufficientFunds,
			quote.Service.Charge,
			g.PartyState.Inventory.Gold.Get(),
		)
	}

	character := g.PartyState.GetCharacter(request.CharacterIndex)
	result := HealerServiceResult{
		Service:          request.Service,
		CharacterIndex:   request.CharacterIndex,
		CharacterName:    quote.Recipient.Name,
		ChargeKind:       quote.Service.ChargeKind,
		SourcePrice:      quote.Service.SourcePrice,
		Charge:           quote.Service.Charge,
		GoldBefore:       g.PartyState.Inventory.Gold.Get(),
		GoldAfterPayment: g.PartyState.Inventory.Gold.Get(),
		GoldAfterTheft:   g.PartyState.Inventory.Gold.Get(),
		StatusBefore:     character.GetStatus(),
		StatusAfter:      character.GetStatus(),
		HPBefore:         character.GetCurrentHp(),
		HPAfter:          character.GetCurrentHp(),
		MaxHPBefore:      character.GetMaxHp(),
		MaxHPAfter:       character.GetMaxHp(),
		MPBefore:         character.GetCurrentMp(),
		MPAfter:          character.GetCurrentMp(),
		ExpBefore:        character.Exp,
		ExpAfter:         character.Exp,
		LevelBefore:      character.Level,
		LevelAfter:       character.Level,
	}

	if quote.Service.ChargeKind == HealerChargePaid {
		if !g.PartyState.Inventory.Gold.DecrementBy(quote.Service.Charge) {
			panic("validated healer service could not deduct gold")
		}
		result.GoldAfterPayment = g.PartyState.Inventory.Gold.Get()
		result.Theft = g.ApplyFaulineiShopTheft()
		result.GoldAfterTheft = g.PartyState.Inventory.Gold.Get()
	}

	changes := transientStatusChanges{}
	statusBefore := character.GetStatus()
	hpBefore := character.GetCurrentHp()
	g.applyHealerTreatment(request.Service, request.CharacterIndex, character)
	if request.Service == references.HealerHeal {
		changes.observeHealing(hpBefore, character)
	}
	if request.Service == references.HealerCure {
		changes.observePoison(statusBefore, character)
	}
	g.publishTransientStatusChanges(changes)

	result.StatusAfter = character.GetStatus()
	result.HPAfter = character.GetCurrentHp()
	result.MaxHPAfter = character.GetMaxHp()
	result.MPAfter = character.GetCurrentMp()
	result.ExpAfter = character.Exp
	result.LevelAfter = character.Level

	return result, nil
}

// QuoteHealerServiceForAll returns the aggregate charge and every eligible active recipient.
func (g *GameState) QuoteHealerServiceForAll(
	session MerchantSession,
	service references.HealerService,
) (HealerAllTreatmentQuote, error) {
	_, merchant, err := g.validateMerchantSession(session, references.MerchantFamilyHealer)
	if err != nil {
		return HealerAllTreatmentQuote{}, err
	}

	return g.quoteHealerServiceForAll(merchant, service)
}

// ProvideHealerServiceToAll commits the offered batch in active roster order.
func (g *GameState) ProvideHealerServiceToAll(
	session MerchantSession,
	service references.HealerService,
) (HealerAllServiceResult, error) {
	quote, err := g.QuoteHealerServiceForAll(session, service)
	if err != nil {
		return HealerAllServiceResult{}, err
	}
	if len(quote.Recipients) == 0 {
		return HealerAllServiceResult{}, fmt.Errorf("%w: %s for all", ErrHealerServiceUnneeded, service)
	}
	if !quote.Service.Affordable {
		return HealerAllServiceResult{}, fmt.Errorf(
			"%w: need %d gold, have %d",
			ErrHealerInsufficientFunds,
			quote.Service.Charge,
			g.PartyState.Inventory.Gold.Get(),
		)
	}

	// REDUX DEVIATION: healer services treats one selected recipient per
	// transaction. All groups recipients for keyboard convenience; the default
	// batching option also groups payment and theft into one boundary.
	// See ref:MERCH-0007.
	result := g.newHealerAllServiceResult(quote)
	batched := g.GameConfig.GameplayConfiguration.BatchMerchantTheft
	if quote.Service.ChargeKind == HealerChargePaid && batched {
		if !g.PartyState.Inventory.Gold.DecrementBy(quote.Service.Charge) {
			panic("validated aggregate healer service could not deduct gold")
		}
		result.GoldAfterPayment = g.PartyState.Inventory.Gold.Get()
		result.Theft = g.ApplyFaulineiShopTheft()
		result.GoldAfterTheft = g.PartyState.Inventory.Gold.Get()
	}
	if !batched {
		result.Charge = 0
	}
	changes := transientStatusChanges{}
	for _, recipient := range quote.Recipients {
		if quote.Service.ChargeKind == HealerChargePaid && !batched {
			if !g.PartyState.Inventory.Gold.DecrementBy(quote.Service.SourcePrice) {
				break
			}
			result.Charge += quote.Service.SourcePrice
			result.GoldAfterPayment = g.PartyState.Inventory.Gold.Get()
			result.Theft += g.ApplyFaulineiShopTheft()
			result.GoldAfterTheft = g.PartyState.Inventory.Gold.Get()
		}
		character := g.PartyState.GetCharacter(recipient.CharacterIndex)
		statusBefore := character.GetStatus()
		hpBefore := character.GetCurrentHp()
		g.applyHealerTreatment(service, recipient.CharacterIndex, character)
		result.CharacterIndexes = append(result.CharacterIndexes, recipient.CharacterIndex)
		if service == references.HealerHeal {
			changes.observeHealing(hpBefore, character)
		}
		if service == references.HealerCure {
			changes.observePoison(statusBefore, character)
		}
	}
	g.publishTransientStatusChanges(changes)

	return result, nil
}

func (g *GameState) buildHealerShopState(
	merchant references.MerchantDefinition,
	buyer *party_state.PlayerCharacter,
	buyerIndex int,
) (HealerShopState, error) {
	offers, ok := g.GameReferences.MerchantReferences.HealerOffers(merchant.ID)
	if !ok {
		return HealerShopState{}, fmt.Errorf("%w: no healer catalog for merchant %q", ErrInvalidHealerRequest, merchant.ID)
	}

	state := HealerShopState{
		Merchant:              merchant,
		PricingCharacterIndex: buyerIndex,
		PricingCharacterName:  buyer.GetNameAsString(),
		Gold:                  g.PartyState.Inventory.Gold.Get(),
		Recipients:            make([]HealerRecipientState, 0, party_state.MaxActiveParty),
	}
	for index, offer := range offers {
		state.Services[index] = g.quoteHealerService(merchant, offer)
	}
	for index := range party_state.MaxActiveParty {
		character := g.PartyState.GetCharacter(index)
		if character == nil || character.PartyMap != party_state.PartyMapActive {
			continue
		}
		state.Recipients = append(state.Recipients, g.healerRecipientState(index, character))
	}
	for index, offer := range offers {
		state.AllServices[index] = g.quoteHealerAllService(merchant, offer, state.Recipients)
	}

	return state, nil
}

func (g *GameState) quoteHealerTreatment(
	merchant references.MerchantDefinition,
	request HealerServiceRequest,
) (HealerTreatmentQuote, error) {
	offers, ok := g.GameReferences.MerchantReferences.HealerOffers(merchant.ID)
	if !ok {
		return HealerTreatmentQuote{}, fmt.Errorf("%w: no healer catalog for merchant %q", ErrInvalidHealerRequest, merchant.ID)
	}
	serviceIndex := int(request.Service)
	if serviceIndex < 0 || serviceIndex >= len(offers) || offers[serviceIndex].Service != request.Service {
		return HealerTreatmentQuote{}, fmt.Errorf("%w: unsupported service %d", ErrInvalidHealerRequest, request.Service)
	}
	if request.CharacterIndex < 0 || request.CharacterIndex >= party_state.MaxActiveParty {
		return HealerTreatmentQuote{}, fmt.Errorf("%w: recipient index %d is outside the active party", ErrInvalidHealerRequest, request.CharacterIndex)
	}
	character := g.PartyState.GetCharacter(request.CharacterIndex)
	if character == nil || character.PartyMap != party_state.PartyMapActive {
		return HealerTreatmentQuote{}, fmt.Errorf("%w: recipient index %d is not active", ErrInvalidHealerRequest, request.CharacterIndex)
	}
	recipient := g.healerRecipientState(request.CharacterIndex, character)

	return HealerTreatmentQuote{
		Service:    g.quoteHealerService(merchant, offers[serviceIndex]),
		Recipient:  recipient,
		Projection: recipient.Treatments[serviceIndex],
	}, nil
}

func (g *GameState) quoteHealerServiceForAll(
	merchant references.MerchantDefinition,
	service references.HealerService,
) (HealerAllTreatmentQuote, error) {
	offers, ok := g.GameReferences.MerchantReferences.HealerOffers(merchant.ID)
	if !ok {
		return HealerAllTreatmentQuote{}, fmt.Errorf("%w: no healer catalog for merchant %q", ErrInvalidHealerRequest, merchant.ID)
	}
	serviceIndex := int(service)
	if serviceIndex < 0 || serviceIndex >= len(offers) || offers[serviceIndex].Service != service {
		return HealerAllTreatmentQuote{}, fmt.Errorf("%w: unsupported service %d", ErrInvalidHealerRequest, service)
	}
	recipients := make([]HealerRecipientState, 0, party_state.MaxActiveParty)
	for characterIndex := range party_state.MaxActiveParty {
		character := g.PartyState.GetCharacter(characterIndex)
		if character == nil || character.PartyMap != party_state.PartyMapActive {
			continue
		}
		recipient := g.healerRecipientState(characterIndex, character)
		if recipient.Treatments[serviceIndex].Eligibility.IsEligible() {
			recipients = append(recipients, recipient)
		}
	}

	quote := g.quoteHealerAllService(merchant, offers[serviceIndex], recipients)

	return HealerAllTreatmentQuote{
		Service:    quote,
		Recipients: recipients[:quote.RecipientCount],
	}, nil
}

func (g *GameState) quoteHealerService(
	merchant references.MerchantDefinition,
	offer references.HealerOffer,
) HealerServiceQuote {
	quote := HealerServiceQuote{
		Service:     offer.Service,
		SourcePrice: offer.Price,
		Charge:      offer.Price,
		ChargeKind:  HealerChargePaid,
		Affordable:  g.PartyState.Inventory.Gold.Get() >= offer.Price,
	}
	if merchant.ID == references.MerchantHealerMinoc &&
		(offer.Service == references.HealerCure || offer.Service == references.HealerHeal) {
		quote.Charge = 0
		quote.ChargeKind = HealerChargeFree
		quote.Affordable = true
	} else if merchant.ID == references.MerchantHealerSkaraBrae &&
		(offer.Service == references.HealerCure || offer.Service == references.HealerHeal) &&
		!quote.Affordable {
		quote.Charge = 0
		quote.ChargeKind = HealerChargeCharitable
		quote.Affordable = true
	}

	return quote
}

func (g *GameState) quoteHealerAllService(
	merchant references.MerchantDefinition,
	offer references.HealerOffer,
	recipients []HealerRecipientState,
) HealerAllServiceQuote {
	recipientCount := 0
	for _, recipient := range recipients {
		if recipient.Treatments[int(offer.Service)].Eligibility.IsEligible() {
			recipientCount++
		}
	}
	charge := uint16(recipientCount) * offer.Price
	quote := HealerAllServiceQuote{
		Service:        offer.Service,
		SourcePrice:    offer.Price,
		Charge:         charge,
		ChargeKind:     HealerChargePaid,
		Affordable:     g.PartyState.Inventory.Gold.Get() >= charge,
		RecipientCount: recipientCount,
	}
	if merchant.ID == references.MerchantHealerMinoc &&
		(offer.Service == references.HealerCure || offer.Service == references.HealerHeal) {
		quote.Charge = 0
		quote.ChargeKind = HealerChargeFree
		quote.Affordable = true
	} else if merchant.ID == references.MerchantHealerSkaraBrae &&
		(offer.Service == references.HealerCure || offer.Service == references.HealerHeal) {
		// REDUX DEVIATION: healer services offers charity per unaffordable
		// patient. All first reviews only affordable paid patients in roster order;
		// a fresh All review offers bulk charity only when no further patient can
		// be paid. Separate approval prevents silently mixing paid and free care.
		// See ref:MERCH-0007.
		paidCount := min(recipientCount, int(g.PartyState.Inventory.Gold.Get()/offer.Price))
		if paidCount > 0 {
			quote.RecipientCount = paidCount
			quote.Charge = uint16(paidCount) * offer.Price
		} else {
			quote.Charge = 0
			quote.ChargeKind = HealerChargeCharitable
		}
		quote.Affordable = true
	}

	return quote
}

func (g *GameState) newHealerAllServiceResult(quote HealerAllTreatmentQuote) HealerAllServiceResult {
	gold := g.PartyState.Inventory.Gold.Get()

	return HealerAllServiceResult{
		Service:          quote.Service.Service,
		CharacterIndexes: make([]int, 0, len(quote.Recipients)),
		ChargeKind:       quote.Service.ChargeKind,
		SourcePrice:      quote.Service.SourcePrice,
		Charge:           quote.Service.Charge,
		GoldBefore:       gold,
		GoldAfterPayment: gold,
		Theft:            0,
		GoldAfterTheft:   gold,
	}
}

func (g *GameState) applyHealerTreatment(
	service references.HealerService,
	characterIndex int,
	character *party_state.PlayerCharacter,
) {
	if character == nil {
		panic("validated healer request reached missing recipient")
	}
	switch service {
	case references.HealerCure:
		character.SetStatus(party_state.Good)
	case references.HealerHeal:
		character.SetHp(character.GetMaxHp())
	case references.HealerResurrect:
		if !g.ResurrectCharacter(characterIndex) {
			panic("validated healer resurrection could not resurrect recipient")
		}
		character.SetHp(character.GetMaxHp())
	default:
		panic("validated healer request reached unknown service")
	}
}

func (g *GameState) healerRecipientState(
	characterIndex int,
	character *party_state.PlayerCharacter,
) HealerRecipientState {
	recipient := HealerRecipientState{
		CharacterIndex: characterIndex,
		Name:           character.GetNameAsString(),
		Status:         character.GetStatus(),
		HP:             character.GetCurrentHp(),
		MaxHP:          character.GetMaxHp(),
		MP:             character.GetCurrentMp(),
		Exp:            character.Exp,
		Level:          character.Level,
	}
	for _, service := range [...]references.HealerService{
		references.HealerCure,
		references.HealerHeal,
		references.HealerResurrect,
	} {
		recipient.Treatments[int(service)] = g.projectHealerTreatment(service, character)
	}

	return recipient
}

func (g *GameState) projectHealerTreatment(
	service references.HealerService,
	character *party_state.PlayerCharacter,
) HealerTreatmentProjection {
	projection := HealerTreatmentProjection{
		Eligibility: HealerEligible,
		StatusAfter: character.GetStatus(),
		HPAfter:     character.GetCurrentHp(),
		MaxHPAfter:  character.GetMaxHp(),
		MPAfter:     character.GetCurrentMp(),
		ExpAfter:    character.Exp,
		LevelAfter:  character.Level,
	}
	switch service {
	case references.HealerCure:
		if !character.IsPoisoned() {
			projection.Eligibility = HealerNotPoisoned
		} else {
			projection.StatusAfter = party_state.Good
		}
	case references.HealerHeal:
		switch {
		case character.IsDead() || character.GetCurrentHp() == 0:
			projection.Eligibility = HealerDeadCannotHeal
		case character.GetCurrentHp() == character.GetMaxHp():
			projection.Eligibility = HealerNotInjured
		default:
			projection.HPAfter = character.GetMaxHp()
		}
	case references.HealerResurrect:
		if !character.IsDead() {
			projection.Eligibility = HealerLivingCannotResurrect
		} else {
			projection.StatusAfter = party_state.Good
			if g.PartyState.Karma.Value < 98 {
				projection.ExpAfter = uint16((int(character.Exp) * g.PartyState.Karma.Value) / KarmaPercentageBase)
			}
			projection.LevelAfter = CalculateLevelFromExperience(projection.ExpAfter)
			projection.MaxHPAfter = uint16(projection.LevelAfter) * 30
			projection.HPAfter = projection.MaxHPAfter
			switch character.Class {
			case party_state.Avatar, party_state.Wizard:
				projection.MPAfter = character.Intelligence
			case party_state.Bard:
				projection.MPAfter = character.Intelligence / 2
			case party_state.Fighter:
				// Original resurrection leaves Fighter MP unchanged.
			}
		}
	default:
		projection.Eligibility = HealerLivingCannotResurrect
	}

	return projection
}
