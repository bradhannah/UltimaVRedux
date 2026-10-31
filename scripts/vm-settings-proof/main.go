// Command vm-settings-proof drives the opt-in application fixture through real
// Ebitengine keyboard input. Build both binaries with make vm-settings-proof.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/exp/vmhost"

	"github.com/bradhannah/UltimaVRedux/internal/logging"
)

const (
	proofTimeout = 30 * time.Second
	idleTimeout  = 10 * time.Second
	proofWidth   = 1280
	proofHeight  = 960
)

var (
	errDataRequired    = errors.New("specify ULTIMA5_DATA_PATH or -data")
	errIncompleteProof = errors.New("VM settings proof ended before its final frame")
)

func main() {
	logging.SetGlobalEnabled(true)
	logging.SetCategoryEnabled(logging.CategorySystem, true)
	logging.SetStdoutEnabled(true)
	guest := flag.String("guest", "bin/settings-vm-guest", "prebuilt settings-proof guest binary")
	data := flag.String("data", os.Getenv("ULTIMA5_DATA_PATH"), "original Ultima V data directory (required)")
	flag.Parse()
	if err := run(*guest, *data); err != nil {
		// Application-level fatal: a failed proof must exit nonzero after cleanup.
		log.Fatal(err)
	}
	logging.Infof(logging.CategorySystem, "VM settings proof passed; guest reaped and temporary home removed")
}

func run(guestPath, dataPath string) error {
	if dataPath == "" {
		return errDataRequired
	}
	guestPath, err := filepath.Abs(guestPath)
	if err != nil {
		return fmt.Errorf("resolve guest binary: %w", err)
	}
	dataPath, err = filepath.Abs(dataPath)
	if err != nil {
		return fmt.Errorf("resolve original data: %w", err)
	}
	home, err := os.MkdirTemp("", "redux-vm-")
	if err != nil {
		return fmt.Errorf("create isolated home: %w", err)
	}
	defer func() { reportCleanup(os.RemoveAll(home)) }()
	if err := prepareHome(home, dataPath); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), proofTimeout)
	defer cancel()

	return runGuest(ctx, guestPath, home)
}

func prepareHome(home, dataPath string) error {
	const privateDirectory = 0o700
	const privateFile = 0o600
	directory := filepath.Join(home, ".ultima_v_redux")
	if err := os.Mkdir(directory, privateDirectory); err != nil {
		return fmt.Errorf("create isolated configuration directory: %w", err)
	}
	// JSON is valid YAML; marshaling preserves paths containing quotes/backslashes.
	contents, err := json.Marshal(map[string]string{"DataFilePath": dataPath})
	if err != nil {
		return fmt.Errorf("encode isolated configuration: %w", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "config.yaml"), contents, privateFile); err != nil {
		return fmt.Errorf("write isolated configuration: %w", err)
	}

	return nil
}

//nolint:funlen // Keep subprocess, socket and cleanup ownership in one bounded lifecycle.
func runGuest(ctx context.Context, guestPath, home string) error {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen for VM guest: %w", err)
	}
	defer func() { reportCleanup(listener.Close()) }()
	// Accept must also be bounded when the guest fails before connecting.
	tcpListener, ok := listener.(*net.TCPListener)
	if !ok {
		panic("TCP listener has unexpected type")
	}
	if err := tcpListener.SetDeadline(time.Now().Add(idleTimeout)); err != nil {
		return fmt.Errorf("bound VM accept: %w", err)
	}
	endpoint, err := vmhost.EndpointURLFromAddr(listener.Addr())
	if err != nil {
		return fmt.Errorf("resolve VM endpoint: %w", err)
	}
	command := exec.CommandContext(ctx, guestPath)
	command.Dir = home
	command.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home,
		"XDG_CONFIG_HOME="+home, "XDG_DATA_HOME="+home, "EBITENGINE_VM_ENDPOINT="+endpoint,
		"ULTIMA5_SETTINGS_PROOF_HOST=1")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Start(); err != nil {
		return fmt.Errorf("start VM guest: %w", err)
	}
	waited := false
	defer func() {
		if !waited {
			reportCleanup(command.Process.Kill())
			// A killed child returns ExitError; Wait still reaps it.
			var exitError *exec.ExitError
			if err := command.Wait(); err != nil && !errors.As(err, &exitError) {
				reportCleanup(err)
			}
		}
	}()
	connection, err := listener.Accept()
	if err != nil {
		return fmt.Errorf("accept VM guest: %w", err)
	}
	defer func() { reportCleanup(connection.Close()) }()
	if err := listener.Close(); err != nil {
		return fmt.Errorf("retire VM listener: %w", err)
	}
	deadline, _ := ctx.Deadline()
	if err := connection.SetDeadline(deadline); err != nil {
		return fmt.Errorf("bound VM session: %w", err)
	}
	if err := driveGuest(connection); err != nil {
		return err
	}
	err = command.Wait()
	waited = true
	if err != nil {
		return fmt.Errorf("VM guest assertions failed: %w", err)
	}

	return nil
}

