package widgets

import (
	stdcolor "image/color"

	"github.com/bradhannah/UltimaVRedux/pkg/color"
)

// Keep the color construction centralized so merchant dialogs can be recolored together.
const merchantDialogBackgroundIntensity = 0.0

// Shared dialog color variables - used across all dialog widgets.
// These provide consistent styling for common dialog elements.
// Individual dialogs can define their own unexported colors for unique elements.
//
// Note: These are var not const because color.RGBA is a struct type.
// Treat as read-only constants at runtime.

//nolint:gochecknoglobals // Shared read-only palette; colour structs cannot be constants.
var (
	// =============================================================================
	// TEXT HIERARCHY COLORS
	// Used to establish visual hierarchy between different types of text content
	// =============================================================================

	// DialogHeaderColor - Panel and section titles that identify areas of the dialog
	// Examples: "CHARACTER", "EQUIPPED", "AVAILABLE", "DETAILS"
	// Visual role: Most prominent text, draws the eye to section boundaries
	DialogHeaderColor stdcolor.Color = color.EGAWhite

	// DialogLabelColor - Descriptive labels that identify what a value represents
	// Examples: "STR:", "ATK:", "Slot:", "STR Req:"
	// Visual role: Secondary prominence, pairs with values to form label:value pairs
	DialogLabelColor stdcolor.Color = color.EGAYellow

	// DialogValueColor - Data values that labels describe
	// Examples: "24" (after STR:), "Weapon" (after Slot:), character class names
	// Visual role: Neutral prominence, the actual information being displayed
	DialogValueColor stdcolor.Color = color.EGAWhite

	// MerchantDialogBackgroundColor is the shared backing for merchant shells and nested trade dialogs.
	//nolint:gochecknoglobals // Shared immutable semantic UI colour.
	MerchantDialogBackgroundColor stdcolor.Color = color.From(color.EGABlue).
					Intensity(merchantDialogBackgroundIntensity).Opaque().RGBA()

	// DialogEmptyColor - Placeholder text indicating absence of data
	// Examples: "(empty)", "No items", "No equipment", "[No character]"
	// Visual role: De-emphasized, indicates something is missing or unset
	DialogEmptyColor stdcolor.Color = color.EGADarkGray

	// DialogNavHintColor - Navigation position indicators
	// Examples: "< 1/6 >" showing current character out of total
	// Visual role: Subtle, informational but not the main focus
	DialogNavHintColor stdcolor.Color = color.EGADarkGray

	// =============================================================================
	// LIST ITEM COLORS
	// Used for scrollable lists of selectable items
	// =============================================================================

	// DialogItemNameColor - Primary text for items in a list
	// Examples: "Long Sword", "Plate Armor", "Healing Potion"
	// Visual role: Clear and readable, main content of list rows
	DialogItemNameColor stdcolor.Color = color.EGAWhite

	// DialogItemQuantityColor - Quantity indicators next to item names
	// Examples: "(2)", "(x3)", showing how many of an item exist
	// Visual role: Secondary to item name, additional detail
	DialogItemQuantityColor stdcolor.Color = color.EGALightGray

	// DialogAtlasDiscoveredColor marks places personally discovered by the party.
	//nolint:gochecknoglobals // Shared immutable semantic UI colour.
	DialogAtlasDiscoveredColor stdcolor.Color = color.EGALightGreen

	// DialogAtlasUndiscoveredColor marks supplied chart knowledge without personal discovery.
	//nolint:gochecknoglobals // Shared immutable semantic UI colour.
	DialogAtlasUndiscoveredColor stdcolor.Color = color.EGALightGray

	// DialogUnavailableItemColor dims Use, Ready and spell rows that also carry the
	// "[No]" text marker. Selected rows keep DialogSelectedText.
	//nolint:gochecknoglobals // Shared immutable semantic UI colour.
	DialogUnavailableItemColor stdcolor.Color = color.UITextDisabled.RGBA()

	// DialogScrollIndicator - Arrows showing more content above/below visible area
	// Examples: "^" at top, "v" at bottom of scrollable lists
	// Visual role: Distinct from content, indicates navigation possibility
	DialogScrollIndicator stdcolor.Color = color.EGACyan

	// =============================================================================
	// STATUS BAR COLORS
	// Used for the status/control bar at the bottom of dialogs
	// =============================================================================

	// DialogStatusOKColor - Positive status messages indicating ready/success state
	// Examples: "Ready to equip", "Spell prepared", "Transaction complete"
	// Visual role: Reassuring, indicates user can proceed
	DialogStatusOKColor stdcolor.Color = color.EGAGreen

	// DialogStatusWarningColor - Warning or error status messages
	// Examples: "Cannot equip - strength too low", "Not enough gold"
	// Visual role: Attention-grabbing, indicates a problem or blocker
	// Light red: dark EGA red is hard to read as text on black.
	DialogStatusWarningColor stdcolor.Color = color.EGALightRed

	// DialogControlKeyColor - Keyboard keys shown in control hints
	// Examples: "[Space]", "[Esc]", "[Up/Down]", "[</>/1-6]"
	// Visual role: Highlighted to stand out from hint descriptions
	DialogControlKeyColor stdcolor.Color = color.EGAWhite

	// DialogControlHintColor - Descriptions of what control keys do
	// Examples: ":Equip", ":Close", ":Item", ":Char"
	// Visual role: Secondary to keys, explains the action
	DialogControlHintColor stdcolor.Color = color.EGALightGray

	// =============================================================================
	// STAT COMPARISON COLORS
	// Used when showing how equipping/using something changes character stats
	// =============================================================================

	// DialogDeltaPositive - Stat improvements when comparing equipment
	// Examples: "(+5)", "(+12)" showing attack or defense increase
	// Visual role: Encouraging, this change is beneficial
	DialogDeltaPositive stdcolor.Color = color.EGALightGreen

	// DialogDeltaNegative - Stat reductions when comparing equipment
	// Examples: "(-3)", "(-8)" showing attack or defense decrease
	// Visual role: Cautionary, this change is detrimental
	DialogDeltaNegative stdcolor.Color = color.EGALightRed

	// DialogDeltaNeutral - No stat change when comparing equipment
	// Examples: "(0)" showing no difference
	// Visual role: Neutral, neither good nor bad
	DialogDeltaNeutral stdcolor.Color = color.EGAWhite

	// =============================================================================
	// INLINE WARNING COLORS
	// Used for warning text embedded within content (not status bar)
	// =============================================================================

	// DialogWarningColor - Inline warning text within dialog content
	// Examples: STR requirement value when character doesn't meet it
	// Visual role: Draws attention to a specific problem within the content
	DialogWarningColor stdcolor.Color = color.EGALightRed

	// =============================================================================
	// SELECTION HIGHLIGHT COLORS
	// Used for indicating the currently selected item in a list
	// =============================================================================

	// DialogSelectedBackground - Background color for the currently selected list item
	// Examples: White rectangle behind selected item in equipment/item lists
	// Visual role: Strong visual indicator of current selection (inversion style)
	DialogSelectedBackground stdcolor.Color = color.EGAWhite

	// DialogSelectedText - Text color for the currently selected list item
	// Examples: Black text on white background for selected item
	// Visual role: Inverted from normal to maintain readability on highlight
	DialogSelectedText stdcolor.Color = color.EGABlack

	// DialogActiveTabBackground - Plate behind the open tab's title in every tabbed dialog
	// Examples: Z-Stats, Journal, Map, Help and Configure tabs
	// Visual role: Inversion in the interface blue, so a tab never looks like a selected row
	DialogActiveTabBackground stdcolor.Color = color.EGALightBlue

	// DialogActiveTabText - Title of the open tab, drawn on DialogActiveTabBackground
	DialogActiveTabText stdcolor.Color = color.EGAWhite

	// DialogTabColor - Titles of the tabs that are not open
	DialogTabColor stdcolor.Color = color.EGADarkGray

	// DialogSelectedLabelColor separates labels from black values on the white selection bar.
	//nolint:gochecknoglobals // Shared immutable semantic UI colour.
	DialogSelectedLabelColor stdcolor.Color = color.EGADarkGray

	// DialogCheckedMarkColor distinguishes a selected ingredient from its brackets.
	//nolint:gochecknoglobals // Shared immutable semantic UI colour.
	DialogCheckedMarkColor stdcolor.Color = color.EGALightGreen

	// DialogSelectedCheckedMarkColor retains readable contrast on the white selection bar.
	//nolint:gochecknoglobals // Shared immutable semantic UI colour.
	DialogSelectedCheckedMarkColor stdcolor.Color = color.EGAGreen

	// =============================================================================
	// DECORATIVE COLORS
	// Used for visual separators and decorative elements
	// =============================================================================

	// DialogRuleColor - Horizontal or vertical separator lines
	// Examples: Line under item sprite in detail panel, section dividers
	// Visual role: Visual separation between content areas
	DialogRuleColor stdcolor.Color = color.EGAWhite

	// DialogRuleLightColor - Subtle separator lines for secondary divisions
	// Examples: Line between quantity sections, minor separators
	// Visual role: Lighter separation, less prominent than main rules
	DialogRuleLightColor stdcolor.Color = color.EGALightGray
)

// atlasNoteColor marks the player's own map notes: their flags, their tab and
// their action keys. No place family shares this ink.
func atlasNoteColor() stdcolor.Color {
	return color.EGALightMagenta
}
