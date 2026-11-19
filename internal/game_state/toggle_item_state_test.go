package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestToggleItemType_FriendlyName(t *testing.T) {
	tests := []struct {
		itemType ToggleItemType
		expected string
	}{
		{ToggleItemAmulet, "Amulet"},
		{ToggleItemCrown, "Crown"},
		{ToggleItemBadge, "Badge"},
		{ToggleItemType(0), "Item"}, // Unknown type
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.itemType.FriendlyName())
		})
	}
}

func TestToggleItem_Activation(t *testing.T) {
	gs := createMinimalGameStateForToggleTests(t)

	// Initially inactive
	assert.False(t, gs.IsAmuletActive())
	assert.False(t, gs.IsCrownActive())
	assert.False(t, gs.IsBadgeActive())

	// Activate Amulet
	result := gs.ToggleAmulet()
	assert.True(t, result, "ToggleAmulet should return true")
	assert.True(t, gs.IsAmuletActive())
	assert.Equal(t, ToggleItemDuration, gs.AmuletState.TurnsRemaining)

	// Other items still inactive
	assert.False(t, gs.IsCrownActive())
	assert.False(t, gs.IsBadgeActive())
}

func TestToggleItem_Deactivation(t *testing.T) {
	gs := createMinimalGameStateForToggleTests(t)

	// Activate then deactivate
	gs.ToggleAmulet()
	assert.True(t, gs.IsAmuletActive())

	result := gs.ToggleAmulet()
	assert.True(t, result, "ToggleAmulet (deactivate) should return true")
	assert.False(t, gs.IsAmuletActive())
	assert.Equal(t, 0, gs.AmuletState.TurnsRemaining)
}

func TestToggleItem_MultipleConcurrent(t *testing.T) {
	gs := createMinimalGameStateForToggleTests(t)

	// Activate all three
	gs.ToggleAmulet()
	gs.ToggleCrown()
	gs.ToggleBadge()

	// All should be active
	assert.True(t, gs.IsAmuletActive())
	assert.True(t, gs.IsCrownActive())
	assert.True(t, gs.IsBadgeActive())

	// Deactivate one - others remain active
	gs.ToggleCrown()
	assert.True(t, gs.IsAmuletActive())
	assert.False(t, gs.IsCrownActive())
	assert.True(t, gs.IsBadgeActive())
}

func TestToggleBadgePreservesIndependentEffects(t *testing.T) {
	gs := createMinimalGameStateForToggleTests(t)
	gs.AmuletState = ToggleItemState{IsActive: true, TurnsRemaining: 77}
	gs.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectProtection, TurnsRemaining: 9}

	require.True(t, gs.ToggleBadge())
	assert.Equal(t, ToggleItemState{IsActive: true, TurnsRemaining: 77}, gs.AmuletState)
	assert.Equal(t, ToggleItemState{}, gs.CrownState)
	assert.Equal(t, DurationEffect{EffectType: DurationEffectProtection, TurnsRemaining: 9}, gs.ActiveDurationEffect)
	assert.Equal(t, ToggleItemState{IsActive: true, TurnsRemaining: ToggleItemDuration}, gs.BadgeState)

	require.True(t, gs.ToggleBadge())
	assert.Equal(t, ToggleItemState{IsActive: true, TurnsRemaining: 77}, gs.AmuletState)
	assert.Equal(t, ToggleItemState{}, gs.CrownState)
	assert.Equal(t, DurationEffect{EffectType: DurationEffectProtection, TurnsRemaining: 9}, gs.ActiveDurationEffect)
	assert.Equal(t, ToggleItemState{}, gs.BadgeState)
}

