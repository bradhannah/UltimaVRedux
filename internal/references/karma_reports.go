package references

import (
	"bytes"
	"errors"
	"fmt"
	"os"
)

const (
	karmaReportCount        = 6
	karmaBandSize           = 20
	karmaMaximum            = 99
	karmaTopBand            = 4
	karmaCampingTopReport   = 5
	karmaSecondReportOffset = 132
	karmaThirdReportOffset  = 269
	karmaFourthReportOffset = 410
	karmaFifthReportOffset  = 546
	karmaSixthReportOffset  = 671
)

var (
	errCorruptKarmaReports    = errors.New("corrupt KARMA.DAT")
	errKarmaOutsideRange      = errors.New("karma outside report range")
	errUnknownKarmaReportType = errors.New("unknown karma report context")
)

// KarmaReportContext distinguishes the original top-band report used by each caller.
type KarmaReportContext byte

const (
	// KarmaReportDeath selects the fifth report for the top karma band.
	KarmaReportDeath KarmaReportContext = iota
	// KarmaReportCamping selects the distinct sixth report for the top karma band.
	KarmaReportCamping
)

// KarmaReports contains the six original reports loaded from the user's game data.
type KarmaReports struct {
	reports [karmaReportCount]string
}

// NewKarmaReports loads the required KARMA.DAT report table.
func NewKarmaReports(path string) (*KarmaReports, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading required KARMA.DAT: %w", err)
	}

	reports, err := parseKarmaReports(data)
	if err != nil {
		return nil, fmt.Errorf("parsing KARMA.DAT: %w", err)
	}

	return reports, nil
}

// Select returns the authoritative five-band report for a death or camping context.
func (r *KarmaReports) Select(karma int, context KarmaReportContext) (string, error) {
	if r == nil {
		return "", fmt.Errorf("%w: reports are not loaded", errCorruptKarmaReports)
	}
	if karma < 0 || karma > karmaMaximum {
		return "", fmt.Errorf("%w: %d is outside 0-%d", errKarmaOutsideRange, karma, karmaMaximum)
	}

	report := karma / karmaBandSize
	switch context {
	case KarmaReportDeath:
	case KarmaReportCamping:
		if report == karmaTopBand {
			report = karmaCampingTopReport
		}
	default:
		return "", fmt.Errorf("%w %d", errUnknownKarmaReportType, context)
	}

	return r.reports[report], nil
}

func parseKarmaReports(data []byte) (*KarmaReports, error) {
	// Reports use KARMA.DAT offsets, without embedding the prose; ref:KARMA-0017.
	offsets := [...]int{
		0,
		karmaSecondReportOffset,
		karmaThirdReportOffset,
		karmaFourthReportOffset,
		karmaFifthReportOffset,
		karmaSixthReportOffset,
	}
	var reports KarmaReports
	for index, offset := range offsets {
		if offset >= len(data) {
			return nil, fmt.Errorf(
				"%w: report %d offset %d is outside %d-byte file",
				errCorruptKarmaReports, index+1, offset, len(data),
			)
		}

		limit := len(data)
		if index+1 < len(offsets) {
			limit = offsets[index+1]
		}
		relativeEnd := bytes.IndexByte(data[offset:limit], 0)
		if relativeEnd < 0 {
			return nil, fmt.Errorf(
				"%w: report %d has no null terminator before offset %d",
				errCorruptKarmaReports, index+1, limit,
			)
		}
		end := offset + relativeEnd
		if end == offset {
			return nil, fmt.Errorf("%w: report %d is empty", errCorruptKarmaReports, index+1)
		}
		if end+1 != limit {
			return nil, fmt.Errorf(
				"%w: report %d ends at %d; expected record boundary %d",
				errCorruptKarmaReports, index+1, end+1, limit,
			)
		}
		reports.reports[index] = string(data[offset:end])
	}

	return &reports, nil
}
