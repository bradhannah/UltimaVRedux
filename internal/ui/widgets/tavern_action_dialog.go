package widgets

import (
	"fmt"
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	tavernEndsService      = "Asking anyway ends service."
	tavernOrderEndsService = "Ordering anyway ends service."
	tavernCharityPossible  = "The keeper may spare 1 or 2 food."
	tavernAnyway           = " anyway"
	tavernOrderVerb        = "Order"
)

var _ Widget = (*TavernActionDialog)(nil)

// TavernActionDialogConfig configures one compact keyboard-only Tavern approval.
type TavernActionDialogConfig struct {
	Keyboard     *input.Keyboard
	Intent       TavernActionIntent
	IsKeyPressed func(ebiten.Key) bool
}

// TavernActionDialog presents detached reviewed consequences and emits exact requests unchanged.
type TavernActionDialog struct {
	quantityInput
	keyboard        *input.Keyboard
	isKeyPressed    func(ebiten.Key) bool
	intent          TavernActionIntent
	visible         bool
	dismissed       bool
	cancelled       bool
	actionKeysReady bool
	pending         *TavernActionRequest
	provisionPacks  int
	window          *amountWindow
}

// NewTavernActionDialog creates a compact approval from one fully detached intent union.
func NewTavernActionDialog(config TavernActionDialogConfig) *TavernActionDialog {
	if config.Keyboard == nil {
		panic("TavernActionDialog: keyboard is required")
	}
	validateTavernActionIntent(config.Intent)
	isKeyPressed := config.IsKeyPressed
	if isKeyPressed == nil {
		isKeyPressed = config.Keyboard.IsKeyHeld
	}
	dialog := &TavernActionDialog{
		quantityInput:   newQuantityInput(config.Keyboard, isKeyPressed, ebiten.KeyM, ebiten.KeyUp, ebiten.KeyDown),
		keyboard:        config.Keyboard,
		isKeyPressed:    isKeyPressed,
		intent:          cloneTavernActionIntent(config.Intent),
		visible:         true,
		dismissed:       false,
		cancelled:       false,
		actionKeysReady: tavernActionControlsReleased(isKeyPressed),
		pending:         nil,
		provisionPacks:  config.Intent.PackCount,
		window:          newAmountWindow(),
	}
	config.Keyboard.PrepareForTransition(0)
	dialog.window.set(dialog.step())

	return dialog
}

// Draw renders the opaque semantic Tavern approval modal.
func (d *TavernActionDialog) Draw(screen *ebiten.Image) {
	if screen == nil || !d.IsVisible() {
		return
	}
	d.window.draw(screen)
}

// Update confirms one normal action, chooses warned Stop/Continue, or cancels.
func (d *TavernActionDialog) Update() {
	if !d.IsVisible() {
		return
	}
	if !d.prepareActionKeys() {
		return
	}
	if d.intent.Drink.Warning {
		switch {
		case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape):
			d.accept(d.intent.StopRequest)
		case d.confirmPressed():
			d.accept(d.intent.Request)
		}

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.cancel()

		return
	}
	if d.intent.Action == TavernActionProvision && d.updateProvisionQuantity() {
		return
	}
	if d.confirmPressed() {
		d.accept(d.intent.Request)
	}
}

func (d *TavernActionDialog) confirmPressed() bool {
	return d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
}

func (d *TavernActionDialog) prepareActionKeys() bool {
	if d.actionKeysReady {
		return true
	}
	if !tavernActionControlsReleased(d.isKeyPressed) {
		return false
	}
	d.actionKeysReady = true

	return true
}

// updateProvisionQuantity steps or types the pack count. With no gold for even
// one pack every count comes to the same thing, so there is none to choose.
func (d *TavernActionDialog) updateProvisionQuantity() bool {
	if d.noPackAffordable() {
		_, owned := d.pollQuantityInput()

		return owned
	}
	packs, owned := d.pollAmount(d.provisionPacks, d.intent.Provision.MaximumUsefulPacks)
	if packs != d.provisionPacks {
		d.provisionPacks = packs
		d.intent.PackCount = packs
		d.intent.Request.Provision.PackCount = packs
		d.window.set(d.step())
	}

	return owned
}

func (d *TavernActionDialog) noPackAffordable() bool {
	outcome := d.intent.Provision.Approvals[d.provisionPacks-1].Outcome

	return outcome == game_state.TavernProvisionCharityEjection || outcome == game_state.TavernProvisionEjection
}

