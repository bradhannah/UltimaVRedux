package main

import (
	"errors"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/character_creation"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestCharacterCreationMenuHandoffUsesFixedSeedAndDebouncesHeldEnter(t *testing.T) {
	held := map[ebiten.Key]bool{ebiten.KeyEnter: true}
	intro := newIntroInputTestScene()
	intro.config.SavedConfigData.DataFilePath = t.TempDir()
	openIntroSubmenu(intro, introChoiceNewGame, introNewGameCreate)
	intro.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })
	title := ebiten.NewImage(1, 1)
	t.Cleanup(title.Deallocate)
	title.Fill(ucolor.EGABlue)
	flame := ebiten.NewImage(1, 1)
	t.Cleanup(flame.Deallocate)
	flame.Fill(ucolor.EGARed)
	intro.introSprites = new(sprites.IntroSprites)
	intro.introSprites.Ultima16Logo = title
	intro.introSprites.FlameAnimation = sprites.NewImageAnimation([]*ebiten.Image{flame}, 100)
	game := NewGame(intro)
	seedCalls := 0
	game.characterCreationSeedProvider = func() (uint64, error) {
		seedCalls++

		return 42, nil
	}
	var creation *CharacterCreationScene
	game.characterCreationSceneBuilder = func(
		configuration *config.UltimaVConfiguration,
		source *beradrng.Source,
		callbacks CharacterCreationCallbacks,
		identityPresentation *characterCreationIdentityPresentation,
	) (Scene, error) {
		assert.Same(t, intro.config, configuration)
		assert.Equal(t, uint64(42), source.Seed())
		assert.NotNil(t, identityPresentation)
		assert.Equal(t, int64(introUpdateInterval), identityPresentation.elapsedMs)
		fixture := newCharacterCreationSceneFixture(t, callbacks)
		creation = fixture.scene
		creation.config = configuration
		creation.source = source
		creation.identityPresentation = identityPresentation
		creation.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })

		return creation, nil
	}

	require.NoError(t, game.Update())
	require.Same(t, creation, game.currentScene)
	assert.Equal(t, 1, seedCalls)
	assert.True(t, intro.disposed)
	assert.Equal(t, characterCreationName, creation.state)
	screen := ebiten.NewImage(introLogicalWidth, introLogicalHeight)
	t.Cleanup(screen.Deallocate)
	creation.Draw(screen)
	assertCharacterCreationPixel(t, creation.canvas, 0, 0, ucolor.EGABlue)
	assertCharacterCreationPixel(t, creation.canvas, introFlameX, introFlameY, ucolor.EGARed)

	require.NoError(t, game.Update())
	assert.Equal(t, characterCreationName, creation.state)
	assert.False(t, creation.terminal)
	held[ebiten.KeyEnter] = false
	require.NoError(t, game.Update())
	assert.Equal(t, characterCreationName, creation.state)
}

