package references

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

var errSignsTooSmall = errors.New("SIGNS.DAT too small")

// SignRecord represents a single sign in the game world.
type SignRecord struct {
	Location Location // Small map location ID
	Floor    int      // Floor/level (0 = ground, -1 = basement/Underworld)
	X        int      // X coordinate within the map
	Y        int      // Y coordinate within the map
	RawText  []byte   // Raw text bytes (includes font encoding)
}

// SignReferences holds all sign data indexed by location.
type SignReferences struct {
	// SignsByLocation maps Location -> list of signs at that location
	SignsByLocation map[Location][]*SignRecord
}

// SIGNS.DAT format constants
const (
	signHeaderSize    = 66 // 33 locations * 2 bytes per offset
	signHeaderEntries = 33 // Number of location entries in header
	signRecordMinSize = 5  // onmap(1) + floor(1) + x(1) + y(1) + null terminator(1)
)

// Sign text encoding constants
// In Ultima V sign text, the high bit determines font type:
// - High bit clear (0x00-0x7F): Runic font characters
// - High bit set (0x80-0xFF): Normal IBM font characters (mask with 0x7F for index)
const (
	SignHighBitMask   = 0x80 // Mask to check/set high bit for font type
	SignCharIndexMask = 0x7F // Mask to get 7-bit character index (0-127)
	SignPauseMarker   = 0x0D // Wait for a key without moving the cursor; ref:SIGN-0004.
	SignMaxCharIndex  = 0x7F // Maximum valid character index (127)
)

// NewSignReferences loads and parses SIGNS.DAT from the given path.
func NewSignReferences(signsPath string) (*SignReferences, error) {
	data, err := os.ReadFile(signsPath)
	if err != nil {
		return nil, fmt.Errorf("reading SIGNS.DAT: %w", err)
	}

	if len(data) < signHeaderSize {
		return nil, fmt.Errorf("%w: expected at least %d bytes, got %d", errSignsTooSmall, signHeaderSize, len(data))
	}

	refs := &SignReferences{
		SignsByLocation: make(map[Location][]*SignRecord),
	}

	// Parse header - 33 little-endian 16-bit offsets
	// Each entry points to sign data for that location index
	offsets := make([]uint16, signHeaderEntries)
	for i := range signHeaderEntries {
		offsets[i] = binary.LittleEndian.Uint16(data[i*2 : i*2+2])
	}

	// Build sorted list of non-zero offsets to determine end boundaries
	sortedOffsets := make([]int, 0, signHeaderEntries)
	for _, off := range offsets {
		if off > 0 {
			sortedOffsets = append(sortedOffsets, int(off))
		}
	}
	// Sort offsets
	for i := range len(sortedOffsets) - 1 {
		for j := i + 1; j < len(sortedOffsets); j++ {
			if sortedOffsets[j] < sortedOffsets[i] {
				sortedOffsets[i], sortedOffsets[j] = sortedOffsets[j], sortedOffsets[i]
			}
		}
	}
	// Remove duplicates
	uniqueOffsets := make([]int, 0, len(sortedOffsets))
	for i, off := range sortedOffsets {
		if i == 0 || off != sortedOffsets[i-1] {
			uniqueOffsets = append(uniqueOffsets, off)
		}
	}

	// Parse signs for each unique offset
	for i, startOffset := range uniqueOffsets {
		// Determine end offset (next offset or end of file)
		endOffset := len(data)
		if i+1 < len(uniqueOffsets) {
			endOffset = uniqueOffsets[i+1]
		}

		// Parse signs between startOffset and endOffset
		signs := parseSignsInRange(data, startOffset, endOffset)

		for _, sign := range signs {
			loc := sign.Location
			refs.SignsByLocation[loc] = append(refs.SignsByLocation[loc], sign)
		}
	}

	return refs, nil
}