func TestCrownCompetesWithEveryDurationSpell(t *testing.T) {
	for _, test := range []struct {
		name     string
		effect   DurationEffectType
		activate func(*GameState, int) bool
	}{
		{"Protection", DurationEffectProtection, (*GameState).ActivateProtectionEffect},
		{"Quickness", DurationEffectQuickness, (*GameState).ActivateQuicknessEffect},
		{"Confusion", DurationEffectConfusion, (*GameState).ActivateConfusionEffect},
		{"NegateMagic", DurationEffectNegateMagic, (*GameState).ActivateNegateMagicEffect},
		{"TimeStop", DurationEffectTimeStop, (*GameState).ActivateTimeStopEffect},
	} {
		t.Run(test.name, func(t *testing.T) {
			gs := newPersistentEffectsTestState(t)
			gs.MagicLightMinutesRemaining = 100
			gs.MapState.Lighting.SetMagicLight(true)
			gs.ToggleBadge()
			gs.ToggleAmulet()
			badge, amulet := gs.BadgeState, gs.AmuletState
			gs.ToggleCrown()
			assert.Equal(t, PersistentEffectCrownSuppression, gs.GetPersistentEffectStatuses()[1].ID)
			assert.True(t, gs.GetWornArtifactStatuses()[0].Worn)

			require.True(t, test.activate(gs, 9))
			assert.Equal(t, ToggleItemState{}, gs.CrownState, "spell removes Crown and its timer")
			assert.Equal(t, DurationEffect{EffectType: test.effect, TurnsRemaining: 9}, gs.ActiveDurationEffect)
			assert.Equal(t, badge, gs.BadgeState)
			assert.Equal(t, amulet, gs.AmuletState)
			assert.Equal(t, persistentDurationEffectID(test.effect), gs.GetPersistentEffectStatuses()[1].ID)
			assert.False(t, gs.GetWornArtifactStatuses()[0].Worn)

			gs.processDurationEffects()
			require.True(t, test.activate(gs, 3))
			assert.Equal(t, DurationEffect{EffectType: test.effect, TurnsRemaining: 3}, gs.ActiveDurationEffect,
				"same spell refreshes rather than toggling off")
			for range 3 {
				gs.processDurationEffects()
			}
			assert.Equal(t, DurationEffect{}, gs.ActiveDurationEffect)
			assert.Equal(t, ToggleItemState{}, gs.CrownState, "expired spell never resumes Crown")
			assert.Equal(t, []PersistentEffectStatus{{ID: PersistentEffectMagicLight, Atmosphere: EffectAtmosphereSteady}},
				gs.GetPersistentEffectStatuses())

			test.activate(gs, 9)
			gs.ToggleCrown()
			assert.Equal(t, DurationEffect{}, gs.ActiveDurationEffect, "Crown replaces the spell")
			assert.Equal(t, ToggleItemState{IsActive: true, TurnsRemaining: ToggleItemDuration}, gs.CrownState)
			gs.ToggleCrown()
			assert.Equal(t, DurationEffect{}, gs.ActiveDurationEffect, "removing Crown never resumes the spell")
			assert.Equal(t, ToggleItemState{}, gs.CrownState)
			assert.Equal(t, badge, gs.BadgeState)
			assert.Equal(t, amulet, gs.AmuletState)
			assert.Equal(t, 100, gs.MagicLightMinutesRemaining)
			assert.True(t, gs.MapState.Lighting.HasMagicLight())
		})
	}
}

func TestCrownAndDurationSpellNameWhatTheyDisplace(t *testing.T) {
	gs := createMinimalGameStateForToggleTests(t)
	var messages []string
	gs.SystemCallbacks.UI.ShowActionFeedback = func(message string, _ bool) {
		messages = append(messages, message)
	}
	take := func() []string {
		got := messages
		messages = nil

		return got
	}

	gs.ToggleCrown()
	gs.ActivateQuicknessEffect(30)
	assert.Equal(t, []string{"Crown donned!", CrownDisplacedMessage, "Quickness!"}, take())

	gs.ActivateQuicknessEffect(30)
	assert.Equal(t, []string{"Quickness!"}, take(), "a refresh with no Crown worn reports no loss")

	gs.ToggleCrown()
	assert.Equal(t, []string{"Quickness ended!", "Crown donned!"}, take())

	gs.ActivateTimeStopEffect(10)
	assert.Equal(t, []string{CrownDisplacedMessage}, take(), "Time Stop names the Crown before its own popup")

	gs.ClearDurationEffect()
	gs.ToggleCrown()
	gs.ToggleCrown()
	assert.Equal(t, []string{"Crown donned!", "Removed!"}, take(), "taking the Crown off by hand ends no spell")
}

func TestIndependentArtifactTogglesPreserveSpell(t *testing.T) {
	gs := createMinimalGameStateForToggleTests(t)
	gs.ActivateProtectionEffect(9)
	want := gs.ActiveDurationEffect
	for range 2 {
		gs.ToggleAmulet()
		gs.ToggleBadge()
		assert.Equal(t, want, gs.ActiveDurationEffect)
		assert.False(t, gs.IsCrownActive())
	}
	gs.processToggleItems()
	assert.Equal(t, want, gs.ActiveDurationEffect, "inactive Crown processing must not clear a spell")
}

