package save_snapshot

// validateCircleStudyV28 checks that the studied roster slots are occupied,
// unique and ascending.
func validateCircleStudyV28(snapshot SaveSnapshot, add func(path, code, message string)) {
	for index, slot := range snapshot.CircleStudy {
		if slot < 0 || slot >= len(snapshot.Party.Roster) {
			add("circleStudy", "out_of_range", "studied roster slot must be from 0 to 15")

			continue
		}
		if !snapshot.Party.Roster[slot].Occupied {
			add("circleStudy", "invalid", "studied roster slot must be occupied")
		}
		if index > 0 && snapshot.CircleStudy[index-1] >= slot {
			add("circleStudy", "invalid", "studied roster slots must be unique and ascending")
		}
	}
}
