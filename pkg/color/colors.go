// Package color defines the EGA and semantic palettes and reusable color transformations.
package color

import "image/color"

// Alpha Constants for consistent transparency values
const (
	AlphaOpaque          = 255
	AlphaSemiTransparent = 128
	AlphaVeryTransparent = 60
	AlphaSubtle          = 40
	AlphaHeavy           = 200
	AlphaLight           = 100
)

// Color conversion constants
const (
	// maxUint16Float is the maximum value of a uint16 as float64.
	// Used for converting color.RGBA() output (0-65535 range) to 0.0-1.0 range.
	maxUint16Float = 65535.0
	// RGBA expands an 8-bit channel by repeating its byte into a 16-bit value.
	colorChannelExpansion = 0x101
	colorChannelBits      = 8
	maxColorChannel       = 255
)

// ============================================================================
// EGA COLOR TYPE
// ============================================================================

// EGAColor represents an EGA palette color with pixel matching capabilities.
// Implements color.Color interface for seamless compatibility with existing code.
type EGAColor struct {
	R, G, B, A uint8
}

// RGBA implements color.Color interface.
// Returns 16-bit premultiplied values as required by the interface.
func (c EGAColor) RGBA() (uint32, uint32, uint32, uint32) {
	return uint32(c.R) * colorChannelExpansion, uint32(c.G) * colorChannelExpansion,
		uint32(c.B) * colorChannelExpansion, uint32(c.A) * colorChannelExpansion
}

// MatchesWithTolerance returns true if RGB values match within the given tolerance.
func (c EGAColor) MatchesWithTolerance(r, g, b uint8, tolerance int) bool {
	return intAbs(int(c.R)-int(r)) <= tolerance &&
		intAbs(int(c.G)-int(g)) <= tolerance &&
		intAbs(int(c.B)-int(b)) <= tolerance
}

// ToRGBA returns this color as a standard color.RGBA for explicit conversion.
func (c EGAColor) ToRGBA() color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: c.A}
}

// intAbs returns the absolute value of an integer.
// Used for color tolerance comparisons.
func intAbs(x int) int {
	if x < 0 {
		return -x
	}

	return x
}

// Standard EGA 16-Color Palette
// These are the exact colors available in the original EGA standard
//
//goland:noinspection GoUnusedGlobalVariable
//nolint:gochecknoglobals // Fixed palette values are shared read-only; Go has no struct constants.
var (
	// Primary EGA Colors (0-7)
	EGABlack     = EGAColor{0, 0, 0, AlphaOpaque}       // 0: Black
	EGABlue      = EGAColor{0, 0, 170, AlphaOpaque}     // 1: Blue
	EGAGreen     = EGAColor{0, 170, 0, AlphaOpaque}     // 2: Green
	EGACyan      = EGAColor{0, 170, 170, AlphaOpaque}   // 3: Cyan
	EGARed       = EGAColor{170, 0, 0, AlphaOpaque}     // 4: Red
	EGAMagenta   = EGAColor{170, 0, 170, AlphaOpaque}   // 5: Magenta
	EGABrown     = EGAColor{170, 85, 0, AlphaOpaque}    // 6: Brown/Dark Yellow
	EGALightGray = EGAColor{170, 170, 170, AlphaOpaque} // 7: Light Gray

	// Bright EGA Colors (8-15)
	EGADarkGray     = EGAColor{85, 85, 85, AlphaOpaque}    // 8: Dark Gray
	EGALightBlue    = EGAColor{85, 85, 255, AlphaOpaque}   // 9: Light Blue
	EGALightGreen   = EGAColor{85, 255, 85, AlphaOpaque}   // 10: Light Green
	EGALightCyan    = EGAColor{85, 255, 255, AlphaOpaque}  // 11: Light Cyan
	EGALightRed     = EGAColor{255, 85, 85, AlphaOpaque}   // 12: Light Red
	EGALightMagenta = EGAColor{255, 85, 255, AlphaOpaque}  // 13: Light Magenta
	EGAYellow       = EGAColor{255, 255, 85, AlphaOpaque}  // 14: Yellow
	EGAWhite        = EGAColor{255, 255, 255, AlphaOpaque} // 15: White

	// Special
	EGATransparent = EGAColor{0, 0, 0, 0} // Fully transparent
)

// Legacy aliases for backward compatibility
//
//nolint:gochecknoglobals,mnd // Read-only compatibility palette; literal channel values preserve its exact colors.
var (
	Black                = EGABlack
	White                = EGAWhite
	Green                = color.RGBA{R: 0, G: 255, B: 0, A: 255}   // Non-EGA green kept for compatibility
	Red                  = color.RGBA{R: 255, G: 0, B: 0, A: 255}   // Non-EGA red kept for compatibility
	Yellow               = color.RGBA{R: 0, G: 255, B: 255, A: 255} // This appears to be cyan, kept for compatibility
	TextInputColor       = EGALightGray
	TextPlaceHolderColor = EGALightGray
	DialogInputTextColor = EGALightGreen // Light green for InputBox dialog text
)

