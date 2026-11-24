// Package datetime owns the Britannian calendar and calendar-derived presentation phases.
package datetime

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/logging"
)

// Calendar dimensions and the maximum supported interval per advancement call.
const (
	DaysInMonth           = 28
	MonthsPerYear         = 13
	MinutesPerHour        = 60
	HoursPerDay           = 24
	MaximumAdvanceMinutes = MinutesPerHour * 9
)

//nolint:gochecknoglobals // Immutable day-indexed lunar phase data; ref:MOON-0002.
var moonPhases = [DaysInMonth][2]MoonPhase{
	{0, 0}, {1, 0}, {1, 1}, {2, 2}, {2, 3}, {3, 4}, {3, 5}, {4, 6},
	{5, 7}, {5, 0}, {6, 0}, {6, 1}, {7, 2}, {7, 3}, {0, 4}, {1, 5},
	{1, 6}, {2, 7}, {2, 0}, {3, 0}, {3, 1}, {4, 2}, {5, 3}, {5, 4},
	{6, 5}, {6, 6}, {7, 7}, {7, 0},
}

const (
	hourOfSunrise = 5
	hourOfNoon    = 12
	hourOfSunset  = 20

	ambientTintTransitionMinutes = 30
)

// MoonPhase represents the 8 moon phases for moongate transportation
// Based on original Ultima V moon phase system (0-7)
type MoonPhase int

// Lunar phases in their original cyclic order.
const (
	MoonPhaseNew            MoonPhase = iota // 0
	MoonPhaseCrescentWaxing                  // 1
	MoonPhaseFirstQuarter                    // 2
	MoonPhaseGibbousWaxing                   // 3
	MoonPhaseFull                            // 4
	MoonPhaseGibbousWaning                   // 5
	MoonPhaseLastQuarter                     // 6
	MoonPhaseCrescentWaning                  // 7
)

// String returns the string representation of the moon phase
func (mp MoonPhase) String() string {
	phases := []string{
		"New Moon",
		"Crescent Waxing",
		"First Quarter",
		"Gibbous Waxing",
		"Full Moon",
		"Gibbous Waning",
		"Last Quarter",
		"Crescent Waning",
	}
	if int(mp) >= 0 && int(mp) < len(phases) {
		return phases[mp]
	}

	return "Unknown Phase"
}

// ToMoonstoneIndex converts moon phase to moonstone index (0-7)
// Each moon phase corresponds to a specific moonstone for moongate travel
func (mp MoonPhase) ToMoonstoneIndex() int {
	return int(mp)
}

// UltimaDate records the authoritative calendar and turn count.
type UltimaDate struct {
	Year   uint16
	Month  byte
	Day    byte
	Hour   byte
	Minute byte
	Turn   uint32
}

// BritannianDate is an in-game calendar date without time-of-day or turn state.
type BritannianDate struct {
	Year  uint16
	Month byte
	Day   byte
}

// DateOnly returns the Britannian calendar date represented by d.
func (d UltimaDate) DateOnly() BritannianDate {
	return BritannianDate{Year: d.Year, Month: d.Month, Day: d.Day}
}

// IsValid reports whether the date is inside the Britannian calendar range.
func (d BritannianDate) IsValid() bool {
	return d.Year >= 1 && d.Month >= 1 && d.Month <= MonthsPerYear && d.Day >= 1 && d.Day <= DaysInMonth
}

// GetDateAsString formats the calendar as month-day-year.
func (d *UltimaDate) GetDateAsString() string {
	return fmt.Sprintf("%d-%d-%d", d.Month, d.Day, d.Year)
}

// GetTimeAsString formats the clock with an AM/PM suffix.
func (d *UltimaDate) GetTimeAsString() string {
	hour := d.Hour
	isAM := true
	if hour >= hourOfNoon {
		hour -= hourOfNoon
		isAM = false
	}

	if hour == 0 {
		hour = hourOfNoon
	}

	if isAM {
		return fmt.Sprintf("%2d:%02dAM", hour, d.Minute)
	}

	return fmt.Sprintf("%2d:%02dPM", hour, d.Minute)
}

// Advance applies one bounded calendar interval and increments the turn count.
func (d *UltimaDate) Advance(nMinutes int) {
	d.Turn++

	// Validate time advancement does not exceed 9 hours (for time-saving assumptions)
	if nMinutes > MaximumAdvanceMinutes {
		logging.Warnf(logging.CategoryGameState, "Time advancement clamped to 9 hours (requested: %d minutes)", nMinutes)
		nMinutes = MaximumAdvanceMinutes
	}
	if nMinutes < 0 {
		// Negative advancement is outside the current contract; retain its legacy behavior.
		d.Minute += byte(nMinutes)

		return
	}

	totalMinutes := int(d.Minute) + nMinutes
	newHour := int(d.Hour) + totalMinutes/MinutesPerHour
	d.Minute = byte(totalMinutes % MinutesPerHour) //nolint:gosec // Nonnegative modulo is bounded to 0..59.
	//nolint:gosec // Valid date plus nonnegative advancement remains bounded.
	d.Hour = byte(newHour % HoursPerDay)
	if newHour < HoursPerDay {
		return
	}

	// The nine-hour clamp means a valid starting date crosses at most one day.
	nDay := d.Day + 1
	if nDay > DaysInMonth {
		d.Day = 1
		nMonth := d.Month + 1
		if nMonth > MonthsPerYear {
			d.Month = 1
			d.Year++
		} else {
			d.Month = nMonth
		}
	} else {
		d.Day = nDay
	}
}

