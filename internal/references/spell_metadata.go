package references

import "strings"

// spell_metadata.go - Spell targeting, context restrictions, and descriptions
// Original spell context permissions; ref:SPELL-0002.

// Spell system constants from original Ultima V
const (
	TotalSpellCount   = 48 // Total number of spells in Ultima V
	SpellsPerCircle   = 6  // Spells per circle (1-8)
	MaxSpellCircle    = 8  // Highest spell circle
	MaxSpellSyllables = 4  // Maximum syllables in any spell name
)

// TargetingType defines how a player selects a spell target. EffectScope is
// recorded separately because a global or random effect may need no selection.
type TargetingType int

// Supported spell target-selection interactions.
const (
	TargetAutomatic          TargetingType = iota // No player target prompt.
	TargetPartyMember                             // "On who?" party-member prompt.
	TargetDirection                               // Cardinal direction prompt.
	TargetAdjacentTile                            // Adjacent combat tile selector.
	TargetCombatantCrosshair                      // Combat crosshair requiring a combatant.
	TargetTileCrosshair                           // Combat crosshair selecting a legal tile.
	TargetMoonPhase                               // Moon phase 1-8 prompt.
	TargetCombatPartyMember                       // Visible exact PC entry on the battlefield.
)

func (t TargetingType) String() string {
	switch t {
	case TargetAutomatic:
		return "Automatic"
	case TargetPartyMember:
		return "Character"
	case TargetCombatPartyMember:
		return "Battlefield recipient"
	case TargetDirection:
		return "Direction"
	case TargetAdjacentTile:
		return "Adjacent tile"
	case TargetCombatantCrosshair:
		return "Combatant crosshair"
	case TargetTileCrosshair:
		return "Tile crosshair"
	case TargetMoonPhase:
		return "Moon phase"
	default:
		return "Unknown"
	}
}

// PlayerLabel says how the caster aims the spell, in a player's words. String
// keeps the plain names that logs and the Workbench use. A spell with no aiming
// step has no aim to describe; SpellTargetLabel says what it falls on instead.
func (t TargetingType) PlayerLabel() string {
	switch t {
	case TargetCombatPartyMember:
		return "A companion in the fight"
	case TargetAdjacentTile:
		return "A square beside you"
	case TargetCombatantCrosshair:
		return "A creature you pick"
	case TargetTileCrosshair:
		return "A square you pick"
	case TargetPartyMember, TargetDirection, TargetMoonPhase:
		return t.String() // Already a player's word.
	case TargetAutomatic:
		return t.String() // Never shown: SpellTargetLabel answers for an unaimed spell.
	default:
		return t.String()
	}
}

// SpellTargetLabel is the spell details' Target line for one context, in a
// player's words: how the spell is aimed there or, for a spell cast with no
// aiming step, who or what it falls on.
func SpellTargetLabel(spell Spell, context SpellContext) string {
	if selection := GetSpellTargetingProfile(spell, context).Selection; selection != TargetAutomatic {
		return selection.PlayerLabel()
	}
	//nolint:exhaustive // Only spells that can be cast with no aiming step are listed.
	switch spell {
	case InQuasCorp, InVasPorYlem, QuasAnWis:
		return "All foes"
	case AnXenCorp:
		return "All undead foes"
	case WisQuas:
		return "All unseen foes"
	case AnTym:
		return "All but the party"
	case InAn:
		return "All magic, yours too"
	case RelTym, InLor, VasLor, InSanct, UusPor, DesPor:
		return "The whole party"
	case InXenMani:
		return "The party's food"
	case SanctLor, InPor: // In Por needs no aim only in battle, where it moves its caster.
		return "The caster"
	case InWis:
		return "Where you stand"
	case InQuasWis:
		return "The area around you"
	case WisAnYlem:
		return "The walls around you"
	case KalXen, InBetXen, KalXenCorp:
		return "Somewhere on the battlefield"
	case AnSanct, AnGrav, InFlamGrav, InNoxGrav, InZuGrav, InSanctGrav:
		return "The square ahead" // Unaimed only in a dungeon, where they follow the party's facing.
	default:
		panic("spell target label: an unaimed spell has no label")
	}
}

// SpellEffectScope describes what a spell can affect after targeting resolves.
type SpellEffectScope int

// Supported spell effect scopes.
const (
	ScopeCaster SpellEffectScope = iota // Effect applies to the caster.
	ScopeParty
	ScopeSingleCharacter
	ScopeSingleCombatant
	ScopeSingleTile
	ScopeCone
	ScopeGlobal
	ScopeRandomPlacement
	ScopeWorldTransition
)

// SpellTargetingProfile combines player selection with effect scope for one context.
type SpellTargetingProfile struct {
	Selection TargetingType
	Scope     SpellEffectScope
}