// ExtractRGBA safely extracts RGBA components from any color.Color
// Returns R, G, B, A values as uint8 (0-255 range)
func ExtractRGBA(c color.Color) (uint8, uint8, uint8, uint8) {
	r, g, b, a := c.RGBA()

	// Mask the selected byte explicitly before narrowing, preserving the RGBA conversion.
	return uint8((r >> colorChannelBits) & maxColorChannel), uint8((g >> colorChannelBits) & maxColorChannel),
		uint8((b >> colorChannelBits) & maxColorChannel), uint8((a >> colorChannelBits) & maxColorChannel)
}

// ToRGBFloats extracts RGB components from a color.Color as float32 values (0.0-1.0).
// This is useful for bitmap font rendering with Ebitengine's ColorScale.
// Alpha channel is ignored; use ExtractRGBA if you need all four components.
func ToRGBFloats(c color.Color) (float32, float32, float32) {
	r16, g16, b16, _ := c.RGBA()

	return float32(r16) / maxUint16Float, float32(g16) / maxUint16Float, float32(b16) / maxUint16Float
}

// clampColorChannel narrows an intensity result only after enforcing its byte bound.
func clampColorChannel(value uint32) uint8 {
	if value > maxColorChannel {
		return maxColorChannel
	}

	return uint8(value)
}

// ColorBuilder provides a fluent interface for color transformations
// Allows chaining of operations like alpha adjustment, intensity changes, etc.
//
//nolint:revive // Preserve the established public ColorBuilder name used throughout presentation code.
type ColorBuilder struct {
	r, g, b, a uint8
}

// From creates a ColorBuilder from any color.Color
func From(c color.Color) *ColorBuilder {
	r, g, b, a := ExtractRGBA(c)

	return &ColorBuilder{r: r, g: g, b: b, a: a}
}

// Alpha sets the alpha (transparency) value (0-255)
func (cb *ColorBuilder) Alpha(alpha uint8) *ColorBuilder {
	return &ColorBuilder{r: cb.r, g: cb.g, b: cb.b, a: alpha}
}

// SemiTransparent sets alpha to 128 (semi-transparent)
func (cb *ColorBuilder) SemiTransparent() *ColorBuilder {
	return cb.Alpha(AlphaSemiTransparent)
}

// VeryTransparent sets alpha to 60 (very transparent)
func (cb *ColorBuilder) VeryTransparent() *ColorBuilder {
	return cb.Alpha(AlphaVeryTransparent)
}

// Subtle sets alpha to 40 (subtle transparency)
func (cb *ColorBuilder) Subtle() *ColorBuilder {
	return cb.Alpha(AlphaSubtle)
}

// Heavy sets alpha to 200 (heavy/strong transparency)
func (cb *ColorBuilder) Heavy() *ColorBuilder {
	return cb.Alpha(AlphaHeavy)
}

// Light sets alpha to 100 (light transparency)
func (cb *ColorBuilder) Light() *ColorBuilder {
	return cb.Alpha(AlphaLight)
}

// Opaque sets alpha to 255 (fully opaque)
func (cb *ColorBuilder) Opaque() *ColorBuilder {
	return cb.Alpha(AlphaOpaque)
}

// Intensity adjusts color brightness by multiplying RGB components
// factor: 0.0 = black, 1.0 = original color, >1.0 = brighter (clamped to 255)
func (cb *ColorBuilder) Intensity(factor float32) *ColorBuilder {
	newR := clampColorChannel(uint32(float32(cb.r) * factor))
	newG := clampColorChannel(uint32(float32(cb.g) * factor))
	newB := clampColorChannel(uint32(float32(cb.b) * factor))

	return &ColorBuilder{r: newR, g: newG, b: newB, a: cb.a}
}

// RGBA returns the final color.RGBA result
func (cb *ColorBuilder) RGBA() color.RGBA {
	return color.RGBA{R: cb.r, G: cb.g, B: cb.b, A: cb.a}
}

