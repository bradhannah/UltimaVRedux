package legacy_save

import (
	"path/filepath"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
)

func TestBCarpetFixtureReadsAndConvertsToNativeSnapshot(t *testing.T) {
	bundle, err := ReadBundleContext(t.Context(), filepath.Join("testdata", "b_carpet", "SAVED.GAM"))
	if err != nil {
		t.Fatalf("ReadBundleContext() b_carpet error = %T %v", err, err)
	}

	result, err := Convert(bundle)
	if err != nil {
		t.Fatalf(
			"Convert() b_carpet error = %T %v; spell=%#x spellDuration=%d activeSpell=%#x magic=%#x",
			err,
			err,
			bundle.Game.Spell,
			bundle.Game.SpellDuration,
			bundle.Game.ActiveSpell,
			bundle.Game.Magic,
		)
	}
	if issues := save_snapshot.Validate(result.Snapshot); len(issues) != 0 {
		t.Fatalf("converted b_carpet snapshot issues = %#v", issues)
	}
	if result.Snapshot.Party.Roster[0].Name != "Avatar" {
		t.Fatalf("Avatar name = %q", result.Snapshot.Party.Roster[0].Name)
	}
	if bundle.Game.ActivePlayer != 0xff || bundle.Game.LastGave != 0xff ||
		result.Snapshot.Continuation.ActivePlayerIndex != -1 ||
		result.Snapshot.Continuation.BeggarKarmaCounter != 0xff {
		t.Fatalf("converted continuation = source active %#x last_gave %#x, native %#v", bundle.Game.ActivePlayer, bundle.Game.LastGave, result.Snapshot.Continuation)
	}
	if result.Snapshot.World.Position != (save_snapshot.PositionSnapshot{Location: 0, X: 146, Y: 241, Floor: 0}) {
		t.Fatalf("converted position = %#v", result.Snapshot.World.Position)
	}
	if result.Snapshot.Progression.Dungeons[7] {
		t.Fatal("b_carpet fixture must preserve Doom as closed")
	}
	if result.Snapshot.Effects.DurationType != 0 || result.Snapshot.Effects.Amulet != (save_snapshot.ToggleSnapshot{Active: true, Turns: 255}) {
		t.Fatalf("converted effects = %#v", result.Snapshot.Effects)
	}
	if result.Snapshot.Inventory.QuestItems[references.Amulet] != 1 {
		t.Fatalf("converted Amulet quantity = %d", result.Snapshot.Inventory.QuestItems[references.Amulet])
	}
	if result.Snapshot.PartyVehicle == nil || result.Snapshot.PartyVehicle.Type != int(references.CarpetVehicle) {
		t.Fatalf("converted party vehicle = %#v", result.Snapshot.PartyVehicle)
	}
	if !hasConversionWarning(result.Warnings, "effects", "transient_action_flags_omitted") {
		t.Fatalf("transient-action conversion warning missing: %#v", result.Warnings)
	}
}
