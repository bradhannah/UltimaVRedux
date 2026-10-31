//go:build settingsproof && ebitenginevmguest

package main

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
)

// This opt-in test binary runs the production title/game/dialog stack. Only the
// VM host supplies input; these observations never change the application state.
func TestMain(_ *testing.M) {
	logging.SetGlobalEnabled(true)
	logging.SetCategoryEnabled(logging.CategorySystem, true)
	logging.SetStdoutEnabled(true)
	if err := runSettingsVMGuest(); err != nil {
		logging.Errorf(logging.CategorySystem, "VM settings proof: %v", err)
		os.Exit(1)
	}
}

func runSettingsVMGuest() error {
	// Ebitengine consumes and unsets EBITENGINE_VM_ENDPOINT during init.
	if os.Getenv("ULTIMA5_SETTINGS_PROOF_HOST") != "1" {
		return fmt.Errorf("settings proof requires its VM host")
	}
	configuration, err := config.NewUltimaVConfiguration()
	if err != nil {
		return err
	}
	configuration.SoundDisabled = true
	if err := display.GetManager().InitializeMode(config.ModeWindowed, 1280, 960); err != nil {
		return err
	}
	intro, err := CreateIntroMenuScene(configuration)
	if err != nil {
		return err
	}
	defer intro.Dispose()
	path, err := configuration.ConfigurationFilePath()
	if err != nil {
		return err
	}
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	proof := &settingsVMGuest{Game: NewGame(intro), intro: intro, configPath: path, original: original}
	if err := ebiten.RunGame(proof); err != nil {
		return err
	}
	if proof.drawErr != nil {
		return proof.drawErr
	}
	if !proof.complete {
		return fmt.Errorf("host stopped before rollback state and pixels were verified (tick %d)", proof.tick)
	}
	logging.Infof(logging.CategorySystem, "VM settings proof: state, persisted configuration and rendered rollback passed")

	return nil
}

type settingsVMGuest struct {
	*Game
	intro      *IntroMenuScene
	tick       int
	first      *ConfigurationDialog
	configPath string
	original   []byte
	baseline   []byte
	changed    []byte
	drawErr    error
	complete   bool
}

func (proof *settingsVMGuest) Update() error {
	if proof.drawErr != nil {
		return proof.drawErr
	}
	if err := proof.Game.Update(); err != nil {
		return err
	}
	proof.tick++
	if proof.currentScene != proof.intro || proof.intro.config.GameplayConfiguration.AllowRepeatedMonsterDivision {
		return fmt.Errorf("unexpected scene or live gameplay mutation at tick %d", proof.tick)
	}
	dialog, _ := proof.intro.dialogStack.PeekTopModalDialog().(*ConfigurationDialog)
	switch proof.tick {
	case 3:
		if !proof.intro.menuVisible {
			return fmt.Errorf("Enter did not reveal the title menu")
		}
	case 7:
		if proof.intro.nCurrentSelection != introChoiceConfigure || proof.intro.submenuOpen {
			return fmt.Errorf("keyboard navigation selected %d, want Configure", proof.intro.nCurrentSelection)
		}
	case 9:
		if dialog != nil || !proof.intro.submenuOpen || proof.intro.submenuSelection != introConfigureDisplay {
			return fmt.Errorf("Enter did not open the Configure sub-menu on Display")
		}
	case 11:
		if dialog == nil || dialog.currentTab != audioSettingsDisplayTab {
			return fmt.Errorf("Enter did not open Configuration on Display")
		}
		proof.first = dialog
	case 13, 15, 21:
		if dialog == nil || dialog.currentTab != audioSettingsGameplayTab ||
			dialog.stagedGameplay.AllowRepeatedMonsterDivision != (proof.tick == 15) {
			return fmt.Errorf("unexpected staged Gameplay state at tick %d", proof.tick)
		}
		if proof.tick == 21 && dialog == proof.first {
			return fmt.Errorf("cancel/reopen reused the old dialog")
		}
	case 17:
		if dialog != nil || !proof.intro.menuVisible || !proof.intro.submenuOpen {
			return fmt.Errorf("Escape did not return to the title menu's open Configure sub-menu")
		}
	}
	persisted, err := os.ReadFile(proof.configPath)
	if err != nil {
		return err
	}
	if !bytes.Equal(persisted, proof.original) {
		return fmt.Errorf("cancelled configuration edited the persisted file at tick %d", proof.tick)
	}

	return nil
}

func (proof *settingsVMGuest) Draw(screen *ebiten.Image) {
	proof.Game.Draw(screen)
	if proof.tick != 13 && proof.tick != 15 && proof.tick != 21 {
		return
	}
	dialog, ok := proof.intro.dialogStack.PeekTopModalDialog().(*ConfigurationDialog)
	if !ok {
		proof.drawErr = fmt.Errorf("missing Gameplay dialog during frame %d", proof.tick)
		return
	}
	// Compare the actual selected row, excluding the animated title behind the
	// opaque card. ReadPixels crosses the VM boundary and uses host rendering.
	row := dialog.layout.gameplayRows[gameplayRepeatedDivisionIndex].Inset(dialog.layout.pixelUnit)
	if row.Empty() || !row.In(screen.Bounds()) {
		proof.drawErr = fmt.Errorf("Gameplay row is empty or outside the rendered screen: %v", row)
		return
	}
	pixels := make([]byte, row.Dx()*row.Dy()*4)
	screen.SubImage(row).(*ebiten.Image).ReadPixels(pixels)
	switch proof.tick {
	case 13:
		proof.baseline = pixels
	case 15:
		proof.changed = pixels
		if len(proof.baseline) == 0 || bytes.Equal(proof.baseline, pixels) {
			proof.drawErr = fmt.Errorf("toggling Gameplay did not change the rendered row")
		}
	case 21:
		if len(proof.changed) == 0 || bytes.Equal(proof.changed, pixels) || !bytes.Equal(proof.baseline, pixels) {
			proof.drawErr = fmt.Errorf("reopened Gameplay row did not render the original value")
			return
		}
		proof.complete = true
	}
}