// Semantic Color Categories - ColorBuilder Variables
// UI Colors - Common interface elements (ColorBuilder instances for chaining)
//
//nolint:gochecknoglobals // Shared semantic palette; builder transformations return new values rather than mutating it.
var (
	// General UI
	UIBackground     = From(EGABlack)
	UIBackgroundSemi = From(EGABlack).SemiTransparent()
	UIBorder         = From(EGALightGray)
	UIBorderBright   = From(EGAWhite)
	UITextPrimary    = From(EGAWhite)
	UITextSecondary  = From(EGALightGray)
	UITextDisabled   = From(EGADarkGray)
	// Low stock remains readable on a dark supplies panel.
	SupplyLow = EGAYellow
	// Empty stock, paired with explicit OUT text.
	SupplyEmpty = EGALightRed

	// Button States
	ButtonDefault      = From(EGADarkGray)
	ButtonActive       = From(EGALightBlue)
	ButtonDisabled     = From(EGADarkGray).Light()
	ButtonText         = From(EGAWhite)
	ButtonTextActive   = From(EGAWhite)
	ButtonTextDisabled = From(EGADarkGray).Opaque()

	// Focus/Selection
	FocusIndicator     = From(EGAYellow)
	FocusIndicatorSemi = From(EGAYellow).VeryTransparent()
	SelectionHighlight = From(EGALightBlue).Light()
	SelectionBorder    = From(EGALightBlue)

	// Tab Interface
	TabActive     = From(EGALightBlue).Heavy()
	TabInactive   = From(EGADarkGray).Heavy()
	TabText       = From(EGAWhite)
	TabBackground = From(EGADarkGray)

	// Dialog Colors
	DialogBackground    = From(EGABlack)        // Black dialog background
	DialogTabDefault    = From(EGABlue).Heavy() // Dark EGA blue tab with transparency
	DialogTabActive     = From(EGALightBlue)    // EGA light blue for active tab
	DialogTextPrimary   = From(EGAWhite)        // White text for primary labels
	DialogTextSecondary = From(EGALightGray)    // Light gray for secondary text
)

// Visual Effect Colors
//
//nolint:gochecknoglobals // Shared read-only palette keeps visual effect identities consistent across callers.
var (

	// Map Fade/Transition
	MapFadeBlack = EGABlack // Base color for fades

)

// Overlay Effect Colors - ColorBuilder Variables for pulsing glows and auras
//
//nolint:gochecknoglobals,mnd // Read-only overlay palette; literal intensities and alpha values are visual tuning data.
var (
	// MoongateGlow is the pulsing blue glow for active moongates.
	// Uses EGA Light Blue with semi-transparency for mystical effect.
	MoongateGlow = From(EGALightBlue).SemiTransparent()

	// Creature Glow Colors - subtle magical auras for NPCs (additive blending)
	// Using non-light EGA base colors with low intensity to avoid washed-out look

	// WispGlow is an ethereal cyan glow for energy beings (fast flicker)
	WispGlow = From(EGACyan).Intensity(0.4).Alpha(50)

	// DaemonGlow is a fiery red glow for fire creatures (fire flicker)
	DaemonGlow = From(EGARed).Intensity(0.4).Alpha(50)

	// GhostGlow is a pale spectral glow for undead spirits (slow waver)
	GhostGlow = From(EGALightGray).Intensity(0.3).Alpha(40)

	// SeahorseGlow is a subtle aqua glow for mystical sea creatures
	SeahorseGlow = From(EGACyan).Intensity(0.4).Alpha(50)

	// Quest Item Glow Colors - subtle but noticeable for collectible objects
	// ShardGlow is a sinister purple glow for corrupted Shards of Evil
	ShardGlow = From(EGAMagenta).Intensity(0.4).Alpha(50)

	// ArtifactGlow is a noble golden glow for Lord British's Artifacts
	ArtifactGlow = From(EGABrown).Intensity(0.5).Alpha(60)

	// Combat Overlay Colors
	// These semantic colors are intentionally centralized so combat readability
	// can be tuned without changing rendering code.
	// The complete selected cursor stays white and opaque; target meaning and
	// recipient availability remain in the semantic readouts, not a colour pulse.
	CombatAimFrameFace       = EGAWhite
	CombatAimFrameBorder     = EGABlack
	CombatWakeMarkerFace     = EGALightMagenta
	CombatActiveAccentBorder = EGABlack

	CombatActivePCOutline          = EGALightGreen
	CombatInactivePCOutline        = EGABlack
	CombatActiveMonsterOutline     = EGAYellow
	CombatThreatenedPCOutline      = EGALightRed
	CombatPossessed                = EGALightBlue
	CombatUnderground              = EGABrown
	CombatUndergroundText          = EGAYellow
	CombatMaterialization          = EGALightMagenta
	CombatMaterializationHighlight = EGAWhite
	// Live combat side belongs to the health mark's border and explicit
	// Ally/Enemy text, never to the health strip or an upper-left actor symbol.
	// The enemy border and enemy text share one colour; a friend's border has
	// its own, because light blue text on black is too dim to read.
	CombatAllegianceAlly       = EGALightCyan
	CombatAllegianceEnemy      = EGALightRed
	CombatHealthFriend         = EGALightBlue
	CombatTeleportArrival      = EGALightCyan
	CombatAttackEligibleFace   = EGALightBlue
	CombatAttackEligibleBorder = EGABlack
	CombatDamageText           = EGALightRed
	CombatDamageShadow         = EGABlack
	CombatMissText             = EGAWhite
	CombatGrazeText            = EGAYellow
	CombatReportLoot           = EGALightGreen
	CombatReportWarning        = EGALightRed
)

