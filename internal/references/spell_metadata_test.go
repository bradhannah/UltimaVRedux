package references

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetSpellCircle verifies the circle calculation formula matches original U5.
// Formula: Circle = (spell_index / 6) + 1
// Spell circle is one plus the zero-based spell index divided by six, rounded down; ref:SPELL-0001.
func TestGetSpellCircle(t *testing.T) {
	testCases := []struct {
		spell          Spell
		expectedCircle int
		description    string
	}{
		// Circle 1 (spells 0-5)
		{InLor, 1, "In Lor is circle 1"},
		{Mani, 1, "Mani is circle 1"},
		{AnYlem, 1, "An Ylem (spell 5) is circle 1"},

		// Circle 2 (spells 6-11)
		{AnSanct, 2, "An Sanct (spell 6) is circle 2"},
		{RelHur, 2, "Rel Hur is circle 2"},
		{InXenMani, 2, "In Xen Mani (spell 11) is circle 2"},

		// Circle 3 (spells 12-17)
		{VasLor, 3, "Vas Lor (spell 12) is circle 3"},
		{InPor, 3, "In Por is circle 3"},

		// Circle 4 (spells 18-23)
		{AnGrav, 4, "An Grav (spell 18) is circle 4"},
		{WisQuas, 4, "Wis Quas (spell 23) is circle 4"},

		// Circle 5 (spells 24-29)
		{InBetXen, 5, "In Bet Xen (spell 24) is circle 5"},
		{VasMani, 5, "Vas Mani is circle 5"},
		{RelTym, 5, "Rel Tym (spell 29) is circle 5"},

		// Circle 6 (spells 30-35)
		{InVasPorYlem, 6, "In Vas Por Ylem (spell 30) is circle 6"},
		{RelXenBet, 6, "Rel Xen Bet (spell 35) is circle 6"},

		// Circle 7 (spells 36-41)
		{SanctLor, 7, "Sanct Lor (spell 36) is circle 7"},
		{InQuasCorp, 7, "In Quas Corp (spell 41) is circle 7"},

		// Circle 8 (spells 42-47)
		{InManiCorp, 8, "In Mani Corp (spell 42) is circle 8"},
		{AnTym, 8, "An Tym (spell 47) is circle 8"},

		// Edge cases
		{Spell(-1), 0, "Invalid spell -1 returns 0"},
		{Spell(48), 0, "Invalid spell 48 returns 0"},
		{Spell(100), 0, "Invalid spell 100 returns 0"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			circle := GetSpellCircle(tc.spell)
			assert.Equal(t, tc.expectedCircle, circle, tc.description)
		})
	}
}

// TestGetSpellInfo verifies spell metadata lookup works correctly.
func TestGetSpellInfo(t *testing.T) {
	t.Run("Valid spell returns correct info", func(t *testing.T) {
		info := GetSpellInfo(InLor)
		require.NotNil(t, info)
		assert.Equal(t, InLor, info.Spell)
		assert.Equal(t, "In Lor", info.Name)
		assert.Equal(t, 1, info.Circle)
		assert.Equal(t, TargetAutomatic, info.Targeting)
		assert.Equal(t, ContextNonCombat, info.Context)
	})

	t.Run("Circle 8 spell has correct metadata", func(t *testing.T) {
		info := GetSpellInfo(AnTym)
		require.NotNil(t, info)
		assert.Equal(t, AnTym, info.Spell)
		assert.Equal(t, "An Tym", info.Name)
		assert.Equal(t, 8, info.Circle)
		assert.Equal(t, TargetAutomatic, info.Targeting)
		assert.Equal(t, ContextAll, info.Context)
	})

	t.Run("Invalid spell returns nil", func(t *testing.T) {
		info := GetSpellInfo(Spell(-1))
		assert.Nil(t, info)

		info = GetSpellInfo(Spell(48))
		assert.Nil(t, info)
	})

	t.Run("All 48 spells have valid info", func(t *testing.T) {
		for i := 0; i < TotalSpellCount; i++ {
			info := GetSpellInfo(Spell(i))
			require.NotNil(t, info, "Spell %d should have metadata", i)
			assert.Equal(t, Spell(i), info.Spell, "Spell %d should occupy its indexed catalog row", i)
			assert.NotEmpty(t, info.Name, "Spell %d should have a name", i)
			assert.NotEmpty(t, info.Description, "Spell %d should have a description", i)
			assert.GreaterOrEqual(t, info.Circle, 1, "Spell %d circle should be >= 1", i)
			assert.LessOrEqual(t, info.Circle, 8, "Spell %d circle should be <= 8", i)
			assert.GreaterOrEqual(t, info.Targeting, TargetAutomatic, "Spell %d targeting should be recognized", i)
			assert.LessOrEqual(t, info.Targeting, TargetMoonPhase, "Spell %d targeting should be recognized", i)
			assert.GreaterOrEqual(t, info.Scope, ScopeCaster, "Spell %d scope should be recognized", i)
			assert.LessOrEqual(t, info.Scope, ScopeWorldTransition, "Spell %d scope should be recognized", i)
			assert.NotZero(t, info.Context, "Spell %d should have an original legal context", i)
		}
	})
}

