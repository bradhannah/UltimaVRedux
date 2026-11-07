package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
)

var (
	errInvalidCalendarHour   = errors.New("hour must be between 0 and 23")
	errInvalidCalendarMinute = errors.New("minute must be between 0 and 59")
	errInvalidCalendarDay    = errors.New("day must be between 1 and 28")
)

// CalendarChange describes an exact calendar mutation.
type CalendarChange struct {
	Before datetime.UltimaDate
	After  datetime.UltimaDate
}

// SetCalendarTime sets the exact hour and minute without advancing game time.
func (g *GameState) SetCalendarTime(hour, minute int) (CalendarChange, error) {
	change := CalendarChange{Before: g.DateTime, After: g.DateTime}
	if hour < 0 || hour >= datetime.HoursPerDay {
		return change, fmt.Errorf("%w: %d", errInvalidCalendarHour, hour)
	}
	if minute < 0 || minute >= datetime.MinutesPerHour {
		return change, fmt.Errorf("%w: %d", errInvalidCalendarMinute, minute)
	}

	g.DateTime.Hour = byte(hour)
	g.DateTime.Minute = byte(minute)
	change.After = g.DateTime

	return change, nil
}

// SetCalendarDay sets the day of the current month without advancing game time.
func (g *GameState) SetCalendarDay(day int) (CalendarChange, error) {
	change := CalendarChange{Before: g.DateTime, After: g.DateTime}
	if day < 1 || day > datetime.DaysInMonth {
		return change, fmt.Errorf("%w: %d", errInvalidCalendarDay, day)
	}

	g.DateTime.Day = byte(day)
	change.After = g.DateTime

	return change, nil
}