// ConsumePendingRequest returns and clears the exact approved comparison request.
func (d *TavernActionDialog) ConsumePendingRequest() (TavernActionRequest, bool) {
	if d.pending == nil {
		return emptyTavernActionRequest(), false
	}
	request := *d.pending
	d.pending = nil

	return request, true
}

// GetBounds returns the centered responsive modal bounds.
func (d *TavernActionDialog) GetBounds() image.Rectangle {
	return d.window.bounds()
}

// IsCancelled reports whether Escape returned to the parent without a request.
func (d *TavernActionDialog) IsCancelled() bool { return d.cancelled }

// IsDismissed reports whether workflow may remove the approval.
func (d *TavernActionDialog) IsDismissed() bool { return d.dismissed }

// IsVisible reports whether approval currently owns presentation and input.
func (d *TavernActionDialog) IsVisible() bool { return d.visible && !d.dismissed }

// Dismiss closes approval without emitting a request.
func (d *TavernActionDialog) Dismiss() {
	if d.dismissed {
		return
	}
	d.onDialogFocusChanged(false)
	d.keyboard.PrepareForTransition(0)
	d.visible = false
	d.dismissed = true
}

func (d *TavernActionDialog) accept(request TavernActionRequest) {
	accepted := request
	d.pending = &accepted
	d.Dismiss()
}

func (d *TavernActionDialog) cancel() {
	d.cancelled = true
	d.Dismiss()
}

// step describes the approval window for this order: what it is, how its price
// is made up, and what the party's gold and food will be.
func (d *TavernActionDialog) step() amountStep {
	step := emptyAmountStep()
	step.preview = d.intent.Preview
	switch d.intent.Action {
	case TavernActionMeal:
		d.describeMeal(&step)
	case TavernActionDrink:
		d.describeDrink(&step)
	case TavernActionProvision:
		d.describeProvision(&step)
	case TavernActionInformation:
		d.describeInformation(&step)
	case TavernActionStop, TavernActionUnknown:
		panic("TavernActionDialog.step: invalid standalone action")
	default:
		panic("TavernActionDialog.step: unknown action")
	}

	return step
}

func (d *TavernActionDialog) describeMeal(step *amountStep) {
	quote := d.intent.Meal
	step.title, step.verb = tavernOrderVerb+" "+quote.Meal.Name, tavernOrderVerb
	step.unit = fmt.Sprintf("%d gold each for %s", quote.Meal.BasePrice, countedNoun(quote.DinerCount, "diner"))
	describeTavernPayment(step, quote.GoldBefore, quote.GoldAfterPayment, quote.Affordable, quote.Charge,
		tavernOrderEndsService)
}

func (d *TavernActionDialog) describeDrink(step *amountStep) {
	quote := d.intent.Drink
	step.title, step.verb = tavernOrderVerb+" "+quote.Drink.Name, tavernOrderVerb
	step.unit = fmt.Sprintf("%d gold each for %s", quote.Drink.UnitPrice, countedNoun(quote.DinerCount, "drink"))
	if quote.WineIndex != game_state.TavernOrdinaryDrinkWineIndex {
		step.title, step.unit = tavernOrderVerb+" "+quote.Wine.Name, "By the bottle"
	}
	if quote.Warning {
		step.title, step.pitch = "One More Drink?", d.intent.DrinkWarning
		step.verb, step.back = "Another Drink", "I've Had Enough"
	}
	describeTavernPayment(step, quote.GoldBefore, quote.GoldAfterPayment, quote.Affordable, quote.Charge,
		tavernOrderEndsService)
}

func (d *TavernActionDialog) describeInformation(step *amountStep) {
	quote := d.intent.Information
	step.title, step.verb, step.pitch = "Buy Information", "Pay", d.intent.InformationPitch
	describeTavernPayment(step, quote.GoldBefore, quote.GoldAfterPayment, quote.Affordable,
		uint16(quote.Price), //nolint:gosec // Canonical topic price is validated and bounded.
		tavernEndsService)
}

// describeTavernPayment gives the gold row of a payment the party can make. One
// it cannot make is still a real request, which the keeper answers by ending
// service (MERCH-0012 to MERCH-0014), so Enter stays and says what it will do.
func describeTavernPayment(step *amountStep, before, after uint16, affordable bool, charge uint16, ends string) {
	if affordable {
		step.rows = []amountRow{{label: amountGoldRow, have: int(before), then: int(after)}}

		return
	}
	if step.back == "" {
		step.pitch = ""
		step.verb += tavernAnyway
	}
	step.notes = shortOfGoldNotes("It costs", int(charge), int(before), ends)
}

