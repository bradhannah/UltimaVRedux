package widgets

import (
	"math"
	"strconv"
	"strings"
)

// readyOtherStrength is the Strength the rest of the loadout would use once the
// item is readied: everything except the item itself.
func readyOtherStrength(item *availableEquipment) int {
	return item.Preview.Resulting.Strength - item.Equipment.GetStrengthRequired()
}

func (d *ReadyEquipmentDialog) appendStrengthBudget(item *availableEquipment) {
	preview := item.Preview
	cost := item.Equipment.GetStrengthRequired()
	remaining := preview.Strength - preview.Resulting.Strength
	resultLabel := "Remaining"
	if remaining < 0 {
		resultLabel, remaining = "Short by", -remaining
	}
	itemCost := readyNoneLabel
	if cost > 0 {
		itemCost = "-" + strconv.Itoa(cost)
	}
	otherCost := readyOtherStrength(item)
	other := strconv.Itoa(otherCost)
	if otherCost > 0 {
		other = "-" + other
	}
	rows := []readyReviewLine{
		{d.getSelectedCharacter().GetNameAsString() + "'s Strength", strconv.Itoa(preview.Strength)},
		{"Other equipment", other},
		{item.Equipment.String(), itemCost},
		{resultLabel, strconv.Itoa(remaining)},
	}
	labelWidth, valueWidth := 0.0, 0.0
	for _, row := range rows {
		label, _ := d.font.MeasureString(row.label)
		value, _ := d.font.MeasureString(row.value)
		labelWidth, valueWidth = max(labelWidth, label), max(valueWidth, value)
	}
	space, _ := d.font.MeasureString(" ")
	width := min(float64(d.detailContentRect().Dx())-space, labelWidth+space+valueWidth)
	d.detailOutput.AppendLine().Text(" ", DialogLabelColor).Done()
	d.detailOutput.AppendLine().Text("Strength budget", DialogHeaderColor).WordWrap().Done()
	for index, row := range rows {
		if index == len(rows)-1 {
			d.detailOutput.AppendLine().Text("---", DialogRuleColor).Done()
		}
		value, _ := d.font.MeasureString(row.value)
		label := row.label
		measured, _ := d.font.MeasureString(label)
		if measured+value+space > width {
			d.detailOutput.AppendLine().Text(" "+label, DialogLabelColor).
				WithContinuationIndent(1).WordWrap().Done()
			label, measured = "", 0
		}
		padding := max(1, int(math.Floor((width-measured-value)/space)))
		valueColor := DialogValueColor
		if index == len(rows)-1 && preview.Resulting.Strength > preview.Strength {
			valueColor = DialogWarningColor
		}
		d.detailOutput.AppendLine().Text(" "+label+strings.Repeat(" ", padding), DialogLabelColor).
			Text(row.value, valueColor).WithContinuationIndent(1).Done()
	}
}