func driveGuest(connection net.Conn) error {
	options := new(vmhost.NewGuestSessionOptions)
	options.IdleTimeout = idleTimeout
	guest, err := vmhost.NewGuestSession(connection, options)
	if err != nil {
		return fmt.Errorf("handshake VM guest: %w", err)
	}
	driver := &settingsDriver{guest: guest, closeErr: nil, complete: false}
	ebiten.SetWindowVisible(false)
	ebiten.SetWindowSize(proofWidth, proofHeight)
	if err := ebiten.RunGame(driver); err != nil {
		return fmt.Errorf("drive settings proof: %w", err)
	}
	if driver.closeErr != nil {
		return fmt.Errorf("close VM session: %w", driver.closeErr)
	}
	if !driver.complete {
		return errIncompleteProof
	}

	return nil
}

type settingsDriver struct {
	guest    *vmhost.GuestSession
	closeErr error
	complete bool
}

func (driver *settingsDriver) Update() error {
	// Session cleanup must run on a host frame, including assertion failures.
	defer func() { driver.closeErr = driver.guest.Close() }()
	screen := ebiten.NewImage(proofWidth, proofHeight)
	defer screen.Deallocate()
	if err := driver.guest.SetOutsideScreen(screen); err != nil {
		return fmt.Errorf("size VM guest: %w", err)
	}
	driver.guest.AdvanceTicks(1)
	if !driver.guest.WaitTicks() {
		return fmt.Errorf("initialize VM guest: %w", driver.guest.Err())
	}
	// Title skip; Load -> New Game -> Configure; open its sub-menu; Display ->
	// Gameplay; toggle; cancel; reopen; Display -> Gameplay. Every press and
	// release gets a completed tick, so asynchronous injection cannot coalesce.
	keys := []ebiten.Key{
		ebiten.KeyEnter,
		ebiten.KeyDown, ebiten.KeyDown,
		ebiten.KeyEnter, ebiten.KeyEnter, ebiten.KeyTab, ebiten.KeySpace,
		ebiten.KeyEscape, ebiten.KeyEnter, ebiten.KeyTab,
	}
	for _, key := range keys {
		driver.guest.PressKey(key)
		driver.guest.AdvanceTicks(1)
		if !driver.guest.WaitTicks() {
			return fmt.Errorf("press %s: %w", key, driver.guest.Err())
		}
		driver.guest.ReleaseKey(key)
		driver.guest.AdvanceTicks(1)
		driver.guest.AdvanceFrame()
		if !driver.guest.WaitFrame() || !driver.guest.CompositeFrame() {
			return fmt.Errorf("render after %s: %w", key, errors.Join(errIncompleteProof, driver.guest.Err()))
		}
	}
	driver.complete = true

	return ebiten.Termination
}

func (*settingsDriver) Draw(*ebiten.Image) {}

func (*settingsDriver) Layout(_, _ int) (int, int) { return proofWidth, proofHeight }

func reportCleanup(err error) {
	if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, os.ErrProcessDone) {
		logging.Warnf(logging.CategorySystem, "VM proof cleanup: %v", err)
	}
}