func TestCharacterCreationMenuHandoffFailureLeavesMenuActive(t *testing.T) {
	startupErr := errors.New("creation startup failed")
	for _, test := range []struct {
		name      string
		configure func(*Game, *int)
	}{
		{
			name: "seed provider",
			configure: func(game *Game, builds *int) {
				game.characterCreationSeedProvider = func() (uint64, error) { return 0, startupErr }
				game.characterCreationSceneBuilder = func(
					*config.UltimaVConfiguration,
					*beradrng.Source,
					CharacterCreationCallbacks,
					*characterCreationIdentityPresentation,
				) (Scene, error) {
					*builds++

					return &playerSaveTestScene{}, nil
				}
			},
		},
		{
			name: "scene builder",
			configure: func(game *Game, builds *int) {
				game.characterCreationSeedProvider = func() (uint64, error) { return 42, nil }
				game.characterCreationSceneBuilder = func(
					*config.UltimaVConfiguration,
					*beradrng.Source,
					CharacterCreationCallbacks,
					*characterCreationIdentityPresentation,
				) (Scene, error) {
					*builds++

					return nil, startupErr
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			intro := newIntroInputTestScene()
			openIntroSubmenu(intro, introChoiceNewGame, introNewGameCreate)
			game := NewGame(intro)
			builds := 0
			test.configure(game, &builds)
			generation := game.sceneGeneration.Load()
			intro.keyboard.SimulateKeyPress(ebiten.KeyEnter)

			require.NoError(t, game.Update())
			assert.Same(t, intro, game.currentScene)
			assert.Equal(t, generation, game.sceneGeneration.Load())
			assert.False(t, intro.disposed)
			assert.True(t, intro.active)
			assert.Equal(t, "Character creation could not be started.", intro.notice)
			if test.name == "seed provider" {
				assert.Zero(t, builds)
			} else {
				assert.Equal(t, 1, builds)
			}
		})
	}
}

func TestCompletedCharacterCreationRetriesPreparationAndSaveSelection(t *testing.T) {
	game, scene, result := newCompletedCharacterCreationFlow(t)
	snapshot := validCommandTestSnapshot()
	preparationErr := errors.New("template unavailable")
	preparations := 0
	game.characterCreationSnapshotPreparer = func(
		path string,
		got character_creation.Result,
	) (save_snapshot.SaveSnapshot, error) {
		preparations++
		assert.Equal(t, scene.config.SavedConfigData.DataFilePath, path)
		assert.Equal(t, result, got)
		if preparations == 1 {
			return save_snapshot.SaveSnapshot{}, preparationErr
		}

		return snapshot, nil
	}
	repository := newCharacterCreationFlowRepository(player_save.Slot01, player_save.SlotEmpty, "")
	game.ConfigurePlayerSaveRepository(repository, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
		return &playerSaveTestScene{}, nil
	})

	game.completeCharacterCreation(result)
	assert.Zero(t, scene.dialogStack.Count())
	assert.Equal(t, "New game could not be prepared.", scene.terminalNotice)
	assert.Equal(t, "[Enter] Retry", scene.terminalHint)
	assertCharacterCreationResultRetained(t, scene, result)

	pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyEnter)
	require.IsType(t, &widgets.SaveSlotBrowser{}, scene.dialogStack.PeekTopModalDialog())
	assert.Equal(t, 2, preparations)

	pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyEscape)
	assert.Zero(t, scene.dialogStack.Count())
	assert.Equal(t, "Character complete.", scene.terminalNotice)
	assert.Equal(t, "[Enter] Choose a save slot", scene.terminalHint)
	assertCharacterCreationResultRetained(t, scene, result)

	pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyEnter)
	require.IsType(t, &widgets.SaveSlotBrowser{}, scene.dialogStack.PeekTopModalDialog())
	assert.Equal(t, 2, preparations, "reopening the browser reuses the prepared snapshot")
}

func TestCompletedCharacterCreationWaitsForConclusionKeyReleaseBeforeSaving(t *testing.T) {
	game, scene, result := newCompletedCharacterCreationFlow(t)
	held := map[ebiten.Key]bool{ebiten.KeyEnter: true}
	scene.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })
	scene.keyboard.PrepareForTransition(0)
	repository := newCharacterCreationFlowRepository(player_save.Slot01, player_save.SlotEmpty, "")
	candidate := &playerSaveTestScene{}
	openCharacterCreationFlowBrowser(t, game, scene, result, repository, candidate)

	require.NoError(t, scene.Update(game))
	assert.Empty(t, repository.saves)
	held[ebiten.KeyEnter] = false
	require.NoError(t, scene.Update(game))
	assert.Empty(t, repository.saves)

	held[ebiten.KeyEnter] = true
	require.NoError(t, scene.Update(game))
	assert.Empty(t, repository.saves)
	editor, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveDescriptionEntryModal)
	require.True(t, ok)
	require.NoError(t, scene.Update(game))
	assert.Empty(t, repository.saves, "held opening Enter must not confirm")
	editor.Submit()
	require.Len(t, repository.saves, 1)
	assert.Same(t, candidate, game.currentScene)
}