func TestVasRelPorMetadataRequiresMoonPhaseOnSurface(t *testing.T) {
	info := GetSpellInfo(VasRelPor)
	require.NotNil(t, info)
	assert.Equal(t, TargetMoonPhase, info.Targeting)
	assert.Equal(t, ScopeWorldTransition, info.Scope)
	assert.Equal(t, ContextNonCombat, info.Context)
}

func TestAllSpellTargetingProfilesUseCatalogScopeOutsideCombat(t *testing.T) {
	for spell := Spell(0); spell < TotalSpellCount; spell++ {
		info := GetSpellInfo(spell)
		require.NotNil(t, info)

		for _, context := range []SpellContext{ContextOverworld, ContextTown, ContextDungeon} {
			expected := SpellTargetingProfile{Selection: info.Targeting, Scope: info.Scope}
			if context == ContextDungeon {
				switch spell {
				case AnSanct, InFlamGrav, InNoxGrav, InZuGrav, InSanctGrav, AnGrav:
					expected.Selection = TargetAutomatic
				}
			}
			assert.Equal(t,
				expected,
				GetSpellTargetingProfile(spell, context),
				"%s/%s should use its context profile", info.Name, context.String(),
			)
		}
	}
}

func TestAnXenCorpRequiresNoTarget(t *testing.T) {
	info := GetSpellInfo(AnXenCorp)
	require.NotNil(t, info)
	assert.Equal(t, TargetAutomatic, info.Targeting)
	assert.Equal(t, ContextCombat, info.Context)
}

