package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	//nolint:gosec // Profiling handlers are served only by the explicit opt-in loopback profiler below.
	_ "net/http/pprof"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/debugging/workbench"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/originaldata"
	"github.com/bradhannah/UltimaVRedux/internal/profiling"
	"github.com/bradhannah/UltimaVRedux/internal/text"
)

// Layout sets the game's screen size.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	width, height := g.LayoutF(float64(outsideWidth), float64(outsideHeight))

	return int(width), int(height)
}

// LayoutF preserves fractional window units before converting to output density.
func (g *Game) LayoutF(outsideWidth, outsideHeight float64) (float64, float64) {
	width, height := display.GetManager().LayoutSize(outsideWidth, outsideHeight)

	return float64(width), float64(height)
}

// DrawFinalScreen explicitly clears aspect-fit bars and assigns odd-pixel
// remainders to the right or bottom edge.
func (g *Game) DrawFinalScreen(screen ebiten.FinalScreen, offscreen *ebiten.Image, _ ebiten.GeoM) {
	screen.Fill(color.Black)
	display.GetManager().PublishOutputSize(screen.Bounds().Dx(), screen.Bounds().Dy())
	geometry := finalScreenGeometry(screen.Bounds(), offscreen.Bounds())
	ebiten.DefaultDrawFinalScreen(screen, offscreen, geometry)
}

func finalScreenGeometry(destination, source image.Rectangle) ebiten.GeoM {
	const centerDivisor = 2
	scale := min(
		float64(destination.Dx())/float64(source.Dx()),
		float64(destination.Dy())/float64(source.Dy()),
	)
	translationX := float64(destination.Min.X) +
		math.Floor((float64(destination.Dx())-float64(source.Dx())*scale)/centerDivisor)
	translationY := float64(destination.Min.Y) +
		math.Floor((float64(destination.Dy())-float64(source.Dy())*scale)/centerDivisor)

	var geometry ebiten.GeoM
	geometry.Scale(scale, scale)
	geometry.Translate(
		translationX-float64(source.Min.X)*scale,
		translationY-float64(source.Min.Y)*scale,
	)

	return geometry
}

func main() {
	configureCrashOutput()
	if err := runApplication(); err != nil {
		// Application-level fatal: launch/game failure is reported only after deferred cleanup has run.
		log.Fatal(err)
	}
}