func TestTimeStopDurationBoundariesNotifyForCrownReplacement(t *testing.T) {
	gs := createMinimalGameStateForToggleTests(t)
	gs.ElapsedMs = 12345
	var observed []DurationEffectType
	gs.SystemCallbacks.Screen.UpdateDisplay = func() {
		observed = append(observed, gs.ActiveDurationEffect.EffectType)
	}
	require.True(t, gs.ActivateTimeStopEffect(20))
	assert.Equal(t, []DurationEffectType{DurationEffectTimeStop}, observed)
	require.True(t, gs.ActivateTimeStopEffect(10))
	assert.Len(t, observed, 1, "refresh preserves the current animation episode")
	gs.SystemCallbacks.Flow.Items.TriggerToggleItemFlash = func(itemType ToggleItemType) {
		assert.Equal(t, ToggleItemCrown, itemType)
		assert.Equal(t, []DurationEffectType{DurationEffectTimeStop, DurationEffectNone}, observed,
			"Crown must publish the normal-rate boundary before its presentation feedback")
	}
	require.True(t, gs.ToggleCrown())
	assert.False(t, gs.HasAnyDurationEffect())
	assert.True(t, gs.IsCrownActive())
	require.True(t, gs.ToggleCrown())
	assert.Len(t, observed, 2, "removing Crown must not resume the replaced spell")
	require.True(t, gs.ActivateTimeStopEffect(1))
	gs.processDurationEffects()
	assert.Equal(t, []DurationEffectType{
		DurationEffectTimeStop, DurationEffectNone, DurationEffectTimeStop, DurationEffectNone,
	}, observed)
	assert.EqualValues(t, 12345, gs.ElapsedMs)
}

func TestDebugDurationActivationReplacesCrownButLightDoesNot(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()
	gs.ToggleCrown()
	_, err := gs.ActivateDebugDurationEffect(DurationEffectLight, "", 0, 0)
	require.NoError(t, err)
	assert.True(t, gs.IsCrownActive())
	_, err = gs.ActivateDebugDurationEffect(DurationEffectTimeStop, "", 0, gs.MagicLightMinutesRemaining)
	require.NoError(t, err)
	assert.Equal(t, ToggleItemState{}, gs.CrownState)
	assert.True(t, gs.HasActiveDurationEffect(DurationEffectTimeStop))
}

func TestToggleItem_PermanentNeverDecrements(t *testing.T) {
	gs := createMinimalGameStateForToggleTests(t)

	gs.ToggleAmulet()
	// Verify starts at permanent duration (255)
	assert.Equal(t, ToggleItemDuration, gs.AmuletState.TurnsRemaining)

	// Process multiple turns - should stay at 255 (permanent)
	for i := 0; i < 10; i++ {
		gs.processToggleItems()
	}

	assert.Equal(t, ToggleItemDuration, gs.AmuletState.TurnsRemaining)
	assert.True(t, gs.IsAmuletActive())
}

func TestToggleItem_NonPermanentDecrements(t *testing.T) {
	gs := createMinimalGameStateForToggleTests(t)

	gs.ToggleAmulet()
	// Set to a non-permanent value to test decrement
	gs.AmuletState.TurnsRemaining = 100

	// Process one turn
	gs.processToggleItems()

	assert.Equal(t, 99, gs.AmuletState.TurnsRemaining)
	assert.True(t, gs.IsAmuletActive())
}

func TestToggleItem_Expiration(t *testing.T) {
	gs := createMinimalGameStateForToggleTests(t)

	gs.ToggleAmulet()
	// Set to 1 turn remaining
	gs.AmuletState.TurnsRemaining = 1

	// Process - should expire
	gs.processToggleItems()

	assert.False(t, gs.IsAmuletActive())
	assert.Equal(t, 0, gs.AmuletState.TurnsRemaining)
}

