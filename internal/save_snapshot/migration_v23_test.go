package save_snapshot

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
)

func requireV23RoundTrip(t *testing.T, schema *jsonschema.Schema, snapshot SaveSnapshot) {
	t.Helper()
	envelope := validEnvelope(snapshot)
	envelope.Metadata.PlaythroughID = "0123456789abcdef0123456789abcdef"
	envelope.Metadata.DoomRunID = "abcdef0123456789abcdef0123456789"
	envelope.Metadata.UserDescription = "Continuation and earned evidence"
	data, err := EncodeCanonical(envelope)
	require.NoError(t, err)
	require.NoError(t, schema.Validate(decodeJSONSchemaInstance(t, data)))
	decoded, err := Decode(data)
	require.NoError(t, err)
	require.Equal(t, envelope, decoded)
	again, err := EncodeCanonical(decoded)
	require.NoError(t, err)
	require.Equal(t, data, again, "canonical encoding must preserve all authorities and metadata")
}

func requireV23Rejected(t *testing.T, schema *jsonschema.Schema, snapshot SaveSnapshot, path string) {
	t.Helper()
	_, err := EncodeCanonical(validEnvelope(snapshot))
	require.ErrorIs(t, err, ErrInvalidSnapshot)
	require.ErrorContains(t, err, path)
	data, err := json.Marshal(validEnvelope(snapshot))
	require.NoError(t, err)
	_, err = Decode(data)
	require.ErrorIs(t, err, ErrInvalidSnapshot)
	require.ErrorContains(t, err, path)
	require.Error(t, schema.Validate(decodeJSONSchemaInstance(t, data)))
}

func v23DoorSnapshot() SaveSnapshot {
	snapshot := validSnapshot()
	snapshot.World.Position = PositionSnapshot{Location: int(references.Britain), X: 1, Y: 1}
	snapshot.MapUnits, snapshot.ItemStacks, snapshot.MapMutations = nil, nil, nil
	snapshot.OpenDoor = &OpenDoorSnapshot{
		Position:       PositionSnapshot{Location: int(references.Britain), Floor: 1, X: 10, Y: 14},
		RemainingTurns: 2, ClosedTile: 184,
	}
	return snapshot
}

func TestV23OpenDoorRoundTripAndSchema(t *testing.T) {
	require.Equal(t, 26, CurrentVersion)
	require.Equal(t, 2, CurrentEnvelopeVersion)
	schema, _ := compileCurrentNativeSaveSchema(t)
	requireV23RoundTrip(t, schema, validSnapshot()) // Required openDoor: null.
	for _, tile := range []int{184, 186} {
		for turns := 0; turns <= 2; turns++ {
			for _, floor := range []int{-1, 0, 3} {
				t.Run(fmt.Sprintf("tile-%d-turns-%d-floor-%d", tile, turns, floor), func(t *testing.T) {
					snapshot := v23DoorSnapshot()
					door := snapshot.OpenDoor
					door.RemainingTurns, door.ClosedTile = turns, tile
					door.Position.Floor, door.Position.X, door.Position.Y = floor, 0, 31
					// The player and actors need not occupy an expired door. Its floor
					// may differ from the party's within the same small-map visit.
					requireV23RoundTrip(t, schema, snapshot)
					snapshot.MapMutations = []MapMutationSnapshot{{Position: door.Position, Kind: MapMutationSetObject, Value: tile}}
					requireV23RoundTrip(t, schema, snapshot)
				})
			}
		}
	}
}

