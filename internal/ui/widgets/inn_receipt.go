package widgets

import (
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
)

// FormatInnReceipt supplies the same player-facing outcome to inline and terminal receipts.
// Death announcements remain in the accompanying merchant exchange.
func FormatInnReceipt(display InnActionResultDisplay) string {
	if display.Refused {
		outcome := "You could not stay the night."
		if display.Action == InnActionPickup {
			outcome = display.CharacterName + " remains at the inn. The guards have been called."
		}

		return fmt.Sprintf("%s You need %d gold; no gold was paid.", outcome, display.ListedCharge)
	}
	if display.Action == InnActionLeave {
		return display.CharacterName + " will stay at the inn. No charge today."
	}

	outcome := "You rest until morning."
	if display.Action == InnActionPickup {
		outcome = display.CharacterName + " has been returned to the party."
	}
	parts := []string{outcome}
	if recovery := innReceiptRecovery(display.Members); recovery != "" {
		parts = append(parts, recovery)
	}
	parts = append(parts, fmt.Sprintf("Paid %d gold.", display.ListedCharge))
	if display.FaulineiTheft > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d more gold was stolen! Thou hast %d gold left.", display.FaulineiTheft, display.FinalGold,
		))
	}

	return strings.Join(parts, " ")
}

func innReceiptRecovery(members []game_state.InnCharacterProjection) string {
	health, magic := false, false
	for _, member := range members {
		if member.StatusAfter == party_state.Dead {
			continue
		}
		health = health || member.HPAfter > member.HPBefore
		magic = magic || member.MPAfter > member.MPBefore
	}
	switch {
	case health && magic:
		return "Health and magic restored."
	case health:
		return "Health restored."
	case magic:
		return "Magic restored."
	default:
		return ""
	}
}
