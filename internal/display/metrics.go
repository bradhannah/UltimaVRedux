package display

// DeviceScaleFactor returns the last observed scale, independent of window size.
func (dm *DisplayManager) DeviceScaleFactor() float64 { return dm.deviceScale }

// LayoutSize observes fractional outside dimensions and chooses the automatic
// draw canvas. Ebitengine calls Layout before the first Update.
func (dm *DisplayManager) LayoutSize(width, height float64) (int, int) {
	renderWidth, renderHeight := RenderSize(width, height, dm.backend.DeviceScaleFactor())
	dm.PublishLogicalSize(renderWidth, renderHeight)

	return renderWidth, renderHeight
}

// PublishOutputSize records actual final drawable pixels, including aspect bars.
// Zeroes mean no final frame has been observed yet; these are not monitor pixels.
func (dm *DisplayManager) PublishOutputSize(width, height int) {
	dm.outputWidth, dm.outputHeight = width, height
}

// OutputSize returns the last observed final drawable dimensions in pixels.
func (dm *DisplayManager) OutputSize() (int, int) { return dm.outputWidth, dm.outputHeight }

// FitWindowSize constrains startup geometry without changing its persisted unit.
func (dm *DisplayManager) FitWindowSize(width, height int) (int, int) {
	return FitWindowSize(width, height, dm.monitorWidth, dm.monitorHeight, dm.deviceScale)
}