func TestV23OpenDoorStrictWireMembers(t *testing.T) {
	schema, _ := compileCurrentNativeSaveSchema(t)
	paths := [][]string{
		{"openDoor"}, {"openDoor", "position"}, {"openDoor", "remainingTurns"}, {"openDoor", "closedTile"},
		{"openDoor", "position", "location"}, {"openDoor", "position", "floor"},
		{"openDoor", "position", "x"}, {"openDoor", "position", "y"},
	}
	for _, path := range paths {
		for _, operation := range []string{"missing", "null", "string", "fraction", "alias", "unknown"} {
			if len(path) == 1 && operation == "null" {
				continue
			}
			t.Run(fmt.Sprint(path, operation), func(t *testing.T) {
				document := currentEnvelopeDocument(t, v23DoorSnapshot())
				parent := document["snapshot"].(map[string]any)
				for _, key := range path[:len(path)-1] {
					parent = parent[key].(map[string]any)
				}
				key := path[len(path)-1]
				switch operation {
				case "missing":
					delete(parent, key)
				case "null":
					parent[key] = nil
				case "string":
					parent[key] = "0"
				case "fraction":
					parent[key] = 0.5
				case "alias":
					parent[strings.ToUpper(key[:1])+key[1:]] = parent[key]
					delete(parent, key)
				case "unknown":
					parent["occupant"] = true
				}
				data, err := json.Marshal(document)
				require.NoError(t, err)
				_, err = Decode(data)
				require.ErrorIs(t, err, ErrInvalidEnvelope)
				require.Error(t, schema.Validate(decodeJSONSchemaInstance(t, data)))
			})
		}
	}
}

func TestV23OpenDoorRejectsInvalidContinuation(t *testing.T) {
	schema, _ := compileCurrentNativeSaveSchema(t)
	for name, mutate := range map[string]func(*SaveSnapshot){
		"negative timer": func(s *SaveSnapshot) { s.OpenDoor.RemainingTurns = -1 },
		"long timer":     func(s *SaveSnapshot) { s.OpenDoor.RemainingTurns = 3 },
		"negative x":     func(s *SaveSnapshot) { s.OpenDoor.Position.X = -1 },
		"negative y":     func(s *SaveSnapshot) { s.OpenDoor.Position.Y = -1 },
		"outside x":      func(s *SaveSnapshot) { s.OpenDoor.Position.X = 32 },
		"outside y":      func(s *SaveSnapshot) { s.OpenDoor.Position.Y = 32 },
		"low floor":      func(s *SaveSnapshot) { s.OpenDoor.Position.Floor = -2 },
		"high floor":     func(s *SaveSnapshot) { s.OpenDoor.Position.Floor = 4 },
		"other visit":    func(s *SaveSnapshot) { s.OpenDoor.Position.Location = int(references.Moonglow) },
		"large map": func(s *SaveSnapshot) {
			s.OpenDoor.Position.Location, s.World.Position.Location = 0, 0
		},
		"dungeon identity": func(s *SaveSnapshot) { s.OpenDoor.Position.Location = int(references.Deceit) },
		"locked door":      func(s *SaveSnapshot) { s.OpenDoor.ClosedTile = 185 },
		"locked view door": func(s *SaveSnapshot) { s.OpenDoor.ClosedTile = 187 },
		"magic door":       func(s *SaveSnapshot) { s.OpenDoor.ClosedTile = int(indexes.MagicLockDoor) },
		"raw open tile":    func(s *SaveSnapshot) { s.OpenDoor.ClosedTile = int(indexes.BrickFloor) },
		"wrong closed override": func(s *SaveSnapshot) {
			s.MapMutations = []MapMutationSnapshot{{Position: s.OpenDoor.Position, Kind: MapMutationSetObject, Value: 186}}
		},
		"raw timed floor override": func(s *SaveSnapshot) {
			s.MapMutations = []MapMutationSnapshot{{Position: s.OpenDoor.Position, Kind: MapMutationSetObject, Value: int(indexes.BrickFloor)}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := v23DoorSnapshot()
			mutate(&snapshot)
			requireV23Rejected(t, schema, snapshot, "openDoor")
		})
	}
}