func TestCharacterCreationCancellationReturnsToFreshMenuAndRetriesBuildFailure(t *testing.T) {
	configuration := testCharacterCreationConfig(t.TempDir())
	var game *Game
	fixture := newCharacterCreationSceneFixture(t, CharacterCreationCallbacks{
		OnComplete: func(character_creation.Result) { t.Fatal("unexpected completion") },
		OnCancel: func() {
			game.cancelCharacterCreation(configuration)
		},
	})
	fixture.scene.config = configuration
	game = NewGame(fixture.scene)
	menuBuildErr := errors.New("menu assets unavailable")
	menu := &playerSaveTestScene{configuration: configuration}
	builds := 0
	game.mainMenuSceneBuilder = func(*config.UltimaVConfiguration) (Scene, error) {
		builds++
		if builds == 1 {
			return nil, menuBuildErr
		}

		return menu, nil
	}

	pressCharacterCreationFlowKey(t, game, fixture.scene, ebiten.KeyEscape)
	assert.Same(t, fixture.scene, game.currentScene)
	assert.Equal(t, characterCreationCancelled, fixture.scene.state)
	assert.Equal(t, "The main menu could not be restored.", fixture.scene.terminalNotice)
	assert.Equal(t, "[Enter] Retry", fixture.scene.terminalHint)
	_, available := fixture.scene.Result()
	assert.False(t, available)

	pressCharacterCreationFlowKey(t, game, fixture.scene, ebiten.KeyEnter)
	assert.Same(t, menu, game.currentScene)
	assert.True(t, fixture.scene.disposed)
	assert.Equal(t, 2, builds)
}

func TestCharacterCreationSaveIntoEmptySlotActivatesGameplayExactlyOnce(t *testing.T) {
	game, scene, result := newCompletedCharacterCreationFlow(t)
	snapshot := validCommandTestSnapshot()
	snapshot.MarkMapUnitsForReconstruction()
	canonical := canonicalPlayerSaveTestSnapshot()
	repository := newCharacterCreationFlowRepository(player_save.Slot03, player_save.SlotEmpty, "")
	candidate := &playerSaveTestScene{configuration: scene.config, snapshot: canonical}
	game.characterCreationSnapshotPreparer = func(string, character_creation.Result) (save_snapshot.SaveSnapshot, error) {
		return snapshot, nil
	}
	game.ConfigurePlayerSaveRepository(repository, func(
		configuration *config.UltimaVConfiguration,
		got save_snapshot.SaveSnapshot,
	) (Scene, error) {
		assert.Same(t, scene.config, configuration)
		assert.Equal(t, snapshot, got)

		return candidate, nil
	})

	game.completeCharacterCreation(result)
	pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyEnter)
	confirmCharacterCreationOverwrite(t, game, scene)

	require.Len(t, repository.saves, 1)
	save := repository.saves[0]
	assert.Equal(t, player_save.Slot03, save.slot)
	assert.Empty(t, save.expectedRevision)
	assert.Equal(t, canonical, save.envelope.Snapshot)
	assertCanonicalPreparedPlayerSave(t, save.envelope.Snapshot)
	assert.Equal(t, playerSaveNewGameDescription, save.envelope.Metadata.Description)
	assert.Equal(t, 1, repository.compatibilityChecks)
	assert.Same(t, candidate, game.currentScene)
	assert.True(t, scene.disposed)
	assert.Equal(t, player_save.Slot03, candidate.slot)
	assert.Equal(t, repository.records[player_save.Slot03].Revision, candidate.revision)
	assert.Equal(t, 1, candidate.captures)
	assert.False(t, candidate.provenanceAtCapture)
}

