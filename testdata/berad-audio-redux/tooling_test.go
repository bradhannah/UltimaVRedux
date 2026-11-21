package reduxadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gameaudio "github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/audio/tooling"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/authoring"
	editor "github.com/bradhannah/berad-sdk/tools/berad-audio/editor"
)

func TestReduxToolProjectPreservesPolicyAndSharedAuthoringRoundTrip(t *testing.T) {
	library, files := project(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sounds"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sounds/synthetic.wav"), files["sounds/synthetic.wav"].Data, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := json.MarshalIndent(library.Document(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "library.json")
	if err := os.WriteFile(manifest, before, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := tooling.OpenAuthoringEditor(manifest)
	if err != nil {
		t.Fatal(err)
	}
	options := tooling.AudioEditorOptions()
	for _, slot := range []struct {
		cue, recording string
		terms          []string
	}{
		{"sfx/SoundDispel", "field_dispel_01", []string{"sceptre", "an grav", "field removal", "fizzle", "dungeon"}},
		{"sfx/SoundElectricalDamage", "electrical_injury_01", []string{"electrical field", "shock", "injury", "dungeon"}},
	} {
		for _, term := range slot.terms {
			if !strings.Contains(strings.ToLower(doc.SearchText(slot.cue, true)), term) ||
				!strings.Contains(strings.ToLower(doc.SearchText(slot.recording, false)), term) {
				t.Fatalf("game trigger search lost %q for %s", term, slot.cue)
			}
		}
		if doc.FileAvailability(slot.recording).State != authoring.RecordingNoFile {
			t.Fatal("silent slot changed")
		}
	}
	if err := editor.ValidateOptions(doc, options); err != nil {
		t.Fatal(err)
	}
	volumes := config.DefaultAudioConfiguration()
	want := editor.SliderLevels{Music: volumes.MusicVolume, SFX: volumes.SoundEffectVolume, Ambience: volumes.AmbientVolume}
	if options.InitialSliders == nil || *options.InitialSliders != want {
		t.Fatal("the editor preview must start on a new player's volumes")
	}
	for level := 0; level <= 10; level++ {
		if options.SliderGain(level) != gameaudio.ConvertSliderToVolume(level) {
			t.Fatal("monitoring mapping changed")
		}
	}
	if options.TargetPolicy("naval", "sailing").Category != "music" || !options.TargetPolicy("naval", "frigate").Background {
		t.Fatal("naval policy changed")
	}
	policy, err := doc.CalibrationOptionsFor("ultima_theme")
	if err != nil {
		t.Fatal(err)
	}
	if policy.TargetDB != -23 || policy.Method != authoring.GatedLoudness {
		t.Fatal("score calibration policy changed")
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(manifest)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("no-op tool save reformatted manifest", err)
	}
	var target string
	for _, id := range doc.RecordingIDs() {
		if doc.Recording(id).File != nil {
			target = id
			break
		}
	}
	path, err := doc.RecordingPath(target)
	if err != nil {
		t.Fatal(err)
	}
	clip, err := doc.LoadRecording(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := doc.BeginRecordingRegion(target, clip)
	if err != nil {
		t.Fatal(err)
	}
	window := &authoring.PlaybackWindow{StartMs: 10, EndMs: 50}
	if err := doc.ApplyRecordingRegion(context.Background(), draft, window); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	reopened, err := tooling.OpenAuthoringEditor(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Dirty() || *reopened.Recording(target).PlaybackWindow != *window {
		t.Fatal("saved edit failed to reopen")
	}
}
