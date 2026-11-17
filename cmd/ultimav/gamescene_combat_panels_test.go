package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
)

// Without a Battle Report panel, battle lines are written to the message window.
func TestCombatPanelsWithoutReportWriteBattleLinesToMessageWindow(t *testing.T) {
	for _, panels := range []int{config.CombatPanelsOrderAndReport, config.CombatPanelsTurnOrderOnly, config.CombatPanelsOff} {
		scene := newCombatReportTestScene(t)
		setCombatHPBarsForTest(scene, true)
		scene.gameConfig.GameplayConfiguration.CombatPanels = panels
		event, _ := injuryReportFixture(t, scene)
		before := scene.output.GetOutputStr(false)
		scene.recordCombatEvent(event)
		written := strings.TrimPrefix(scene.output.GetOutputStr(false), before)
		line := strings.Join(strings.Fields(written), " ")
		if panels == config.CombatPanelsOrderAndReport {
			assert.Empty(t, line)

			continue
		}
		require.Contains(t, line, "hits Orc for 12", "panels=%d", panels)
		assert.False(t, scene.combatHUD.toMessageWindow, "later Battle Report drawing keeps its own pane")
	}
}