func (d *TavernActionDialog) describeProvision(step *amountStep) {
	quote := d.intent.Provision
	approval := quote.Approvals[d.provisionPacks-1]
	step.title, step.verb = amountBuyVerb+" "+quote.Provision.Name, amountBuyVerb
	step.unit = fmt.Sprintf("%d gold for a pack of %d food", quote.PricePerPack, quote.Provision.PackQuantity)
	if d.noPackAffordable() {
		consequences := []string{tavernEndsService}
		if approval.CharityPossible {
			consequences = append(consequences, tavernCharityPossible)
		}
		step.verb = "Ask" + tavernAnyway
		step.notes = shortOfGoldNotes("A pack costs", quote.PricePerPack, int(quote.GoldBefore), consequences...)

		return
	}
	step.pitch = d.intent.ProvisionPitch
	step.noun, step.count, step.most = "Packs", d.provisionPacks, quote.MaximumUsefulPacks
	step.rows = []amountRow{
		{label: "Food", have: int(quote.FoodBefore), then: int(approval.FoodAfter)},
		{label: amountGoldRow, have: int(quote.GoldBefore), then: int(approval.GoldAfterPurchases)},
	}
	for _, other := range quote.Approvals {
		if len(tavernProvisionNotes(quote, other)) > 0 {
			step.reserve = 1
		}
	}
	step.notes = tavernProvisionNotes(quote, approval)
}

// tavernProvisionNotes says what an order will not bring: packs the gold does not
// cover, which the keeper leaves out (MERCH-0014), or food past the most carried.
func tavernProvisionNotes(quote game_state.TavernProvisionQuote, approval game_state.TavernProvisionApproval,
) []amountNote {
	lost := approval.PurchasedPacks*int(quote.Provision.PackQuantity) - int(approval.ActualFood)
	switch {
	case approval.Outcome == game_state.TavernProvisionPartial:
		return []amountNote{{
			text: fmt.Sprintf("The gold pays for only %s.", countedNoun(approval.PurchasedPacks, "pack")),
			tone: amountWarning,
		}}
	case approval.Outcome == game_state.TavernProvisionSaturated && lost > 0:
		return overflowNote(lost)
	case approval.Outcome == game_state.TavernProvisionOutcomeUnknown:
		panic("TavernActionDialog: provision projection has no outcome")
	default:
		return nil
	}
}

// countedNoun words a count with its noun, such as "1 diner" or "3 diners".
func countedNoun(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}

	return fmt.Sprintf("%d %ss", count, noun)
}

func validateTavernActionIntent(intent TavernActionIntent) {
	if intent.Action == TavernActionUnknown || intent.Request.Action != intent.Action {
		panic("TavernActionDialog: complete detached Tavern action is required")
	}
	switch intent.Action {
	case TavernActionMeal:
		validateTavernMealIntent(intent)
	case TavernActionDrink:
		validateTavernDrinkIntent(intent)
	case TavernActionProvision:
		validateTavernProvisionIntent(intent)
	case TavernActionInformation:
		validateTavernInformationIntent(intent)
	case TavernActionStop:
		panic("TavernActionDialog: Stop is available only within a warned drink")
	case TavernActionUnknown:
		panic("TavernActionDialog: an empty action cannot be approved")
	default:
		panic("TavernActionDialog: known action is required")
	}
}

func validateTavernMealIntent(intent TavernActionIntent) {
	quote := intent.Meal
	request := intent.Request.Meal
	if request.ReviewedMerchantID == "" || request.ReviewedMeal != quote.Meal ||
		request.ReviewedDinerCount != quote.DinerCount || request.ReviewedCharge != quote.Charge ||
		request.ReviewedGoldBefore != quote.GoldBefore || request.ReviewedFoodBefore != quote.FoodBefore ||
		quote.Charge == 0 {
		panic("TavernActionDialog: exact detached meal review is required")
	}
	validateTavernGoldProjection(quote.Charge, quote.GoldBefore, quote.GoldAfterPayment, quote.Affordable)
}