// GetSpellTargetingProfile resolves context-specific selection and effect scope.
func GetSpellTargetingProfile(spell Spell, context SpellContext) SpellTargetingProfile {
	info := GetSpellInfo(spell)
	if info == nil {
		return SpellTargetingProfile{}
	}
	profile := SpellTargetingProfile{Selection: info.Targeting, Scope: info.Scope}
	if context == ContextDungeon {
		// Dungeon packed-cell spells use the party's existing facing and never
		// open a separate direction selector.
		//nolint:exhaustive // Only dungeon interaction overrides are listed.
		switch spell {
		case AnSanct, InFlamGrav, InNoxGrav, InZuGrav, InSanctGrav, AnGrav:
			profile.Selection, profile.Scope = TargetAutomatic, ScopeSingleTile
		}

		return profile
	}
	if context != ContextCombat {
		return profile
	}
	//nolint:exhaustive // Only combat interaction overrides are listed.
	switch spell {
	case Mani, AnNox, AnZu, VasMani:
		// REDUX DEVIATION: Original character targeting selects a roster number (ref:SPELL-0006). Combat uses
		// visible battlefield sprites for identity/readability, retaining roster effects.
		profile.Selection, profile.Scope = TargetCombatPartyMember, ScopeSingleCharacter
	case InPor:
		profile.Selection, profile.Scope = TargetAutomatic, ScopeCaster
	case InFlamGrav, InNoxGrav, InZuGrav, InSanctGrav:
		profile.Selection, profile.Scope = TargetTileCrosshair, ScopeSingleTile
	case AnGrav, AnYlem, AnSanct, AnExPor, InExPor:
		profile.Selection, profile.Scope = TargetAdjacentTile, ScopeSingleTile
	case InZu, InNoxHur, InFlamHur, InVasGravCorp:
		profile.Selection, profile.Scope = TargetDirection, ScopeCone
	}

	return profile
}

// SpellContext defines where a spell can be cast
// Uses a bitmask for the permitted casting contexts; ref:SPELL-0002.
type SpellContext uint8

// Casting context flags identify supported spell environments; ref:SPELL-0002.
const (
	ContextOverworld SpellContext = 1 << iota // Outdoor casting.
	ContextTown                               // Town casting.
	ContextDungeon                            // First-person dungeon casting.
	ContextCombat                             // Tactical casting.
)

// ContextAll is a convenience constant for spells usable everywhere
const ContextAll = ContextOverworld | ContextTown | ContextDungeon | ContextCombat

// ContextNonCombat is a convenience constant for spells usable outside combat
const ContextNonCombat = ContextOverworld | ContextTown | ContextDungeon

func (c SpellContext) String() string {
	if c == ContextAll {
		return "All"
	}
	if c == ContextNonCombat {
		return "Non-Combat"
	}

	var contexts []string
	if c&ContextOverworld != 0 {
		contexts = append(contexts, "Overworld")
	}
	if c&ContextTown != 0 {
		contexts = append(contexts, "Town")
	}
	if c&ContextDungeon != 0 {
		contexts = append(contexts, "Dungeon")
	}
	if c&ContextCombat != 0 {
		contexts = append(contexts, "Combat")
	}

	if len(contexts) == 0 {
		return "None"
	}

	result := contexts[0]
	var resultSb160 strings.Builder
	for i := 1; i < len(contexts); i++ {
		resultSb160.WriteString(", " + contexts[i])
	}
	result += resultSb160.String()

	return result
}

// PlayerLabel says where the spell may be cast, in a player's words and as a
// place: "In battle", "On the overworld or in town". String keeps the plain
// names that logs and the Workbench use.
func (c SpellContext) PlayerLabel() string {
	if c == ContextAll {
		return "Anywhere"
	}
	if c == ContextNonCombat {
		return "Outside battle"
	}
	label := joinSpellPlaces(c.places(true))

	return strings.ToUpper(label[:1]) + label[1:]
}

// OnlyLabel says the same to a caster standing in the wrong place, after
// "Cannot be cast here:": "battle only", "overworld or town".
func (c SpellContext) OnlyLabel() string {
	if c == ContextNonCombat {
		return "not in battle"
	}
	places := c.places(false)
	if len(places) == 1 {
		return places[0] + " only"
	}

	return joinSpellPlaces(places)
}

// places names each permitted context, bare ("town") or as a place ("in town").
func (c SpellContext) places(placed bool) []string {
	names := [...][2]string{
		{"overworld", "on the overworld"}, {"town", "in town"}, {"dungeon", "in a dungeon"}, {"battle", "in battle"},
	}
	form := 0
	if placed {
		form = 1
	}
	var places []string
	for bit, name := range names {
		if c&(SpellContext(1)<<bit) != 0 {
			places = append(places, name[form])
		}
	}

	return places
}

