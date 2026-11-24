package game_state

import (
	"errors"
	"fmt"
	"hash"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

var (
	// ErrWishingWellRequestInvalid means the requested well or wish is unavailable.
	ErrWishingWellRequestInvalid = errors.New("invalid wishing-well request")
	// ErrWishingWellRequestStale means relevant world or party state changed after review.
	ErrWishingWellRequestStale = errors.New("wishing-well request changed after review")
)

// WishingWellEffect identifies the committed gameplay result for presentation.
type WishingWellEffect int

const (
	// WishingWellNoEffect consumes the coin without changing world objects.
	WishingWellNoEffect WishingWellEffect = iota
	// WishingWellHorseEffect directly boards the wished-for horse.
	WishingWellHorseEffect
)

// WishingWellReview is a detached eligibility snapshot captured before wish input.
type WishingWellReview struct {
	Fingerprint  string
	WellPosition references.Position
	Gold         uint16
	CanAfford    bool
	Message      string
}

// WishingWellCommitRequest identifies the reviewed well and submitted wish.
type WishingWellCommitRequest struct {
	WellPosition        references.Position
	Wish                string
	ExpectedFingerprint string
}

// WishingWellOutcome contains the committed result and all presentation prose.
type WishingWellOutcome struct {
	Effect            WishingWellEffect
	GoldBefore        uint16
	GoldAfter         uint16
	ElapsedMinutes    int
	SpawnPosition     references.Position
	FeedbackMessage   string
	AppearanceMessage string
	FailureMessage    string
}

// ReviewWishingWell validates a well interaction and captures stale-sensitive state.
func (g *GameState) ReviewWishingWell(wellPosition references.Position) (WishingWellReview, error) {
	if !g.DetectWishingWellAtPosition(&wellPosition) {
		return WishingWellReview{}, fmt.Errorf("%w: no wishing well at the reviewed location", ErrWishingWellRequestInvalid)
	}

	review := WishingWellReview{
		WellPosition: wellPosition,
		Gold:         g.PartyState.Inventory.Gold.Get(),
		CanAfford:    g.PartyState.Inventory.Gold.Get() > 0,
	}
	if !review.CanAfford {
		review.Message = "Thou hast not enough gold!"
	}
	review.Fingerprint = g.wishingWellFingerprint(review)

	return review, nil
}

// CommitWishingWellWish revalidates and atomically commits the submitted wish.
func (g *GameState) CommitWishingWellWish(request WishingWellCommitRequest) (WishingWellOutcome, error) {
	failure := WishingWellOutcome{FailureMessage: "Thy wish can no longer be granted."}
	review, err := g.ReviewWishingWell(request.WellPosition)
	if err != nil {
		return failure, err
	}
	if request.ExpectedFingerprint == "" || request.ExpectedFingerprint != review.Fingerprint {
		return failure, ErrWishingWellRequestStale
	}
	if !review.CanAfford {
		failure.FailureMessage = review.Message

		return failure, fmt.Errorf("%w: insufficient gold", ErrWishingWellRequestInvalid)
	}

	outcome := WishingWellOutcome{
		Effect:          WishingWellNoEffect,
		GoldBefore:      review.Gold,
		SpawnPosition:   g.MapState.PlayerLocation.Position,
		FeedbackMessage: "No effect!",
	}
	// REDUX DEVIATION: wishing-well service deducts the coin before reading wish text.
	// Redux commits only a submitted, revalidated wish so cancellation has no cost.
	// See ref:WELL-0001.
	g.PartyState.Inventory.Gold.DecrementBy(1)
	outcome.GoldAfter = g.PartyState.Inventory.Gold.Get()
	if isWishingWellHorseWish(request.Wish) &&
		(g.MapState.PlayerLocation.Location == references.Paws || g.MapState.PlayerLocation.Location == references.EmpathAbbey) &&
		g.PartyVehicle.GetVehicleDetails().VehicleType == references.NoPartyVehicle {
		g.commitWishingWellHorse(&outcome)
	}

	return outcome, nil
}

func (g *GameState) commitWishingWellHorse(outcome *WishingWellOutcome) {
	// REDUX DEVIATION: Original wishing-well success places a horse east
	// of the party on two eligible maps. Redux boards it directly to avoid
	// a temporary map unit. ref:WELL-0001
	g.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(
		references.HorseVehicle,
		g.MapState.PlayerLocation.Position,
		g.MapState.PlayerLocation.Floor,
	)

	outcome.Effect = WishingWellHorseEffect
	outcome.FeedbackMessage = "Poof a Horse!"
	outcome.AppearanceMessage = "A horse appears!"
}

func (g *GameState) wishingWellFingerprint(review WishingWellReview) string {
	location := g.MapState.PlayerLocation
	vehicleType := g.PartyVehicle.GetVehicleDetails().VehicleType

	return reviewedMutationFingerprint(func(digest hash.Hash) {
		writeFingerprintf(
			digest,
			"%d:%d:%d:%d:%d:%d:%d:%d",
			location.Location,
			location.Floor,
			location.Position.X,
			location.Position.Y,
			review.WellPosition.X,
			review.WellPosition.Y,
			review.Gold,
			vehicleType,
		)
	})
}

func isWishingWellHorseWish(wish string) bool {
	normalized := strings.ToLower(strings.TrimSpace(wish))
	if normalized == "" {
		return false
	}
	aliases := [...]string{"corvette", "ferrari", "lamborghini", "lotus", "porsche", "horse", "mustang"}
	// REDUX DEVIATION: WELL-0001: original has six aliases and can skip overlapping
	// starts. Redux keeps Mustang and normalized Contains matching for predictable input.
	for _, alias := range aliases {
		if strings.Contains(normalized, alias) {
			return true
		}
	}

	return false
}