// TestIsSpellValidInContext verifies context validation works correctly.
// Original permissions distinguish outdoor, town, dungeon, and combat contexts; ref:SPELL-0002.
func TestIsSpellValidInContext(t *testing.T) {
	testCases := []struct {
		spell    Spell
		context  SpellContext
		expected bool
		desc     string
	}{
		// In Lor - Non-Combat only (overworld, town, dungeon)
		{InLor, ContextOverworld, true, "In Lor valid in overworld"},
		{InLor, ContextTown, true, "In Lor valid in town"},
		{InLor, ContextDungeon, true, "In Lor valid in dungeon"},
		{InLor, ContextCombat, false, "In Lor NOT valid in combat"},

		// Mani - All contexts
		{Mani, ContextOverworld, true, "Mani valid in overworld"},
		{Mani, ContextTown, true, "Mani valid in town"},
		{Mani, ContextDungeon, true, "Mani valid in dungeon"},
		{Mani, ContextCombat, true, "Mani valid in combat"},

		// Grav Por - Combat only
		{GravPor, ContextOverworld, false, "Grav Por NOT valid in overworld"},
		{GravPor, ContextTown, false, "Grav Por NOT valid in town"},
		{GravPor, ContextDungeon, false, "Grav Por NOT valid in dungeon"},
		{GravPor, ContextCombat, true, "Grav Por valid in combat"},

		// Rel Hur - Overworld only (wind change)
		{RelHur, ContextOverworld, true, "Rel Hur valid in overworld"},
		{RelHur, ContextTown, false, "Rel Hur NOT valid in town"},
		{RelHur, ContextDungeon, false, "Rel Hur NOT valid in dungeon"},
		{RelHur, ContextCombat, false, "Rel Hur NOT valid in combat"},

		// In Por - Overworld + Combat
		{InPor, ContextOverworld, true, "In Por valid in overworld"},
		{InPor, ContextTown, false, "In Por NOT valid in town"},
		{InPor, ContextDungeon, false, "In Por NOT valid in dungeon"},
		{InPor, ContextCombat, true, "In Por valid in combat"},

		// An Grav - Dungeon + Combat
		{AnGrav, ContextOverworld, false, "An Grav NOT valid in overworld"},
		{AnGrav, ContextTown, false, "An Grav NOT valid in town"},
		{AnGrav, ContextDungeon, true, "An Grav valid in dungeon"},
		{AnGrav, ContextCombat, true, "An Grav valid in combat"},

		// An Ex Por - Town + Combat (magic unlock)
		{AnExPor, ContextOverworld, false, "An Ex Por NOT valid in overworld"},
		{AnExPor, ContextTown, true, "An Ex Por valid in town"},
		{AnExPor, ContextDungeon, false, "An Ex Por NOT valid in dungeon"},
		{AnExPor, ContextCombat, true, "An Ex Por valid in combat"},

		// In Mani Corp - Non-Combat only (resurrection)
		{InManiCorp, ContextOverworld, true, "In Mani Corp valid in overworld"},
		{InManiCorp, ContextTown, true, "In Mani Corp valid in town"},
		{InManiCorp, ContextDungeon, true, "In Mani Corp valid in dungeon"},
		{InManiCorp, ContextCombat, false, "In Mani Corp NOT valid in combat"},

		// Invalid spell
		{Spell(-1), ContextAll, false, "Invalid spell returns false"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result := IsSpellValidInContext(tc.spell, tc.context)
			assert.Equal(t, tc.expected, result, tc.desc)
		})
	}
}

// TestGetSpellsByContext verifies context-based spell filtering.
func TestGetSpellsByContext(t *testing.T) {
	t.Run("Combat spells include combat-only spells", func(t *testing.T) {
		spells := GetSpellsByContext(ContextCombat)
		assert.Contains(t, spells, GravPor, "Grav Por is combat spell")
		assert.Contains(t, spells, VasFlam, "Vas Flam is combat spell")
		assert.Contains(t, spells, Mani, "Mani is usable in combat")
	})

	t.Run("Overworld spells exclude combat-only spells", func(t *testing.T) {
		spells := GetSpellsByContext(ContextOverworld)
		assert.Contains(t, spells, InLor, "In Lor is overworld spell")
		assert.Contains(t, spells, RelHur, "Rel Hur is overworld spell")
		assert.NotContains(t, spells, GravPor, "Grav Por is NOT overworld spell")
		assert.NotContains(t, spells, VasFlam, "Vas Flam is NOT overworld spell")
	})

	t.Run("Dungeon spells include dungeon-specific spells", func(t *testing.T) {
		spells := GetSpellsByContext(ContextDungeon)
		assert.Contains(t, spells, AnGrav, "An Grav is dungeon spell")
		assert.Contains(t, spells, UusPor, "Uus Por is dungeon spell")
		assert.Contains(t, spells, DesPor, "Des Por is dungeon spell")
	})
}

// TestSpellContextString verifies context string representation.
func TestSpellContextString(t *testing.T) {
	assert.Equal(t, "All", ContextAll.String())
	assert.Equal(t, "Non-Combat", ContextNonCombat.String())
	assert.Equal(t, "Overworld", ContextOverworld.String())
	assert.Equal(t, "Combat", ContextCombat.String())
	assert.Equal(t, "Overworld, Combat", (ContextOverworld | ContextCombat).String())
	assert.Equal(t, "None", SpellContext(0).String())
}

// TestTargetingTypeString verifies targeting string representation.
func TestTargetingTypeString(t *testing.T) {
	assert.Equal(t, "Automatic", TargetAutomatic.String())
	assert.Equal(t, "Character", TargetPartyMember.String())
	assert.Equal(t, "Direction", TargetDirection.String())
	assert.Equal(t, "Adjacent tile", TargetAdjacentTile.String())
	assert.Equal(t, "Combatant crosshair", TargetCombatantCrosshair.String())
	assert.Equal(t, "Tile crosshair", TargetTileCrosshair.String())
	assert.Equal(t, "Unknown", TargetingType(99).String())
}

