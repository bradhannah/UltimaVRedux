package widgets

import (
	"fmt"
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/sprites"
)

type readyCharacterHeaderLayout struct {
	portrait image.Rectangle
	name     image.Rectangle
	position image.Rectangle
	strength image.Rectangle
}

func (d *ReadyEquipmentDialog) characterPositionLabel() string {
	return fmt.Sprintf("CHAR %d/%d", d.selectedCharIdx+1, d.partyState.GetActivePartyCount())
}

// Reserve actual glyph heights and the position label before fitting the name.
// The portrait is an integer multiple of the original tile, never filtered/stretch-scaled.
func (d *ReadyEquipmentDialog) characterHeaderLayout() readyCharacterHeaderLayout {
	bounds := d.charBorder.GetInteriorBounds().Inset(itemDialogTextPadding)
	titleHeight := int(math.Ceil(d.titleOutput.GetTotalHeightForLines(1)))
	bodyHeight := int(math.Ceil(d.charOutput.GetTotalHeightForLines(1)))
	textHeight := titleHeight + itemDialogTextPadding + bodyHeight
	top := bounds.Min.Y + max(0, (bounds.Dy()-textHeight)/readyCenterDivisor)
	size := max(sprites.TileSize, bounds.Dy()/sprites.TileSize*sprites.TileSize)
	portraitTop := bounds.Min.Y + (bounds.Dy()-size)/readyCenterDivisor
	portrait := image.Rect(bounds.Min.X, portraitTop, bounds.Min.X+size, portraitTop+size)
	textLeft := portrait.Max.X + itemDialogTextPadding
	positionWidth, _ := d.font.MeasureString(d.characterPositionLabel())
	positionLeft := bounds.Max.X - int(math.Ceil(positionWidth))

	return readyCharacterHeaderLayout{
		portrait: portrait,
		name:     image.Rect(textLeft, top, positionLeft-itemDialogTextPadding, top+titleHeight),
		position: image.Rect(positionLeft, top, bounds.Max.X, top+bodyHeight),
		strength: image.Rect(textLeft, top+titleHeight+itemDialogTextPadding, bounds.Max.X, top+textHeight),
	}
}

func (d *ReadyEquipmentDialog) drawCharacterPanel(screen *ebiten.Image) {
	layout := d.characterHeaderLayout()
	d.populateCharacterOutput(layout)
	if char := d.getSelectedCharacter(); char != nil && char.IsAlive() {
		d.drawCharacterSprite(screen, layout.portrait)
	}
	d.titleOutput.DrawPanelContent(screen, layout.name.Min, layout.name.Dx(), etext.AlignStart)
	d.charOutput.DrawPanelContent(screen, layout.strength.Min, layout.strength.Dx(), etext.AlignStart)
	d.indicatorOutput.DrawPanelContent(screen, layout.position.Min, layout.position.Dx(), etext.AlignStart)
}

func (d *ReadyEquipmentDialog) populateCharacterOutput(layout readyCharacterHeaderLayout) {
	char := d.getSelectedCharacter()
	d.titleOutput.Reset()
	d.charOutput.Reset()
	if char != nil && char.IsAlive() {
		name := FitDialogLine(char.GetNameAsString(), d.titleFont, layout.name.Dx())
		d.titleOutput.AppendLine().Text(name, DialogHeaderColor).Done()
		used := char.GetTotalEquippedStrRequirement()
		line := d.charOutput.AppendLine().Text("Strength in use: ", DialogLabelColor).
			Textf("%d of %d", DialogValueColor, used, char.Strength)
		if over := used - int(char.Strength); over > 0 {
			line.Textf("   %d over", DialogWarningColor, over)
		}
		line.Done()
	} else {
		d.titleOutput.AppendLine().Text("CHARACTER", DialogHeaderColor).Done()
		d.charOutput.AppendLine().Text("[No character]", DialogEmptyColor).Done()
	}
	d.indicatorOutput.Reset()
	d.indicatorOutput.AppendLine().Text("CHAR ", DialogLabelColor).
		Textf("%d/%d", DialogValueColor, d.selectedCharIdx+1, d.partyState.GetActivePartyCount()).Done()
}

func (d *ReadyEquipmentDialog) drawCharacterSprite(screen *ebiten.Image, bounds image.Rectangle) {
	if d.spriteSheet == nil || d.spriteSheet.SpriteImage == nil {
		return
	}
	sprite := d.spriteSheet.GetRawSprite(d.getSelectedCharacter().GetKeySpriteIndex())
	scale := float64(bounds.Dx() / sprites.TileSize)
	d.drawOp.GeoM.Reset()
	d.drawOp.Filter = ebiten.FilterNearest
	d.drawOp.GeoM.Scale(scale, scale)
	d.drawOp.GeoM.Translate(float64(bounds.Min.X), float64(bounds.Min.Y))
	screen.DrawImage(sprite, d.drawOp)
}
