package game_state

import (
	"errors"
	"fmt"
	"hash"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

var (
	// ErrDeathResurrectionInvalid means the party cannot enter the Guardian resurrection sequence.
	ErrDeathResurrectionInvalid = errors.New("invalid death resurrection request")
	// ErrDeathResurrectionStale means relevant party or world state changed after review.
	ErrDeathResurrectionStale = errors.New("death resurrection state changed after review")
)

const resurrectionTimeStepMinutes = 9

// DeathResurrectionMemberReview identifies one active member awaiting resurrection.
type DeathResurrectionMemberReview struct {
	Slot int
	Name string
}

// DeathResurrectionReview is a detached, stale-sensitive party-wipe snapshot.
type DeathResurrectionReview struct {
	Fingerprint  string
	KarmaAtDeath int
	Members      []DeathResurrectionMemberReview
}

// DeathResurrectionMemberOutcome reports one member's complete recovery.
type DeathResurrectionMemberOutcome struct {
	Slot   int
	Name   string
	Before ActiveCharacterSnapshot
	After  ActiveCharacterSnapshot
}

// DeathResurrectionOutcome reports the complete authoritative resurrection commit.
type DeathResurrectionOutcome struct {
	KarmaAtDeath   int
	Members        []DeathResurrectionMemberOutcome
	Destination    references.PlayerLocation
	DateTimeBefore datetime.UltimaDate
	DateTimeAfter  datetime.UltimaDate
}

// ReviewDeathResurrection captures all state changed by the Guardian resurrection.
func (g *GameState) ReviewDeathResurrection() (DeathResurrectionReview, error) {
	if g.IsInCombat() {
		return DeathResurrectionReview{}, fmt.Errorf("%w: combat is still active", ErrDeathResurrectionInvalid)
	}
	if !g.PartyState.IsAllPartyDead() {
		return DeathResurrectionReview{}, fmt.Errorf("%w: active party is not dead", ErrDeathResurrectionInvalid)
	}

	active := g.ActivePartyCharacters()
	if len(active) == 0 {
		return DeathResurrectionReview{}, fmt.Errorf("%w: active party is empty", ErrDeathResurrectionInvalid)
	}
	members := make([]DeathResurrectionMemberReview, 0, len(active))
	for _, character := range active {
		members = append(members, DeathResurrectionMemberReview{Slot: character.Slot, Name: character.Name})
	}

	review := DeathResurrectionReview{
		KarmaAtDeath: g.PartyState.Karma.Value,
		Members:      members,
	}
	review.Fingerprint = g.deathResurrectionFingerprint()

	return review, nil
}

// CommitDeathResurrection atomically applies penalty, recovery, relocation, and calendar advancement.
func (g *GameState) CommitDeathResurrection(
	expectedFingerprint string,
) (DeathResurrectionOutcome, error) {
	review, err := g.ReviewDeathResurrection()
	if err != nil {
		return DeathResurrectionOutcome{}, err
	}
	if expectedFingerprint == "" || expectedFingerprint != review.Fingerprint {
		return DeathResurrectionOutcome{}, ErrDeathResurrectionStale
	}

	outcome := DeathResurrectionOutcome{
		KarmaAtDeath:   review.KarmaAtDeath,
		Members:        make([]DeathResurrectionMemberOutcome, 0, len(review.Members)),
		Destination:    g.MapState.PlayerLocation,
		DateTimeBefore: g.DateTime,
		DateTimeAfter:  g.DateTime,
	}
	before := make(map[int]ActiveCharacterSnapshot, len(review.Members))
	for _, member := range review.Members {
		before[member.Slot] = snapshotActiveCharacter(member.Slot, g.PartyState.GetCharacter(member.Slot))
	}

	g.applyFullPartyDeathConsequences()
	for _, member := range review.Members {
		g.ResurrectCharacterWithFullRecovery(member.Slot)
		outcome.Members = append(outcome.Members, DeathResurrectionMemberOutcome{
			Slot:   member.Slot,
			Name:   member.Name,
			Before: before[member.Slot],
			After:  snapshotActiveCharacter(member.Slot, g.PartyState.GetCharacter(member.Slot)),
		})
	}

	// The original revives the party on foot: it resets the party object to the
	// player shape. A ship must not follow the party into the castle, where a
	// frigate under sail could neither move nor be exited ("Under sail!"). Horse
	// and carpet stay as they are; both can be dismounted where the party wakes.
	if vehicle := g.PartyVehicle.GetVehicleDetails().VehicleType; vehicle == references.FrigateVehicle ||
		vehicle == references.SkiffVehicle {
		g.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()
	}
	g.TransitionToLocation(
		g.GetResurrectionLocation(),
		g.GetResurrectionFloor(),
		g.GetResurrectionPosition(),
		TransitionFullLocation,
		false,
	)
	// Original party recovery rules advance normal calendar time in nine-minute steps until hour 6,
	// preserving the resulting minute and any crossed date boundaries.
	// See ref:RECOVERY-0002.
	for g.DateTime.Hour != ResurrectionHour {
		g.advanceElapsedGameplayMinutes(resurrectionTimeStepMinutes, false)
	}
	outcome.Destination = g.MapState.PlayerLocation
	outcome.DateTimeAfter = g.DateTime

	return outcome, nil
}

func (g *GameState) deathResurrectionFingerprint() string {
	roster := g.CurrentPartyRosterSnapshot()
	location := g.MapState.PlayerLocation
	lighting := &g.MapState.Lighting

	return reviewedMutationFingerprint(func(digest hash.Hash) {
		writeFingerprintf(
			digest,
			"%s:%d:%d:%d:%d:%d:%d:%d:%v:%d:%t:%v:%t",
			roster.Fingerprint,
			g.PartyState.Karma.Value,
			g.PartyState.Inventory.Provisions.Food.Get(),
			location.Location,
			location.Floor,
			location.Position.X,
			location.Position.Y,
			g.MagicLightMinutesRemaining,
			g.ActiveDurationEffect,
			lighting.TorchMinutesRemaining(),
			lighting.HasMagicLight(),
			g.DateTime,
			g.PartyState.IsAllPartyDead(),
		)
	})
}