func v23ReceiptEvidence(hour int, flexible bool) journal.AtlasLandmarkObservation {
	site := journal.SiteLocation{X: 104, Y: 216}
	at := journal.OptionalGameTime{Known: true, Time: journal.GameTime{Year: 139, Month: 4, Day: 5, Hour: hour}}
	named := at
	named.Time.Day--
	return journal.AtlasLandmarkObservation{
		SiteKey: 11, Site: site, Kind: "lighthouse", Name: "Greyhaven",
		Source: journal.AtlasSourceSextantReceipt, NameSource: journal.AtlasSourceEnter,
		ObservedAt: at, NamedAt: named, SurveySource: journal.AtlasSourceSextantReceipt,
		Survey: journal.SurveyedLocation{Measured: true, Flexible: flexible, Location: site, RecordedAt: at},
	}
}

func v23GuardiansEvidence() journal.AtlasLandmarkObservation {
	site := journal.SiteLocation{X: 233, Y: 235}
	at := journal.OptionalGameTime{Known: true, Time: journal.GameTime{Year: 139, Month: 4, Day: 5, Hour: 21}}
	return journal.AtlasLandmarkObservation{
		SiteKey: 108, Site: site, Kind: "guardians", Source: journal.AtlasSourceLook, ObservedAt: at,
		Survey: journal.SurveyedLocation{Measured: true, Location: site, RecordedAt: at}, SurveySource: journal.AtlasSourceLook,
	}
}

func TestV23ReceiptPreservesTruthfulPolicyAndPriorNaming(t *testing.T) {
	schema, _ := compileCurrentNativeSaveSchema(t)
	for hour := range 24 {
		for _, flexible := range []bool{false, true} {
			for _, source := range []journal.AtlasObservationSource{journal.AtlasSourceSextantReceipt, journal.AtlasSourceLook, journal.AtlasSourceEnter} {
				t.Run(fmt.Sprintf("hour-%d-flexible-%t-source-%s", hour, flexible, source), func(t *testing.T) {
					snapshot := validSnapshot()
					record := v23ReceiptEvidence(hour, flexible)
					if source != journal.AtlasSourceSextantReceipt {
						record.Source = source
						record.ObservedAt.Time.Day++ // A later observation retains receipt authority.
					}
					snapshot.AtlasLandmarks = []journal.AtlasLandmarkObservation{record}
					requireV23RoundTrip(t, schema, snapshot)
					// Older legitimate Enter evidence need not have a naming date.
					snapshot.AtlasLandmarks[0].NamedAt = journal.OptionalGameTime{}
					requireV23RoundTrip(t, schema, snapshot)
				})
			}
		}
	}
}