//nolint:cyclop,funlen,gocognit,gocyclo,maintidx // Ordered launch/recovery and diagnostic cleanup.
func runApplication() error {
	var snapshotOptions snapshotStartupOptions
	addSnapshotFlags(&snapshotOptions)
	var playerSaveOptions playerSaveStartupOptions
	addPlayerSaveFlags(&playerSaveOptions)

	// Command-line flags for profiling
	var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
	var memprofile = flag.String("memprofile", "", "write memory profile to file")
	var webprofile = flag.Bool("webprofile", false, "enable web-based profiling on :6060")
	var noSound = flag.Bool("no-sound", false, "disable music, sound effects, and ambient audio for this session")
	var debugWorkbench = flag.Bool("debug-workbench", false, "enable browser debug workbench")
	var showVersion = flag.Bool("version", false, "print the game version and exit")
	var debugWorkbenchPort = flag.Int(
		"debug-workbench-port",
		workbench.DefaultPort,
		"browser debug workbench loopback port",
	)
	flag.Parse()
	if *showVersion {
		if _, err := fmt.Fprintln(os.Stdout, "Ultima V Redux "+runtimeGameVersion()); err != nil {
			return fmt.Errorf("printing version: %w", err)
		}

		return nil
	}
	defer text.CloseRenderer()

	// Start CPU profiling if requested
	if *cpuprofile != "" {
		profileFile, err := os.Create(*cpuprofile)
		if err != nil {
			// System failure: Cannot create profiling file - fatal for profiling mode
			return fmt.Errorf("creating CPU profile: %w", err)
		}
		defer func() {
			if closeErr := profileFile.Close(); closeErr != nil {
				logging.Warnf(logging.CategorySystem, "Closing CPU profile: %v", closeErr)
			}
		}()
		if err := pprof.StartCPUProfile(profileFile); err != nil {
			// System failure: Cannot initialize profiling - fatal for profiling mode
			return fmt.Errorf("starting CPU profile: %w", err)
		}
		defer pprof.StopCPUProfile()
		logging.Infof(logging.CategorySystem, "CPU profiling enabled, writing to %s", *cpuprofile)
	}

	// Enable web-based profiling if requested
	if *webprofile {
		metrics := profiling.Enable()
		http.HandleFunc("/debug/performance", func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(writer).Encode(metrics.Snapshot()); err != nil {
				logging.Warnf(logging.CategorySystem, "Encoding profiling metrics: %v", err)
			}
		})
		go func() {
			const profilerHeaderTimeout = 10 * time.Second
			logging.Infof(logging.CategorySystem, "Profiling server: http://localhost:6060/debug/pprof")
			//nolint:exhaustruct // Loopback diagnostic server uses standard defaults except header/idle deadlines.
			server := &http.Server{Addr: "localhost:6060", ReadHeaderTimeout: profilerHeaderTimeout, IdleTimeout: time.Minute}
			logging.Debugf(logging.CategorySystem, "pprof server result: %v", server.ListenAndServe())
		}()
	}

	// Set the window title and size
	ebiten.SetWindowTitle("Ultima V Redux")
	applyApplicationIcon()
	ebiten.SetRunnableOnUnfocused(true)
	// Gameplay offers to save before a window close; Game.Update decides.
	ebiten.SetWindowClosingHandled(true)

	startup := config.LoadStartupConfiguration()
	gameConfig := startup.Configuration
	if gameConfig == nil {
		// Application-level fatal: even the bundled recovery UI requires a preference model.
		return fmt.Errorf("loading startup configuration: %w", startup.ConfigurationError)
	}
	applySavedLoggingConfiguration(gameConfig)
	gameConfig.SoundDisabled = *noSound
	windowWidth := gameConfig.DisplayConfiguration.WindowWidth
	windowHeight := gameConfig.DisplayConfiguration.WindowHeight
	displayManager := display.GetManager()
	windowWidth, windowHeight = displayManager.FitWindowSize(windowWidth, windowHeight)
	gameConfig.DisplayConfiguration.WindowWidth = windowWidth
	gameConfig.DisplayConfiguration.WindowHeight = windowHeight
	if err := displayManager.InitializeMode(
		gameConfig.DisplayConfiguration.Mode,
		windowWidth,
		windowHeight,
	); err != nil {
		// Application-level fatal: validated display configuration is required before the game loop starts.
		return fmt.Errorf("applying display configuration: %w", err)
	}
	var initialScene Scene
	var err error
	startupError := errors.Join(startup.ConfigurationError, startup.OriginalDataError)
	if startupError == nil {
		startupError = originaldata.ValidateInstallation(
			context.Background(),
			gameConfig.SavedConfigData.DataFilePath,
		)
	}
	if startupError == nil {
		playable, loadErr := gameConfig.PlayableWithDataFilePath(gameConfig.SavedConfigData.DataFilePath)
		if loadErr != nil {
			startupError = loadErr
		} else {
			playable.SoundDisabled = *noSound
			gameConfig = playable
		}
	}
	if startupError == nil {
		initialScene, err = CreateIntroMenuScene(gameConfig)
		if err != nil {
			startupError = err
		}
	}
	if startupError != nil {
		host, hostErr := originaldata.CurrentHostProfile()
		if hostErr != nil {
			// Application-level fatal: setup cannot offer deterministic host paths without a home or executable location.
			return fmt.Errorf("resolving setup host: %w", hostErr)
		}
		initialScene = newDataSetupScene(
			gameConfig,
			startup.ConfigurationError,
			startup.OriginalDataError,
			host,
			dataSetupFirstRun,
			*noSound,
			func(game *Game, playable *config.UltimaVConfiguration) error {
				if err := game.ReplaceScene(func() (Scene, error) {
					return CreateIntroMenuScene(playable)
				}); err != nil {
					return err
				}

				return configureRuntimeServices(game, playerSaveOptions, snapshotOptions)
			},
			nil,
		)
	}
	game := NewGame(initialScene)
	defer game.disposeActiveScene()
	if _, setup := initialScene.(*dataSetupScene); !setup {
		if err := configureRuntimeServices(game, playerSaveOptions, snapshotOptions); err != nil {
			// Application-level fatal: runtime services cannot be configured.
			return fmt.Errorf("configuring runtime services: %w", err)
		}
	}
	var debugServer *runningDebugWorkbench
	if *debugWorkbench {
		var err error
		debugServer, err = startDebugWorkbench(game, *debugWorkbenchPort)
		if err != nil {
			logging.Errorf(logging.CategorySystem, "Debug workbench unavailable: %v", err)
		}
	}

	// Run the game
	runErr := ebiten.RunGame(game)
	if runErr == nil {
		windowWidth, windowHeight = displayManager.PersistableWindowSize()
		activeConfiguration := game.currentScene.GetUltimaConfiguration()
		activeConfiguration.DisplayConfiguration.WindowWidth = windowWidth
		activeConfiguration.DisplayConfiguration.WindowHeight = windowHeight
		if err := activeConfiguration.UpdateSaveFile(); err != nil {
			logging.Warnf(logging.CategoryConfig, "Failed to save final window size: %v", err)
		} else {
			displayManager.CommitNormalWindowSize(windowWidth, windowHeight)
		}
	}
	if err := shutdownDebugWorkbench(debugServer); err != nil {
		logging.Warnf(logging.CategorySystem, "Debug workbench shutdown failed: %v", err)
	}
	game.disposeActiveScene()

	// Write memory profile if requested (after game ends)
	if *memprofile != "" {
		profileFile, err := os.Create(*memprofile)
		if err != nil {
			// System failure: Cannot create memory profile file - fatal for profiling mode
			return fmt.Errorf("creating memory profile: %w", err)
		}
		runtime.GC() // get up-to-date statistics
		writeErr := pprof.WriteHeapProfile(profileFile)
		closeErr := profileFile.Close()
		if writeErr != nil {
			// System failure: Cannot write memory profile - fatal for profiling mode
			return fmt.Errorf("writing memory profile: %w", writeErr)
		}
		if closeErr != nil {
			// System failure: An incomplete requested profile cannot be reported as successful.
			return fmt.Errorf("closing memory profile: %w", closeErr)
		}
		logging.Infof(logging.CategorySystem, "Memory profile written to %s", *memprofile)
	}

	if runErr != nil {
		// Critical system corruption: game engine failure prevents operation
		return fmt.Errorf("running game: %w", runErr)
	}

	return nil
}

func configureRuntimeServices(
	game *Game,
	playerSaveOptions playerSaveStartupOptions,
	snapshotOptions snapshotStartupOptions,
) error {
	if err := configurePlayerSaveRepository(game, playerSaveOptions); err != nil {
		logging.Errorf(logging.CategorySystem, "Player save repository unavailable: %v", err)
	}
	if err := configureSnapshotRepository(game, snapshotOptions); err != nil {
		logging.Errorf(logging.CategorySystem, "Save snapshot repository unavailable: %v", err)
	}

	return nil
}