func TestCharacterCreationSaveConfirmsReviewedOverwrite(t *testing.T) {
	for _, status := range []player_save.SlotStatus{player_save.SlotReady, player_save.SlotIncompatible} {
		t.Run(string(status), func(t *testing.T) {
			game, scene, result := newCompletedCharacterCreationFlow(t)
			snapshot := validCommandTestSnapshot()
			snapshot.MarkMapUnitsForReconstruction()
			existing := playerSaveTestRecord(player_save.Slot02, "reviewed", snapshot)
			repository := newCharacterCreationFlowRepository(player_save.Slot02, status, "reviewed")
			repository.records[player_save.Slot02] = existing
			candidate := &playerSaveTestScene{configuration: scene.config, snapshot: canonicalPlayerSaveTestSnapshot()}
			game.characterCreationSnapshotPreparer = func(string, character_creation.Result) (save_snapshot.SaveSnapshot, error) {
				return snapshot, nil
			}
			game.ConfigurePlayerSaveRepository(repository, func(
				*config.UltimaVConfiguration,
				save_snapshot.SaveSnapshot,
			) (Scene, error) {
				return candidate, nil
			})

			game.completeCharacterCreation(result)
			pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyDown)
			pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyEnter)
			assert.IsType(t, &widgets.SaveDescriptionEntryModal{}, scene.dialogStack.PeekTopModalDialog())
			assert.Equal(t, 2, scene.dialogStack.Count())
			assert.Empty(t, repository.saves)

			confirmCharacterCreationOverwrite(t, game, scene)

			require.Len(t, repository.saves, 1)
			assert.Equal(t, "reviewed", repository.saves[0].expectedRevision)
			assertCanonicalPreparedPlayerSave(t, repository.saves[0].envelope.Snapshot)
			assert.Same(t, candidate, game.currentScene)
		})
	}
}

func TestCharacterCreationSaveRejectsUnsafeAndStaleDestinations(t *testing.T) {
	for _, status := range []player_save.SlotStatus{player_save.SlotCorrupt, player_save.SlotUnavailable} {
		t.Run(string(status), func(t *testing.T) {
			game, scene, result := newCompletedCharacterCreationFlow(t)
			repository := newCharacterCreationFlowRepository(player_save.Slot06, status, "")
			openCharacterCreationFlowBrowser(t, game, scene, result, repository, &playerSaveTestScene{})

			pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyEnter)
			assert.IsType(t, &widgets.SaveSlotBrowser{}, scene.dialogStack.PeekTopModalDialog())
			assert.Empty(t, repository.saves)
		})
	}

	t.Run("stale overwrite revision", func(t *testing.T) {
		game, scene, result := newCompletedCharacterCreationFlow(t)
		snapshot := validCommandTestSnapshot()
		repository := newCharacterCreationFlowRepository(player_save.Slot07, player_save.SlotReady, "reviewed")
		repository.records[player_save.Slot07] = playerSaveTestRecord(player_save.Slot07, "changed", snapshot)
		candidate := &playerSaveTestScene{}
		openCharacterCreationFlowBrowser(t, game, scene, result, repository, candidate)
		pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyDown)
		pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyEnter)

		confirmCharacterCreationOverwrite(t, game, scene)

		assert.Same(t, scene, game.currentScene)
		assert.Empty(t, repository.saves)
		assert.Equal(t, 1, candidate.captures)
		assert.Equal(t, 1, candidate.disposals)
		assert.IsType(t, &widgets.SaveSlotBrowser{}, scene.dialogStack.PeekTopModalDialog())
		assertCharacterCreationResultRetained(t, scene, result)
	})
}