// Debug Overlay Colors - ColorBuilder Variables
//
//nolint:gochecknoglobals,mnd // Read-only debug palette; alpha values are per-overlay visual tuning data.
var (
	// Debug Overlays
	DebugLightOverlay     = From(EGAYellow).Subtle()         // Yellow for light areas
	DebugSearchOverlay    = From(EGAGreen).VeryTransparent() // Green for searchable
	DebugMoonstoneOverlay = From(EGAMagenta).Light()         // Purple for moonstones
	DebugPathOverlay      = From(EGABlue).Subtle()           // Blue for pathfinding
	DebugCollisionOverlay = From(EGARed).Subtle()            // Red for collision

	// Search Overlay States
	DebugSearchUnsearched       = From(EGAGreen).VeryTransparent() // Green for unsearched locations
	DebugSearchAlreadyFound     = From(EGARed).VeryTransparent()   // Red for already searched locations
	DebugSearchBorderUnsearched = From(EGAGreen).Light()           // Green border for unsearched
	DebugSearchBorderFound      = From(EGARed).Light()             // Red border for searched

	// Moonstone Debug States
	MoonstoneActiveBright = From(EGALightMagenta).Alpha(120) // Bright purple for active moonstone
	MoonstoneFoundGreen   = From(EGALightGreen).Alpha(80)    // Green for found moonstone
	MoonstoneActiveBorder = From(EGAWhite).Alpha(180)        // White border for active moonstone

	// Debug UI
	DebugBackground = From(EGABlack).Opaque()
	DebugText       = From(EGAWhite)
	DebugBorder     = From(EGAWhite)
)

// Widget-Specific Colors - ColorBuilder Variables
//
//nolint:gochecknoglobals,mnd // Read-only widget palette; intensity and alpha literals encode each named visual style.
var (
	// Button
	ButtonTextDisabledGray = From(EGADarkGray).Heavy() // Gray text for disabled buttons

	// Slider
	SliderTrack      = From(EGABlue) // EGA blue track
	SliderHandle     = From(EGAWhite)
	SliderLabel      = From(EGAWhite)
	SliderBackground = From(EGADarkGray)
	SliderFocus      = From(EGAWhite) // Bright white for focus

	// List Widget
	ListHighlight   = From(EGABlue).Light()
	ListScrollTrack = From(EGADarkGray)
	ListScrollThumb = From(EGALightGray)

	// Loading Indicator
	LoadingBackground = From(EGADarkGray).Heavy()
	LoadingSpinner    = From(EGALightBlue)
	LoadingText       = From(EGAWhite)

	// Tabbed Interface
	TabActiveBlue     = From(EGALightBlue).Heavy()        // Light blue for active tabs
	TabInactiveGray   = From(EGADarkGray)                 // Gray for inactive tabs
	TabDisabledDim    = From(EGADarkGray).Intensity(0.4)  // Dim gray for empty/disabled tabs
	TabTextWhite      = From(EGAWhite)                    // White tab text
	TabTextDim        = From(EGALightGray).Intensity(0.6) // Dim text for disabled tabs
	TabBackgroundDark = From(EGADarkGray)                 // Dark gray tab background
	TabBorderDefault  = From(EGALightGray)                // Default tab border
	TabBorderActive   = From(EGAWhite)                    // Active tab border

	// Focus Indicator
	FocusOutline            = From(EGAWhite)             // White focus outline
	FocusIndicatorBlue      = From(EGALightBlue)         // Blue focus indicator
	FocusIndicatorSemiBlue  = From(EGALightBlue).Light() // Semi-transparent blue
	FocusHighContrastYellow = From(EGAYellow)            // High contrast yellow
	FocusBlackOutline       = From(EGABlack)             // Black outline for high contrast

	// NewOrder Dialog specific
	NewOrderAvatarBackground = From(EGABlue).Intensity(0.25).Alpha(128) // Very dark blue for Avatar
	NewOrderLockedBackground = From(EGABlue).Intensity(0.4).Alpha(200)  // Very dark blue for locked state (chosen)
	NewOrderNormalBackground = From(EGABlack).Alpha(64)                 // Semi-transparent for normal
)