// IsDayLight reports the broad daytime window, including twilight.
func (d *UltimaDate) IsDayLight() bool {
	return d.Hour >= hourOfSunrise && d.Hour < hourOfSunset
}

// IsNight reports whether the clock is outside the daytime window.
func (d *UltimaDate) IsNight() bool {
	return !d.IsDayLight()
}

// IsFullDaylight returns true during full daylight hours (after dawn, before dusk).
// Ordinary original ambient sight reaches 50 only during hours 6-18 (ref:LIGHT-0001).
// Cosmetic dusk interpolation is independent of this lighthouse activation boundary.
func (d *UltimaDate) IsFullDaylight() bool {
	return d.Hour > hourOfSunrise && d.Hour < hourOfSunset-1
}

// IsVisibilityDaylight preserves Redux's daylight visibility window separately
// from the original lighthouse sight threshold. Cosmetic dusk does not reduce LOS.
func (d *UltimaDate) IsVisibilityDaylight() bool {
	return d.Hour > hourOfSunrise && d.Hour < hourOfSunset
}

// GetAmbientTintDarkness returns the cosmetic night-tint strength from 0 to 1.
// REDUX DEVIATION: Original sight changes in steps during hours 5 (ref:LIGHT-0001)
// and 19. Redux fades only the ambient tint during 05:00-05:30 and 19:30-20:00
// so dawn and dusk change smoothly without altering tile visibility.
func (d *UltimaDate) GetAmbientTintDarkness() float32 {
	switch {
	case d.Hour == hourOfSunrise && d.Minute < ambientTintTransitionMinutes:
		return 1 - float32(d.Minute)/ambientTintTransitionMinutes
	case d.Hour == hourOfSunset-1 && d.Minute >= MinutesPerHour-ambientTintTransitionMinutes:
		return float32(int(d.Minute)-(MinutesPerHour-ambientTintTransitionMinutes)) /
			ambientTintTransitionMinutes
	case d.Hour < hourOfSunrise || d.Hour >= hourOfSunset:
		return 1
	default:
		return 0
	}
}

// GetVisibilityFactorWithoutTorch returns a 0–1 visibility factor
// (despite the word “Percent” in the name).
func (d *UltimaDate) GetVisibilityFactorWithoutTorch(baselineMin float32) float32 {
	switch {
	case d.Hour == hourOfSunrise:
		// Dawn: 0 → 1 hour
		frac := float32(d.Minute) / float32(MinutesPerHour) // 0‥1
		val := baselineMin + (1-baselineMin)*frac           // 0.1‥1.0

		return min(val, 1) // cheap safety‑clamp

	case d.Hour == hourOfSunset:
		// Dusk: 1 → 0 hour
		frac := float32(d.Minute) / float32(MinutesPerHour) // 0‥1
		val := 1 - (1-baselineMin)*frac                     // 1.0‥0.1

		return max(val, baselineMin) // keep ≥ baseline

	case d.Hour > hourOfSunrise && d.Hour < hourOfSunset:
		return 1 // full daylight

	default:
		return baselineMin // night
	}
}

// GetCurrentMoonPhase returns Trammel's phase for compatibility with displays
// that represent one moon. Moongate routing uses GetMoongateMoonPhase.
func (d *UltimaDate) GetCurrentMoonPhase() MoonPhase {
	return d.GetTrammelMoonPhase()
}

// GetTrammelMoonPhase returns Trammel's phase for the current day; ref:MOON-0002.
func (d *UltimaDate) GetTrammelMoonPhase() MoonPhase {
	return moonPhases[d.GetTotalDays()%DaysInMonth][0]
}

// GetFeluccaMoonPhase returns Felucca's phase for the current day; ref:MOON-0002.
func (d *UltimaDate) GetFeluccaMoonPhase() MoonPhase {
	return moonPhases[d.GetTotalDays()%DaysInMonth][1]
}

// GetMoongateMoonPhase selects Trammel before noon and Felucca from noon onward; ref:MOON-0003.
func (d *UltimaDate) GetMoongateMoonPhase() MoonPhase {
	if d.Hour < hourOfNoon {
		return d.GetTrammelMoonPhase()
	}

	return d.GetFeluccaMoonPhase()
}

// GetTotalDays returns the total number of days since the start of the game.
// Ultima V uses Year 1, Month 1, Day 1 as the epoch.
func (d *UltimaDate) GetTotalDays() int {
	return int(d.Day-1) + int(d.Month-1)*DaysInMonth + int(d.Year-1)*DaysInMonth*MonthsPerYear
}