// parseSignsInRange parses all sign records within the given byte range.
func parseSignsInRange(data []byte, startOffset, endOffset int) []*SignRecord {
	var signs []*SignRecord

	pos := startOffset
	for pos < endOffset {
		// Check for end marker (three zero bytes)
		if pos+2 < len(data) && data[pos] == 0 && data[pos+1] == 0 && data[pos+2] == 0 {
			break
		}

		// Check minimum size
		if pos+signRecordMinSize > len(data) {
			break
		}

		sign := &SignRecord{
			Location: Location(data[pos]),
			Floor:    decodeSignFloor(data[pos+1]),
			X:        int(data[pos+2]),
			Y:        int(data[pos+3]),
		}
		pos += 4

		// Find null terminator for text
		textStart := pos
		for pos < len(data) && data[pos] != 0 {
			pos++
		}

		if pos > textStart {
			sign.RawText = make([]byte, pos-textStart)
			copy(sign.RawText, data[textStart:pos])
		}

		// Skip null terminator
		if pos < len(data) {
			pos++
		}

		signs = append(signs, sign)
	}
	resolveSignAliases(signs)

	return signs
}

func decodeSignFloor(encoded byte) int {
	if encoded == 0xff {
		return int(Basement)
	}

	return int(encoded)
}

// resolveSignAliases follows a leading 0x0A alias to another record; ref:SIGN-0002.
// The alias record reuses the next record's text but retains independent storage.
func resolveSignAliases(signs []*SignRecord) {
	for index := len(signs) - 2; index >= 0; index-- {
		if len(signs[index].RawText) != 1 || signs[index].RawText[0] != '\n' {
			continue
		}

		signs[index].RawText = append([]byte(nil), signs[index+1].RawText...)
	}
}

// GetSignAt returns the sign at the specified location and coordinates, or nil if none exists.
func (sr *SignReferences) GetSignAt(location Location, floor, x, y int) *SignRecord {
	signs, ok := sr.SignsByLocation[location]
	if !ok {
		return nil
	}

	for _, sign := range signs {
		if sign.Floor == floor && sign.X == x && sign.Y == y {
			return sign
		}
	}

	return nil
}

// GetBorderMacro returns the border string for a macro byte (0x29-0x31).
// Returns empty string if the byte is not a valid border macro.
func GetBorderMacro(b byte) string {
	if b >= 0x29 && b <= 0x31 {
		// Original border-control expansions; ref:SIGN-0004.
		// These are referenced by bytes 0x29-0x31 in sign text
		// All macros are exactly 16 characters to match the original message window width
		signBorderMacros := [9]string{
			"g              g", // 0x29 - side borders (vertical bars)
			"jlllllllnllllllk", // 0x2a - top border with center notch
			"8lllllllmllllll9", // 0x2b - bottom border with center notch
			"jllllllllllllllk", // 0x2c - top border plain
			"8llllllllllllll9", // 0x2d - bottom border plain
			"hllllk    jlllli", // 0x2e - cross arm (left side + gap + right side)
			"jlllli    hllllk", // 0x2f - cross arm mirrored
			"     g    g     ", //nolint:dupword // 0x30 - cross vertical bars with gap (glyph codes)
			"     hlllli     ", // 0x31 - cross bottom center
		}

		return signBorderMacros[b-0x29]
	}

	return ""
}

// IsBorderMacro returns true if the byte is a border macro (0x29-0x31).
func IsBorderMacro(b byte) bool {
	return b >= 0x29 && b <= 0x31
}

// IsSignBorderGlyph reports whether a raw runic-font byte selects one of the
// original frame-drawing glyphs used by literal and macro-expanded sign art.
func IsSignBorderGlyph(b byte) bool {
	return (b >= '8' && b <= ';') || (b >= 'a' && b <= 'n')
}

// IsRunicChar returns true if the byte represents a runic character (high bit clear).
func IsRunicChar(b byte) bool {
	return b&SignHighBitMask == 0
}

// GetCharIndex returns the character index (0-127) for rendering.
// For normal characters, this masks off the high bit.
func GetCharIndex(b byte) int {
	return int(b & SignCharIndexMask)
}
