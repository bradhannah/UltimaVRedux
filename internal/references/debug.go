package references

// DebugOptions are developer toggles; all default to off.
type DebugOptions struct {
	FreeMove                       bool
	MonsterGen                     bool
	LightOverlay                   bool // Show lighting/illumination debug overlay on map
	SearchOverlay                  bool // Show search location debug overlay on map
	ShowEmptyLineDebugPlaceholders bool // Show placeholder symbols for empty/newline lines in conversation text
	DialogBoundingBoxes            bool // Show red bounding boxes around text output areas in dialogs
	UnlimitedCastingResources      bool // Cast without prepared spells or current MP consumption
	BypassSpellLevelRestrictions   bool // Cast spells above the caster's current level
}
