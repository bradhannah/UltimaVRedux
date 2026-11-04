package main

import "github.com/bradhannah/UltimaVRedux/internal/debugging"

func (g *GameScene) executePresentationDebugCommand(request debugging.Request) (debugging.Result, bool) {
	switch request.CommandID {
	case debugging.CommandLightingOverlaySet:
		before := g.gameState.DebugOptions.LightOverlay
		after := mustDebugBooleanArgument(request, "enabled")
		g.gameState.DebugOptions.LightOverlay = after

		return presentationBooleanResult(request, "Lighting overlay updated.", "presentation.overlays.lighting", before, after), true
	case debugging.CommandSearchOverlaySet:
		before := g.gameState.DebugOptions.SearchOverlay
		after := mustDebugBooleanArgument(request, "enabled")
		g.gameState.DebugOptions.SearchOverlay = after

		return presentationBooleanResult(request, "Search overlay updated.", "presentation.overlays.search", before, after), true
	case debugging.CommandMoongateOverlaySet:
		before := g.overlayRenderer.IsMoonstoneOverlayEnabled()
		after := mustDebugBooleanArgument(request, "enabled")
		g.overlayRenderer.SetMoonstoneOverlayEnabled(after)

		return presentationBooleanResult(request, "Moongate overlay updated.", "presentation.overlays.moongate", before, after), true
	default:
		return debugging.Result{}, false
	}
}

func presentationBooleanResult(request debugging.Request, summary, path string, before, after bool) debugging.Result {
	result := debugSuccessResultWithMode(request, debugging.ExecutionPresentation, summary, struct {
		Enabled bool `json:"enabled"`
	}{Enabled: after})
	appendSemanticChange(&result.Changes, path, before, after)

	return result
}
