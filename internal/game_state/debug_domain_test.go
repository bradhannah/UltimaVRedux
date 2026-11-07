package game_state

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
)

// TestSetCalendarTimeSetsExactTimeWithoutAdvancing prevents debug clock edits from consuming a turn or simulating elapsed time.
func TestSetCalendarTimeSetsExactTimeWithoutAdvancing(t *testing.T) {
	initial := datetime.UltimaDate{Year: 3, Month: 4, Day: 5, Hour: 6, Minute: 7, Turn: 99}
	gs := &GameState{DateTime: initial, ElapsedMs: 1234}

	change, err := gs.SetCalendarTime(23, 59)
	if err != nil {
		t.Fatalf("SetCalendarTime() error = %v", err)
	}

	expected := initial
	expected.Hour = 23
	expected.Minute = 59
	if change.Before != initial || change.After != expected {
		t.Fatalf("SetCalendarTime() change = %+v, want before %+v after %+v", change, initial, expected)
	}
	if gs.DateTime != expected {
		t.Fatalf("SetCalendarTime() DateTime = %+v, want %+v", gs.DateTime, expected)
	}
	if gs.ElapsedMs != 1234 {
		t.Fatalf("SetCalendarTime() ElapsedMs = %d, want unchanged 1234", gs.ElapsedMs)
	}
}

// TestSetCalendarTimeRejectsInvalidValuesAtomically prevents integer conversion from wrapping invalid clock values into valid bytes.
func TestSetCalendarTimeRejectsInvalidValuesAtomically(t *testing.T) {
	tests := []struct {
		name   string
		hour   int
		minute int
	}{
		{name: "negative hour", hour: -1, minute: 0},
		{name: "hour too large", hour: 24, minute: 0},
		{name: "negative minute", hour: 0, minute: -1},
		{name: "minute too large", hour: 0, minute: 60},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			initial := datetime.UltimaDate{Year: 1, Month: 2, Day: 3, Hour: 4, Minute: 5, Turn: 6}
			gs := &GameState{DateTime: initial}

			change, err := gs.SetCalendarTime(test.hour, test.minute)
			if err == nil {
				t.Fatal("SetCalendarTime() error = nil, want validation error")
			}
			if gs.DateTime != initial || change.Before != initial || change.After != initial {
				t.Fatalf("SetCalendarTime() mutated state on error: DateTime=%+v change=%+v", gs.DateTime, change)
			}
		})
	}
}

// TestSetCalendarDaySetsExactDayWithoutChangingClockOrTurn prevents a day edit from changing unrelated calendar fields.
func TestSetCalendarDaySetsExactDayWithoutChangingClockOrTurn(t *testing.T) {
	initial := datetime.UltimaDate{Year: 3, Month: 4, Day: 5, Hour: 6, Minute: 7, Turn: 99}
	gs := &GameState{DateTime: initial}

	change, err := gs.SetCalendarDay(28)
	if err != nil {
		t.Fatalf("SetCalendarDay() error = %v", err)
	}

	expected := initial
	expected.Day = 28
	if change.Before != initial || change.After != expected || gs.DateTime != expected {
		t.Fatalf("SetCalendarDay() state/change = %+v/%+v, want %+v", gs.DateTime, change, expected)
	}
}

// TestSetCalendarDayRejectsInvalidValuesAtomically prevents invalid Britannian month days from entering game state.
func TestSetCalendarDayRejectsInvalidValuesAtomically(t *testing.T) {
	for _, day := range []int{0, 29} {
		initial := datetime.UltimaDate{Year: 1, Month: 2, Day: 3, Hour: 4, Minute: 5, Turn: 6}
		gs := &GameState{DateTime: initial}

		change, err := gs.SetCalendarDay(day)
		if err == nil {
			t.Fatalf("SetCalendarDay(%d) error = nil, want validation error", day)
		}
		if gs.DateTime != initial || change.Before != initial || change.After != initial {
			t.Fatalf("SetCalendarDay(%d) mutated state on error: DateTime=%+v change=%+v", day, gs.DateTime, change)
		}
	}
}