// A player reads plain words in the spell details; logs and the Workbench keep
// the short names.
func TestSpellPlayerLabelsUsePlainWords(t *testing.T) {
	assert.Equal(t, "No aim needed", TargetAutomatic.PlayerLabel())
	assert.Equal(t, "A companion in the fight", TargetCombatPartyMember.PlayerLabel())
	assert.Equal(t, "A square beside you", TargetAdjacentTile.PlayerLabel())
	assert.Equal(t, "A creature you pick", TargetCombatantCrosshair.PlayerLabel())
	assert.Equal(t, "A square you pick", TargetTileCrosshair.PlayerLabel())
	assert.Equal(t, "Direction", TargetDirection.PlayerLabel())
	assert.Equal(t, "Anywhere", ContextAll.PlayerLabel())
	assert.Equal(t, "Outside battle", ContextNonCombat.PlayerLabel())
	assert.Equal(t, "Overworld, Combat", (ContextOverworld | ContextCombat).PlayerLabel())
}

func TestSpellTargetingProfilesResolveCombatOverrides(t *testing.T) {
	assert.Equal(t, SpellTargetingProfile{Selection: TargetTileCrosshair, Scope: ScopeSingleTile}, GetSpellTargetingProfile(InFlamGrav, ContextCombat))
	assert.Equal(t, SpellTargetingProfile{Selection: TargetAdjacentTile, Scope: ScopeSingleTile}, GetSpellTargetingProfile(AnSanct, ContextCombat))
	assert.Equal(t, SpellTargetingProfile{Selection: TargetDirection, Scope: ScopeCone}, GetSpellTargetingProfile(InZu, ContextCombat))
	assert.Equal(t, SpellTargetingProfile{Selection: TargetAutomatic, Scope: ScopeCaster}, GetSpellTargetingProfile(InPor, ContextCombat))
	assert.Equal(t, SpellTargetingProfile{Selection: TargetTileCrosshair, Scope: ScopeSingleCombatant}, GetSpellTargetingProfile(XenCorp, ContextCombat))
}

func TestSpellTargetingProfilesResolveAutomaticDungeonPackedCells(t *testing.T) {
	for _, spell := range []Spell{AnSanct, InFlamGrav, InNoxGrav, InZuGrav, InSanctGrav, AnGrav} {
		assert.Equal(t,
			SpellTargetingProfile{Selection: TargetAutomatic, Scope: ScopeSingleTile},
			GetSpellTargetingProfile(spell, ContextDungeon),
			GetSpellName(spell),
		)
	}
}

// TestGetSpellName verifies spell name lookup.
func TestGetSpellName(t *testing.T) {
	assert.Equal(t, "In Lor", GetSpellName(InLor))
	assert.Equal(t, "An Tym", GetSpellName(AnTym))
	assert.Equal(t, "Vas Mani", GetSpellName(VasMani))
	assert.Equal(t, "", GetSpellName(Spell(-1)))
	assert.Equal(t, "", GetSpellName(Spell(48)))
}

// TestSpellMetadataConsistency verifies all metadata is internally consistent.
func TestSpellMetadataConsistency(t *testing.T) {
	t.Run("Circle in SpellInfo matches calculated circle", func(t *testing.T) {
		for i := 0; i < 48; i++ {
			spell := Spell(i)
			info := GetSpellInfo(spell)
			require.NotNil(t, info)
			calculatedCircle := GetSpellCircle(spell)
			assert.Equal(t, calculatedCircle, info.Circle,
				"Spell %d (%s): metadata circle %d should match calculated circle %d",
				i, info.Name, info.Circle, calculatedCircle)
		}
	})

	t.Run("Spell field in SpellInfo matches index", func(t *testing.T) {
		for i := 0; i < 48; i++ {
			info := GetSpellInfo(Spell(i))
			require.NotNil(t, info)
			assert.Equal(t, Spell(i), info.Spell,
				"Spell %d: SpellInfo.Spell should match index", i)
		}
	})
}
