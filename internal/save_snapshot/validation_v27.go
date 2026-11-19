package save_snapshot

import "regexp"

const (
	// MaxDeedExperiencePaid bounds the saved record of paid quest deeds.
	MaxDeedExperiencePaid = 64
	// MaxSharedExperienceRemainder is one less than the largest active party.
	MaxSharedExperienceRemainder = 5
)

// validateExperienceRecordsV27 checks shape only; game state owns the catalog
// of deed keys and rejects an unknown one on hydration.
func validateExperienceRecordsV27(snapshot SaveSnapshot, add func(path, code, message string)) {
	keyPattern := regexp.MustCompile(`^[a-z]+(\.[a-z-]+)+$`)
	if len(snapshot.DeedExperiencePaid) > MaxDeedExperiencePaid {
		add("deedExperiencePaid", "invalid", "too many paid deeds")
	}
	for index, key := range snapshot.DeedExperiencePaid {
		if !keyPattern.MatchString(key) {
			add("deedExperiencePaid", "invalid", "paid deed key is malformed")
		}
		if index > 0 && snapshot.DeedExperiencePaid[index-1] >= key {
			add("deedExperiencePaid", "invalid", "paid deeds must be unique and ascending")
		}
	}
	if snapshot.SharedExperienceRemainder < 0 || snapshot.SharedExperienceRemainder > MaxSharedExperienceRemainder {
		add("sharedExperienceRemainder", "out_of_range", "remainder must be from 0 to 5")
	}
}
