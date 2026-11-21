package widgets

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
)

func TestInnReceiptDescribesOnlyObservedChanges(t *testing.T) {
	tests := []struct {
		name    string
		display InnActionResultDisplay
		want    string
	}{
		{
			name: "ordinary rest with recovery",
			display: InnActionResultDisplay{
				Action: InnActionRest, ListedCharge: 8, FinalGold: 92, PartySize: 4,
				Members: []game_state.InnCharacterProjection{{
					StatusAfter: party_state.Good, HPBefore: 1, HPAfter: 30, MPBefore: 0, MPAfter: 12,
				}},
			},
			want: "You rest until morning. Health and magic restored. Paid 8 gold.",
		},
		{
			name: "rest without recovery omits unchanged resources",
			display: InnActionResultDisplay{
				Action: InnActionRest, ListedCharge: 2, FinalGold: 98,
				Members: []game_state.InnCharacterProjection{{
					StatusAfter: party_state.Good, HPBefore: 30, HPAfter: 30, MPBefore: 12, MPAfter: 12,
				}},
			},
			want: "You rest until morning. Paid 2 gold.",
		},
		{
			name: "unusual gold loss does not identify hidden cause",
			display: InnActionResultDisplay{
				Action: InnActionRest, ListedCharge: 2, FaulineiTheft: 4, FinalGold: 0,
			},
			want: "You rest until morning. Paid 2 gold. 4 more gold was stolen! Thou hast 0 gold left.",
		},
		{
			name: "death does not claim recovery even when magic increased",
			display: InnActionResultDisplay{
				Action: InnActionRest, ListedCharge: 2,
				Members: []game_state.InnCharacterProjection{{
					StatusBefore: party_state.Poisoned, StatusAfter: party_state.Dead,
					HPBefore: 20, HPAfter: 0, MPBefore: 0, MPAfter: 12,
				}},
			},
			want: "You rest until morning. Paid 2 gold.",
		},
		{
			name:    "refused rest is not a payment",
			display: InnActionResultDisplay{Action: InnActionRest, ListedCharge: 8, Refused: true},
			want:    "You could not stay the night. You need 8 gold; no gold was paid.",
		},
		{
			name:    "leave identifies companion without unchanged gold or counts",
			display: InnActionResultDisplay{Action: InnActionLeave, CharacterName: "Iolo", FinalGold: 100},
			want:    "Iolo will stay at the inn. No charge today.",
		},
		{
			name: "pickup health recovery",
			display: InnActionResultDisplay{
				Action: InnActionPickup, CharacterName: "Iolo", ListedCharge: 20,
				Members: []game_state.InnCharacterProjection{{StatusAfter: party_state.Good, HPBefore: 1, HPAfter: 30}},
			},
			want: "Iolo has been returned to the party. Health restored. Paid 20 gold.",
		},
		{
			name: "pickup magic recovery",
			display: InnActionResultDisplay{
				Action: InnActionPickup, CharacterName: "Iolo", ListedCharge: 20,
				Members: []game_state.InnCharacterProjection{{StatusAfter: party_state.Good, MPBefore: 1, MPAfter: 12}},
			},
			want: "Iolo has been returned to the party. Magic restored. Paid 20 gold.",
		},
		{
			name: "refused pickup preserves guest and calls guards",
			display: InnActionResultDisplay{
				Action: InnActionPickup, CharacterName: "Iolo", ListedCharge: 20, Refused: true,
			},
			want: "Iolo remains at the inn. The guards have been called. You need 20 gold; no gold was paid.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receipt := FormatInnReceipt(test.display)
			assert.Equal(t, test.want, receipt)
			assert.NotContains(t, receipt, "Faulinei")
			assert.NotContains(t, receipt, "result:")
			assert.NotContains(t, receipt, "rooms")
		})
	}
}