func TestCharacterCreationSaveFailuresRetainCompletedResult(t *testing.T) {
	failure := errors.New("save transaction failed")
	for _, test := range []struct {
		name          string
		configure     func(*playerSaveTestRepository, *playerSaveTestScene)
		wantSaves     int
		wantDeletes   int
		wantDisposals int
	}{
		{
			name: "compatibility",
			configure: func(repository *playerSaveTestRepository, _ *playerSaveTestScene) {
				repository.compatibilityErr = failure
			},
			wantSaves: 0, wantDeletes: 0, wantDisposals: 0,
		},
		{
			name: "preflight",
			configure: func(_ *playerSaveTestRepository, candidate *playerSaveTestScene) {
				candidate.preflightErr = failure
			},
			wantSaves: 0, wantDeletes: 0, wantDisposals: 1,
		},
		{
			name: "capture",
			configure: func(_ *playerSaveTestRepository, candidate *playerSaveTestScene) {
				candidate.captureErr = failure
			},
			wantSaves: 0, wantDeletes: 0, wantDisposals: 1,
		},
		{
			name: "persistence",
			configure: func(repository *playerSaveTestRepository, _ *playerSaveTestScene) {
				repository.saveErr = failure
			},
			wantSaves: 1, wantDeletes: 0, wantDisposals: 1,
		},
		{
			name: "activation",
			configure: func(_ *playerSaveTestRepository, candidate *playerSaveTestScene) {
				candidate.activatePanic = true
			},
			wantSaves: 1, wantDeletes: 1, wantDisposals: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			game, scene, result := newCompletedCharacterCreationFlow(t)
			repository := newCharacterCreationFlowRepository(player_save.Slot04, player_save.SlotEmpty, "")
			candidate := &playerSaveTestScene{}
			test.configure(repository, candidate)
			openCharacterCreationFlowBrowser(t, game, scene, result, repository, candidate)

			pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyEnter)
			confirmCharacterCreationOverwrite(t, game, scene)

			assert.Same(t, scene, game.currentScene)
			assert.Len(t, repository.saves, test.wantSaves)
			assert.Len(t, repository.deletes, test.wantDeletes)
			assert.Equal(t, test.wantDisposals, candidate.disposals)
			assert.IsType(t, &widgets.SaveSlotBrowser{}, scene.dialogStack.PeekTopModalDialog())
			assertCharacterCreationResultRetained(t, scene, result)
		})
	}
}

func TestCharacterCreationActivationFailureRestoresReviewedOverwrite(t *testing.T) {
	game, scene, result := newCompletedCharacterCreationFlow(t)
	snapshot := validCommandTestSnapshot()
	existing := playerSaveTestRecord(player_save.Slot05, "reviewed", snapshot)
	repository := newCharacterCreationFlowRepository(player_save.Slot05, player_save.SlotReady, "reviewed")
	repository.records[player_save.Slot05] = existing
	candidate := &playerSaveTestScene{activatePanic: true}
	openCharacterCreationFlowBrowser(t, game, scene, result, repository, candidate)
	pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyDown)
	pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyEnter)

	confirmCharacterCreationOverwrite(t, game, scene)

	assert.Same(t, scene, game.currentScene)
	require.Len(t, repository.saves, 1)
	require.Len(t, repository.restores, 1)
	assert.Equal(t, existing, repository.records[player_save.Slot05])
	assertCharacterCreationResultRetained(t, scene, result)
}

func TestCharacterCreationRollbackFailureRefreshesDestination(t *testing.T) {
	rollbackErr := errors.New("rollback failed")
	for _, test := range []struct {
		name      string
		status    player_save.SlotStatus
		revision  string
		configure func(*playerSaveTestRepository)
	}{
		{
			name:   "delete",
			status: player_save.SlotEmpty,
			configure: func(repository *playerSaveTestRepository) {
				repository.deleteErr = rollbackErr
			},
		},
		{
			name:     "restore",
			status:   player_save.SlotReady,
			revision: "reviewed",
			configure: func(repository *playerSaveTestRepository) {
				repository.restoreErr = rollbackErr
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			game, scene, result := newCompletedCharacterCreationFlow(t)
			slot := player_save.Slot08
			repository := newCharacterCreationFlowRepository(slot, test.status, test.revision)
			if test.status == player_save.SlotReady {
				repository.records[slot] = playerSaveTestRecord(slot, test.revision, validCommandTestSnapshot())
			}
			test.configure(repository)
			repository.summariesProvider = func() []player_save.SlotSummary {
				summary := repository.summaries[0]
				if record, occupied := repository.records[slot]; occupied {
					summary.Status = player_save.SlotReady
					summary.Revision = record.Revision
				}

				return []player_save.SlotSummary{summary}
			}
			candidate := &playerSaveTestScene{activatePanic: true}
			openCharacterCreationFlowBrowser(t, game, scene, result, repository, candidate)

			pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyDown)
			pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyEnter)
			confirmCharacterCreationOverwrite(t, game, scene)

			assert.Same(t, scene, game.currentScene)
			assert.Equal(t, 2, repository.listCalls)
			assert.Equal(t, 1, candidate.disposals)
			browser, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveSlotBrowser)
			require.True(t, ok)
			view, selected := browser.GetSelectedView()
			require.True(t, selected)
			assert.True(t, view.HasSave)
			assert.Equal(t, playerSaveStatusLabel(player_save.SlotReady), view.Status)
			assertCharacterCreationResultRetained(t, scene, result)

			pressCharacterCreationFlowKey(t, game, scene, ebiten.KeyEnter)
			assert.IsType(t, &widgets.SaveDescriptionEntryModal{}, scene.dialogStack.PeekTopModalDialog())
			assert.Len(t, repository.saves, 1, "retry requires fresh overwrite confirmation")
		})
	}
}