func TestV23ReceiptRejectsBorrowedAuthority(t *testing.T) {
	schema, _ := compileCurrentNativeSaveSchema(t)
	for name, mutate := range map[string]func(*journal.AtlasLandmarkObservation){
		"other settlement": func(o *journal.AtlasLandmarkObservation) { o.SiteKey = 2 },
		"other lighthouse": func(o *journal.AtlasLandmarkObservation) { o.SiteKey = 12 },
		"shrine":           func(o *journal.AtlasLandmarkObservation) { o.SiteKey = 101 },
		"Guardians":        func(o *journal.AtlasLandmarkObservation) { o.SiteKey = 108 },
		"wrong kind":       func(o *journal.AtlasLandmarkObservation) { o.Kind = "towne" },
		"wrong x": func(o *journal.AtlasLandmarkObservation) {
			o.Site.X++
			o.Survey.Location = o.Site
		},
		"wrong y": func(o *journal.AtlasLandmarkObservation) {
			o.Site.Y++
			o.Survey.Location = o.Site
		},
		"interior": func(o *journal.AtlasLandmarkObservation) {
			o.Site.Map = references.Greyhaven
			o.Survey.Location = o.Site
		},
		"underworld": func(o *journal.AtlasLandmarkObservation) {
			o.Site.Floor = -1
			o.Survey.Location = o.Site
		},
		"unnamed": func(o *journal.AtlasLandmarkObservation) {
			o.Name, o.NameSource, o.NamedAt = "", "", journal.OptionalGameTime{}
		},
		"receipt names":     func(o *journal.AtlasLandmarkObservation) { o.NameSource = journal.AtlasSourceSextantReceipt },
		"look names":        func(o *journal.AtlasLandmarkObservation) { o.NameSource = journal.AtlasSourceLook },
		"unknown source":    func(o *journal.AtlasLandmarkObservation) { o.Source = "purchase" },
		"shrine source":     func(o *journal.AtlasLandmarkObservation) { o.Source = journal.AtlasSourceShrineIntroduction },
		"look measurement":  func(o *journal.AtlasLandmarkObservation) { o.SurveySource = journal.AtlasSourceLook },
		"enter measurement": func(o *journal.AtlasLandmarkObservation) { o.SurveySource = journal.AtlasSourceEnter },
		"absent measurement": func(o *journal.AtlasLandmarkObservation) {
			o.Survey, o.SurveySource = journal.SurveyedLocation{}, ""
		},
		"other measured site": func(o *journal.AtlasLandmarkObservation) { o.Survey.Location.X++ },
		"absent observation":  func(o *journal.AtlasLandmarkObservation) { o.ObservedAt = journal.OptionalGameTime{} },
		"absent survey date":  func(o *journal.AtlasLandmarkObservation) { o.Survey.RecordedAt = journal.OptionalGameTime{} },
		"zero year":           func(o *journal.AtlasLandmarkObservation) { o.Survey.RecordedAt.Time.Year = 0 },
		"invalid month":       func(o *journal.AtlasLandmarkObservation) { o.Survey.RecordedAt.Time.Month = 14 },
		"invalid day":         func(o *journal.AtlasLandmarkObservation) { o.Survey.RecordedAt.Time.Day = 29 },
		"invalid hour":        func(o *journal.AtlasLandmarkObservation) { o.Survey.RecordedAt.Time.Hour = 24 },
		"invalid minute":      func(o *journal.AtlasLandmarkObservation) { o.Survey.RecordedAt.Time.Minute = 60 },
		"future survey":       func(o *journal.AtlasLandmarkObservation) { o.Survey.RecordedAt.Time.Day++ },
		"unequal receipt dates": func(o *journal.AtlasLandmarkObservation) {
			o.ObservedAt.Time.Day++
		},
		"name learned after receipt": func(o *journal.AtlasLandmarkObservation) {
			o.Source = journal.AtlasSourceLook
			o.ObservedAt.Time.Day += 2
			o.NamedAt = o.Survey.RecordedAt
			o.NamedAt.Time.Day++
		},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := validSnapshot()
			// Nighttime makes provenance failures independent of the daytime exception.
			record := v23ReceiptEvidence(21, false)
			mutate(&record)
			snapshot.AtlasLandmarks = []journal.AtlasLandmarkObservation{record}
			requireV23Rejected(t, schema, snapshot, "atlasLandmarks")
		})
	}
	// A receipt elsewhere in the same checkpoint never licenses ordinary daytime
	// observations, including a non-receipt measurement at Greyhaven itself.
	for _, family := range []string{"dungeon", "moonstone", "atlas", "Greyhaven"} {
		t.Run("no policy leak to "+family, func(t *testing.T) {
			snapshot := v22EvidenceSnapshot(12, true)
			snapshot.AtlasLandmarks = append(snapshot.AtlasLandmarks, v23ReceiptEvidence(12, false))
			switch family {
			case "dungeon":
				snapshot.DungeonJournal[0].Sites[0].Survey.Flexible = false
			case "moonstone":
				snapshot.MoonstoneJournal[3].Survey.Flexible = false
			case "atlas":
				snapshot.AtlasLandmarks[0].Survey.Flexible = false
			case "Greyhaven":
				snapshot.AtlasLandmarks[1].Source, snapshot.AtlasLandmarks[1].SurveySource = journal.AtlasSourceEnter, journal.AtlasSourceEnter
			}
			requireV23Rejected(t, schema, snapshot, "nighttime")
		})
	}
}