//nolint:cyclop // Exact drink and warning-alternative comparison fields form one validation boundary.
func validateTavernDrinkIntent(intent TavernActionIntent) {
	quote := intent.Drink
	request := intent.Request.Drink
	wantApproval := game_state.TavernDrinkApprovalPurchase
	if quote.Warning {
		wantApproval = game_state.TavernDrinkApprovalContinue
	}
	if request.ReviewedMerchantID == "" || request.ReviewedDrink != quote.Drink ||
		request.ReviewedWineIndex != quote.WineIndex || request.ReviewedWine != quote.Wine ||
		request.ReviewedDinerCount != quote.DinerCount || request.ReviewedCharge != quote.Charge ||
		request.ReviewedGoldBefore != quote.GoldBefore || request.ReviewedDrunk != quote.Drunk ||
		request.ReviewedSmashed != quote.Smashed || request.ReviewedKarma != quote.Karma ||
		request.Approval != wantApproval || quote.Warning != quote.StopAvailable || quote.Charge == 0 {
		panic("TavernActionDialog: exact detached drink review is required")
	}
	validateTavernGoldProjection(quote.Charge, quote.GoldBefore, quote.GoldAfterPayment, quote.Affordable)
	if quote.Warning {
		if strings.TrimSpace(intent.DrinkWarning) == "" {
			panic("TavernActionDialog: warned drink requires barkeeper prose")
		}
		if intent.StopRequest.Action != TavernActionStop ||
			intent.StopRequest.Stop != (game_state.TavernStopRequest{
				ReviewedMerchantID: request.ReviewedMerchantID,
				ReviewedDrunk:      quote.Drunk,
			}) {
			panic("TavernActionDialog: warned drink requires exact Stop request")
		}
	} else if intent.StopRequest != emptyTavernActionRequest() {
		panic("TavernActionDialog: ordinary drink cannot expose Stop")
	}
}

func validateTavernProvisionIntent(intent TavernActionIntent) {
	quote := intent.Provision
	request := intent.Request.Provision
	if !tavernProvisionReviewMatches(request, quote, intent.ProvisionPitch) ||
		!tavernProvisionQuantityMatches(request.PackCount, intent.PackCount, quote) {
		panic("TavernActionDialog: exact detached provision review is required")
	}
}

func tavernProvisionReviewMatches(
	request game_state.TavernProvisionRequest,
	quote game_state.TavernProvisionQuote,
	pitch string,
) bool {
	return request.ReviewedMerchantID != "" && quote.Provision.Available &&
		strings.TrimSpace(pitch) != "" &&
		request.ReviewedProvision == quote.Provision && request.ReviewedPrice == quote.PricePerPack &&
		request.ReviewedGoldBefore == quote.GoldBefore && request.ReviewedFoodBefore == quote.FoodBefore
}

func tavernProvisionQuantityMatches(
	requestPackCount int,
	packCount int,
	quote game_state.TavernProvisionQuote,
) bool {
	if requestPackCount != packCount || packCount < tavernMinimumProvisionPacks ||
		packCount > quote.MaximumUsefulPacks || quote.MaximumUsefulPacks > tavernMaximumProvisionPacks ||
		!quote.HasValidApprovals() {
		return false
	}

	return true
}

func validateTavernInformationIntent(intent TavernActionIntent) {
	validateTavernInformationPitch(intent.InformationPitch)
	quote := intent.Information
	request := intent.Request.Information
	if request.ReviewedMerchantID == "" || strings.TrimSpace(quote.Topic.Key) == "" ||
		request.ReviewedTopic != quote.Topic || request.ReviewedPrice != quote.Price ||
		request.ReviewedGoldBefore != quote.GoldBefore || quote.Price <= 0 {
		panic("TavernActionDialog: exact authority-recognized information review is required")
	}
	wantAffordable := int(quote.GoldBefore) >= quote.Price
	wantAfter := quote.GoldBefore
	if wantAffordable {
		wantAfter -= uint16(quote.Price) //nolint:gosec // Validated canonical topic price is bounded.
	}
	if quote.Affordable != wantAffordable || quote.GoldAfterPayment != wantAfter {
		panic("TavernActionDialog: information gold projection is inconsistent")
	}
}

func validateTavernInformationPitch(pitch string) {
	if strings.TrimSpace(pitch) == "" {
		panic("TavernActionDialog: source-backed information pitch is required")
	}
}

func tavernActionControlsReleased(isKeyPressed func(ebiten.Key) bool) bool {
	for _, key := range []ebiten.Key{
		ebiten.KeyEscape, ebiten.KeyEnter, ebiten.KeyNumpadEnter,
		ebiten.KeyLeft, ebiten.KeyRight, ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyM,
	} {
		if isKeyPressed(key) {
			return false
		}
	}

	return true
}