func joinSpellPlaces(places []string) string {
	last := len(places) - 1
	if last < 0 {
		return "nowhere"
	}
	if last == 0 {
		return places[0]
	}

	return strings.Join(places[:last], ", ") + " or " + places[last]
}

// HasContext returns true if the spell context includes the specified context
func (c SpellContext) HasContext(ctx SpellContext) bool {
	return c&ctx != 0
}

// SpellInfo contains all metadata for a spell
type SpellInfo struct {
	Spell       Spell         // The spell constant
	Name        string        // Display name (e.g., "In Lor")
	Circle      int           // Spell circle 1-8 (determines MP cost and level requirement)
	Targeting   TargetingType // How the spell selects its target
	Scope       SpellEffectScope
	Context     SpellContext // Where the spell can be cast
	Description string       // Brief description of the spell effect
}

// GetSpellCircle returns the circle (level) of a spell.
// Circle = (spell_index / SpellsPerCircle) + 1
// MP cost equals circle, and caster level must >= circle.
func GetSpellCircle(spell Spell) int {
	if spell < 0 || spell >= TotalSpellCount {
		return 0
	}

	return int(spell)/SpellsPerCircle + 1
}

// spellMetadata contains metadata for all 48 spells
// Indexed by Spell constant for O(1) lookup
var spellMetadata = [TotalSpellCount]SpellInfo{ //nolint:gochecknoglobals // Immutable catalog with compact readable rows.
	// Circle 1 (Spells 0-5)
	{InLor, "In Lor", 1, TargetAutomatic, ScopeParty, ContextNonCombat, "Creates magical light"},
	{GravPor, "Grav Por", 1, TargetTileCrosshair, ScopeSingleCombatant, ContextCombat, "Energy missile attack"},
	{AnZu, "An Zu", 1, TargetPartyMember, ScopeSingleCharacter, ContextAll, "Awakens sleeping character"},
	{AnNox, "An Nox", 1, TargetPartyMember, ScopeSingleCharacter, ContextAll, "Cures poison"},
	{Mani, "Mani", 1, TargetPartyMember, ScopeSingleCharacter, ContextAll, "Heals minor wounds"},
	{AnYlem, "An Ylem", 1, TargetDirection, ScopeSingleTile, ContextTown | ContextCombat, "Destroys a movable obstacle beside you, such as furniture"},

	// Circle 2 (Spells 6-11)
	{AnSanct, "An Sanct", 2, TargetDirection, ScopeSingleTile, ContextAll, "Unlocks mundane doors and disarms chests"},
	{AnXenCorp, "An Xen Corp", 2, TargetAutomatic, ScopeGlobal, ContextCombat, "Repels undead"},
	{RelHur, "Rel Hur", 2, TargetDirection, ScopeGlobal, ContextOverworld, "Changes wind direction"},
	{InWis, "In Wis", 2, TargetAutomatic, ScopeCaster, ContextOverworld, "Reveals where in the world you stand"},
	{KalXen, "Kal Xen", 2, TargetAutomatic, ScopeRandomPlacement, ContextCombat, "Summons creature"},
	{InXenMani, "In Xen Mani", 2, TargetAutomatic, ScopeParty, ContextAll, "Creates food"},

	// Circle 3 (Spells 12-17)
	{VasLor, "Vas Lor", 3, TargetAutomatic, ScopeParty, ContextNonCombat, "Creates great light"},
	{VasFlam, "Vas Flam", 3, TargetTileCrosshair, ScopeSingleCombatant, ContextCombat, "Fireball attack"},
	{InFlamGrav, "In Flam Grav", 3, TargetDirection, ScopeSingleTile, ContextDungeon | ContextCombat, "Creates fire field"},
	{InNoxGrav, "In Nox Grav", 3, TargetDirection, ScopeSingleTile, ContextDungeon | ContextCombat, "Creates poison field"},
	{InZuGrav, "In Zu Grav", 3, TargetDirection, ScopeSingleTile, ContextDungeon | ContextCombat, "Creates sleep field"},
	{InPor, "In Por", 3, TargetDirection, ScopeWorldTransition, ContextOverworld | ContextCombat, "Whisks you a short way from where you stand"},

	// Circle 4 (Spells 18-23)
	{AnGrav, "An Grav", 4, TargetDirection, ScopeSingleTile, ContextDungeon | ContextCombat, "Dispels magic field"},
	{InSanct, "In Sanct", 4, TargetAutomatic, ScopeParty, ContextAll, "Grants party protection"},
	{InSanctGrav, "In Sanct Grav", 4, TargetDirection, ScopeSingleTile, ContextDungeon | ContextCombat, "Creates protection field"},
	{UusPor, "Uus Por", 4, TargetAutomatic, ScopeWorldTransition, ContextDungeon, "Carries the party up one level of a dungeon"},
	{DesPor, "Des Por", 4, TargetAutomatic, ScopeWorldTransition, ContextDungeon, "Carries the party down one level of a dungeon"},
	{WisQuas, "Wis Quas", 4, TargetAutomatic, ScopeGlobal, ContextCombat, "Reveals hidden things"},

	// Circle 5 (Spells 24-29)
	{InBetXen, "In Bet Xen", 5, TargetAutomatic, ScopeRandomPlacement, ContextCombat, "Calls swarms of insects to fight beside you"},
	{AnExPor, "An Ex Por", 5, TargetDirection, ScopeSingleTile, ContextTown | ContextCombat, "Magically locks door"},
	{InExPor, "In Ex Por", 5, TargetDirection, ScopeSingleTile, ContextTown | ContextCombat, "Unlocks magically locked door"},
	{VasMani, "Vas Mani", 5, TargetPartyMember, ScopeSingleCharacter, ContextAll, "Heals major wounds"},
	{InZu, "In Zu", 5, TargetDirection, ScopeCone, ContextCombat, "Casts sleep over the foes before you"},
	{RelTym, "Rel Tym", 5, TargetAutomatic, ScopeGlobal, ContextAll, "Quickens the party, so it acts more often for a while"},

	// Circle 6 (Spells 30-35)
	{InVasPorYlem, "In Vas Por Ylem", 6, TargetAutomatic, ScopeGlobal, ContextCombat, "Earthquake attack"},
	{QuasAnWis, "Quas An Wis", 6, TargetAutomatic, ScopeGlobal, ContextCombat, "Confuses enemies"},
	{InAn, "In An", 6, TargetAutomatic, ScopeGlobal, ContextAll, "Negates magic"},
	{WisAnYlem, "Wis An Ylem", 6, TargetAutomatic, ScopeCaster, ContextOverworld | ContextTown, "Lets you see through walls for a short while"},
	{AnXenEx, "An Xen Ex", 6, TargetCombatantCrosshair, ScopeSingleCombatant, ContextCombat, "Charms creature"},
	{RelXenBet, "Rel Xen Bet", 6, TargetCombatantCrosshair, ScopeSingleCombatant, ContextCombat, "Turns one creature into a rat, if it cannot resist"},

	// Circle 7 (Spells 36-41)
	{SanctLor, "Sanct Lor", 7, TargetAutomatic, ScopeCaster, ContextCombat, "Grants invisibility"},
	{XenCorp, "Xen Corp", 7, TargetTileCrosshair, ScopeSingleCombatant, ContextCombat, "Strikes one foe dead, if it cannot resist"},
	{InQuasXen, "In Quas Xen", 7, TargetCombatantCrosshair, ScopeSingleCombatant, ContextCombat, "Makes a living double of one creature"},
	{InQuasWis, "In Quas Wis", 7, TargetAutomatic, ScopeCaster, ContextNonCombat, "Views the surrounding area"},
	{InNoxHur, "In Nox Hur", 7, TargetDirection, ScopeCone, ContextCombat, "Blows poison across the foes before you"},
	{InQuasCorp, "In Quas Corp", 7, TargetAutomatic, ScopeGlobal, ContextCombat, "Fills your foes with fear"},

	// Circle 8 (Spells 42-47)
	{InManiCorp, "In Mani Corp", 8, TargetPartyMember, ScopeSingleCharacter, ContextNonCombat, "Resurrects dead"},
	{KalXenCorp, "Kal Xen Corp", 8, TargetAutomatic, ScopeRandomPlacement, ContextCombat, "Summons daemon"},
	{InVasGravCorp, "In Vas Grav Corp", 8, TargetDirection, ScopeCone, ContextCombat, "Blows raw energy across the foes before you"},
	{InFlamHur, "In Flam Hur", 8, TargetDirection, ScopeCone, ContextCombat, "Blows flame across the foes before you"},
	{VasRelPor, "Vas Rel Por", 8, TargetMoonPhase, ScopeWorldTransition, ContextNonCombat, "Carries the party to the place of the moon you name"},
	{AnTym, "An Tym", 8, TargetAutomatic, ScopeGlobal, ContextAll, "Stops time"},
}

// GetSpellInfo returns metadata for the specified spell.
// Returns nil if spell is out of range.
func GetSpellInfo(spell Spell) *SpellInfo {
	if spell < 0 || spell >= TotalSpellCount {
		return nil
	}

	return &spellMetadata[spell]
}

// GetSpellName returns the display name for a spell.
// Returns empty string if spell is out of range.
func GetSpellName(spell Spell) string {
	info := GetSpellInfo(spell)
	if info == nil {
		return ""
	}

	return info.Name
}