func TestWornArtifactProjectionPreservesEveryOwnershipAndToggleCombination(t *testing.T) {
	items := []references.QuestItem{references.Crown, references.Amulet, references.BlackBadge}
	identities := []ToggleItemType{ToggleItemCrown, ToggleItemAmulet, ToggleItemBadge}
	for ownedMask := range 8 {
		for wornMask := range 8 {
			gs := createMinimalGameStateForToggleTests(t)
			states := []*ToggleItemState{&gs.CrownState, &gs.AmuletState, &gs.BadgeState}
			for index, item := range items {
				if ownedMask&(1<<index) != 0 {
					gs.PartyState.Inventory.QuestItems.Set(item, 1)
				}
				*states[index] = ToggleItemState{IsActive: wornMask&(1<<index) != 0, TurnsRemaining: ToggleItemDuration}
			}
			projection := gs.GetWornArtifactStatuses()
			for index, id := range identities {
				assert.Equal(t, WornArtifactStatus{ID: id, Owned: ownedMask&(1<<index) != 0, Worn: wornMask&(1<<index) != 0}, projection[index])
				assert.Equal(t, wornMask&(1<<index) != 0, states[index].IsActive, "renderer never repairs ownership")
				assert.Equal(t, ToggleItemDuration, states[index].TurnsRemaining)
			}
			projection[0] = WornArtifactStatus{}
			assert.Equal(t, ToggleItemCrown, gs.GetWornArtifactStatuses()[0].ID, "detached fixed array")
		}
	}
}

func TestWornArtifactProjectionKeepsSlotsAcrossRemovalAndInventoryLoss(t *testing.T) {
	gs := createMinimalGameStateForToggleTests(t)
	gs.ToggleBadge()
	gs.ToggleAmulet()
	gs.ToggleCrown()
	gs.PartyState.Inventory.QuestItems.Set(references.Crown, 1)
	gs.PartyState.Inventory.QuestItems.Set(references.Amulet, 1)
	gs.PartyState.Inventory.QuestItems.Set(references.BlackBadge, 1)
	want := gs.GetWornArtifactStatuses()
	gs.ToggleCrown()
	gs.PartyState.Inventory.QuestItems.Set(references.Amulet, 0)
	got := gs.GetWornArtifactStatuses()
	assert.Equal(t, WornArtifactStatus{ID: ToggleItemCrown, Owned: true}, got[0])
	assert.Equal(t, WornArtifactStatus{ID: ToggleItemAmulet, Worn: true}, got[1])
	assert.Equal(t, want[2], got[2])
}

// createMinimalGameStateForToggleTests creates a minimal GameState for testing toggle items.
func createMinimalGameStateForToggleTests(t *testing.T) *GameState {
	t.Helper()

	gs := &GameState{}
	gs.SystemCallbacks = createMockSystemCallbacksForToggleTests()

	return gs
}

// createMockSystemCallbacksForToggleTests creates mock callbacks for toggle item tests.
func createMockSystemCallbacksForToggleTests() *SystemCallbacks {
	return &SystemCallbacks{
		Screen: ScreenCallbacks{
			UpdateDisplay: func() {}, // Duration/Crown replacements publish presentation boundaries.
		},
		UI: UICallbacks{
			ShowActionFeedback: func(message string, important bool) {
				// No-op for tests
			},
			ShowSpellExpiredPopup: func(effectName string) {
				// No-op for tests
			},
		},
		Flow: FlowCallbacks{
			Items: FlowItemCallbacks{
				TriggerToggleItemFlash: func(itemType ToggleItemType) {
					// No-op for tests - flash effect is UI-only
				},
			},
		},
	}
}

// A party on a darkness square is lost unless it wears the Amulet. See ref:QUEST-0004.
func TestPartyIsLostOnDarknessUnlessWearingTheAmulet(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithFloor(references.FloorNumber(references.UNDERWORLD)).WithPlayerAt(122, 126).WithSystemCallbacks().Build()
	require.False(t, gs.IsLostInDarkness(), "the lava beside the darkness is ordinary ground")

	gs.MapState.PlayerLocation.Position = references.Position{X: 123, Y: 126}
	require.True(t, gs.GetCurrentAvatarGroundTile().Is(indexes.BlackSquare), "fixture: a darkness square")
	require.True(t, gs.IsLostInDarkness())

	require.True(t, gs.ToggleAmulet())
	require.False(t, gs.IsLostInDarkness(), "the Amulet shows the way")
	require.True(t, gs.ToggleAmulet())
	require.True(t, gs.IsLostInDarkness(), "taking it off loses the way again")

	// The rule belongs to the large map: the same tile in a towne is only a tile.
	towne, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	position := towne.MapState.PlayerLocation.Position
	towne.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.BlackSquare)
	require.False(t, towne.IsLostInDarkness())
}