func newCompletedCharacterCreationFlow(
	t *testing.T,
) (*Game, *CharacterCreationScene, character_creation.Result) {
	t.Helper()
	result := characterCreationFlowResult()
	fixture := newCharacterCreationSceneFixture(t, CharacterCreationCallbacks{
		OnComplete: func(character_creation.Result) {},
		OnCancel:   func() {},
	})
	scene := fixture.scene
	scene.state = characterCreationComplete
	scene.terminal = true
	scene.result = result
	scene.hasResult = true

	game := NewGame(scene)
	game.playthroughIDProvider = func() (string, error) { return testPlaythroughID, nil }
	return game, scene, result
}

func characterCreationFlowResult() character_creation.Result {
	return character_creation.Result{
		Name:              "Avatar",
		Gender:            party_state.Female,
		Bonuses:           character_creation.AttributeBonuses{Strength: 4, Intelligence: 8},
		RandomSeed:        42,
		RandomSourceCalls: 64,
	}
}

func newCharacterCreationFlowRepository(
	slot player_save.SlotID,
	status player_save.SlotStatus,
	revision string,
) *playerSaveTestRepository {
	return &playerSaveTestRepository{
		records: make(map[player_save.SlotID]player_save.Record),
		summaries: []player_save.SlotSummary{{
			Slot: slot, Status: status, Revision: revision,
		}},
	}
}

func openCharacterCreationFlowBrowser(
	t *testing.T,
	game *Game,
	scene *CharacterCreationScene,
	result character_creation.Result,
	repository *playerSaveTestRepository,
	candidate *playerSaveTestScene,
) *widgets.SaveSlotBrowser {
	t.Helper()
	snapshot := validCommandTestSnapshot()
	snapshot.MarkMapUnitsForReconstruction()
	if candidate.snapshot.Version == 0 {
		candidate.snapshot = canonicalPlayerSaveTestSnapshot()
	}
	game.characterCreationSnapshotPreparer = func(string, character_creation.Result) (save_snapshot.SaveSnapshot, error) {
		return snapshot, nil
	}
	game.ConfigurePlayerSaveRepository(repository, func(
		*config.UltimaVConfiguration,
		save_snapshot.SaveSnapshot,
	) (Scene, error) {
		return candidate, nil
	})
	game.completeCharacterCreation(result)
	browser, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveSlotBrowser)
	require.True(t, ok)

	return browser
}

func pressCharacterCreationFlowKey(
	t *testing.T,
	game *Game,
	scene *CharacterCreationScene,
	key ebiten.Key,
) {
	t.Helper()
	scene.keyboard.SimulateKeyPress(key)
	require.NoError(t, scene.Update(game))
}

func confirmCharacterCreationOverwrite(t *testing.T, game *Game, scene *CharacterCreationScene) {
	t.Helper()
	editor, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveDescriptionEntryModal)
	require.True(t, ok)
	editor.Submit()
}

func assertCharacterCreationResultRetained(
	t *testing.T,
	scene *CharacterCreationScene,
	want character_creation.Result,
) {
	t.Helper()
	got, available := scene.Result()
	require.True(t, available)
	assert.Equal(t, want, got)
}