func TestV23GuardiansEvidenceAndBoundedAtlas(t *testing.T) {
	schema, _ := compileCurrentNativeSaveSchema(t)
	snapshot := validSnapshot()
	record := v23GuardiansEvidence()
	snapshot.AtlasLandmarks = []journal.AtlasLandmarkObservation{record}
	requireV23RoundTrip(t, schema, snapshot)
	snapshot.AtlasLandmarks[0].Survey, snapshot.AtlasLandmarks[0].SurveySource = journal.SurveyedLocation{}, ""
	requireV23RoundTrip(t, schema, snapshot) // Look can recognize without measuring or naming.
	snapshot.AtlasLandmarks[0].Survey = record.Survey
	snapshot.AtlasLandmarks[0].Survey.Flexible = true
	snapshot.AtlasLandmarks[0].Survey.RecordedAt.Time.Hour = 12
	snapshot.AtlasLandmarks[0].SurveySource = journal.AtlasSourceLook
	requireV23RoundTrip(t, schema, snapshot)
	for name, mutate := range map[string]func(*journal.AtlasLandmarkObservation){
		"wrong key":    func(o *journal.AtlasLandmarkObservation) { o.SiteKey = 107 },
		"unknown key":  func(o *journal.AtlasLandmarkObservation) { o.SiteKey = 109 },
		"wrong kind":   func(o *journal.AtlasLandmarkObservation) { o.Kind = "castle" },
		"enter":        func(o *journal.AtlasLandmarkObservation) { o.Source = journal.AtlasSourceEnter },
		"receipt":      func(o *journal.AtlasLandmarkObservation) { o.Source = journal.AtlasSourceSextantReceipt },
		"enter survey": func(o *journal.AtlasLandmarkObservation) { o.SurveySource = journal.AtlasSourceEnter },
		"receipt survey": func(o *journal.AtlasLandmarkObservation) {
			o.SurveySource = journal.AtlasSourceSextantReceipt
		},
		"invented name": func(o *journal.AtlasLandmarkObservation) {
			o.Name, o.NameSource, o.NamedAt = "Guardians", journal.AtlasSourceEnter, o.ObservedAt
		},
		"wrong anchor": func(o *journal.AtlasLandmarkObservation) {
			o.Site.X++
			o.Survey.Location = o.Site
		},
		"strict daylight": func(o *journal.AtlasLandmarkObservation) { o.Survey.RecordedAt.Time.Hour = 12 },
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := validSnapshot()
			record := v23GuardiansEvidence()
			mutate(&record)
			snapshot.AtlasLandmarks = []journal.AtlasLandmarkObservation{record}
			requireV23Rejected(t, schema, snapshot, "atlasLandmarks")
		})
	}
	snapshot.AtlasLandmarks = v23FullAtlas()
	require.Len(t, snapshot.AtlasLandmarks, 39)
	requireV23RoundTrip(t, schema, snapshot)
	snapshot.AtlasLandmarks = append(snapshot.AtlasLandmarks, v23GuardiansEvidence())
	requireV23Rejected(t, schema, snapshot, "atlasLandmarks")
}

func v23FullAtlas() []journal.AtlasLandmarkObservation {
	var records []journal.AtlasLandmarkObservation
	for key := 1; key <= 32; key++ {
		if key != 25 {
			records = append(records, journal.AtlasLandmarkObservation{
				SiteKey: uint8(key), Site: journal.SiteLocation{X: key, Y: key}, Kind: "towne", Source: journal.AtlasSourceLook,
			})
		}
	}
	for index, position := range references.AtlasShrinePositions() {
		records = append(records, journal.AtlasLandmarkObservation{
			SiteKey: uint8(101 + index), Site: journal.SiteLocation{X: int(position.X), Y: int(position.Y)},
			Kind: "shrine", Source: journal.AtlasSourceLook,
		})
	}
	return append(records, v23GuardiansEvidence())
}
