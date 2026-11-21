package originaldata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/files"
	"github.com/bradhannah/UltimaVRedux/internal/originaldata/dungeonarchive"
	"github.com/bradhannah/UltimaVRedux/internal/originaldata/graphicsarchive"
	"github.com/bradhannah/UltimaVRedux/internal/originaldata/introarchive"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const (
	originalDataOvlSize     = 48_464
	originalInitGamSize     = 4_192
	originalInitObjectsSize = 256
	maximumReferenceBytes   = 4 << 20
)

var (
	errDataOvlSize      = errors.New("DATA.OVL has an unexpected size")
	errRequiredFileSize = errors.New("required original-data file has an unexpected size")
)

type problemKind int

const (
	problemMissing problemKind = iota
	problemWrongSize
	problemUnreadable
)

// fileProblemError is a failure a player can cause: a required file that is missing,
// of the wrong size or unreadable. Its text is the technical error; PlainReason
// words it for the player.
type fileProblemError struct {
	name string
	kind problemKind
	err  error
}

func (p *fileProblemError) Error() string { return p.err.Error() }

func (p *fileProblemError) Unwrap() error { return p.err }

func statProblemKind(err error) problemKind {
	if errors.Is(err, fs.ErrNotExist) {
		return problemMissing
	}

	return problemUnreadable
}

// PlainReason says in plain words why an installation check failed, for the
// problems a player can cause: a required file that is missing, of the wrong size
// or unreadable. It returns "" for any other failure.
func PlainReason(err error) string {
	var problem *fileProblemError
	if !errors.As(err, &problem) {
		return ""
	}
	switch problem.kind {
	case problemMissing:
		return problem.name + " is missing from it."
	case problemWrongSize:
		return problem.name + " is not the right size."
	case problemUnreadable:
		return problem.name + " could not be read."
	default:
		panic("unknown installation problem kind")
	}
}

// ValidateInstallation verifies an exact data directory without creating GPU
// resources or active gameplay state.
func ValidateInstallation(ctx context.Context, directory string) error {
	if strings.TrimSpace(directory) == "" {
		return config.ErrDataFilePathUnset
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cancelling installation validation: %w", err)
	}
	dataPath := files.Resolve(directory, files.DataOvl)
	info, err := os.Stat(dataPath)
	if err != nil {
		return &fileProblemError{name: files.DataOvl, kind: statProblemKind(err),
			err: fmt.Errorf("required %s is unavailable: %w", files.DataOvl, err)}
	}
	if info.Size() != originalDataOvlSize {
		return &fileProblemError{name: files.DataOvl, kind: problemWrongSize,
			err: fmt.Errorf("%w: %s has size %d, expected %d", errDataOvlSize, files.DataOvl, info.Size(), originalDataOvlSize)}
	}
	if err := validateRequiredCatalogue(ctx, directory); err != nil {
		return err
	}
	if err := validateRequiredGraphics(ctx, directory); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cancelling installation validation: %w", err)
	}
	configuration, err := config.NewUltimaVConfigurationWithPath(directory)
	if err != nil {
		return fmt.Errorf("loading original data: %w", err)
	}
	if _, err := references.NewGameReferences(configuration); err != nil {
		return fmt.Errorf("validating original game references: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cancelling installation validation: %w", err)
	}

	return nil
}

//nolint:mnd // Canonical original file lengths are part of the installation format.
func validateRequiredCatalogue(ctx context.Context, directory string) error {
	exactFiles := []struct {
		name string
		size int
	}{
		{name: files.InitGam, size: originalInitGamSize},
		{name: files.InitOol, size: originalInitObjectsSize},
		{name: files.BritDat, size: 52_480},
		{name: files.UnderDat, size: 65_536},
		{name: files.CastleDat, size: 16_384},
		{name: files.TowneDat, size: 16_384},
		{name: files.DwellingDat, size: 16_384},
		{name: files.KeepDat, size: 16_384},
		{name: files.DungeonDat, size: 4_096},
		{name: files.CastleNpc, size: 4_608},
		{name: files.TowneNpc, size: 4_608},
		{name: files.DwellingNpc, size: 4_608},
		{name: files.KeepNpc, size: 4_608},
		{name: files.BritCbt, size: 5_632},
		{name: files.DungeonCbt, size: 39_424},
		{name: files.MiscmapsDat, size: 1_871},
		{name: "QUESTION.DAT", size: 7_746},
		{name: "STORY.DAT", size: 11_679},
	}
	for _, required := range exactFiles {
		if err := validateExactFile(ctx, directory, required.name, required.size); err != nil {
			return err
		}
	}
	boundedFiles := []string{
		files.CastleTlk, files.TowneTlk, files.DwellingTlk, files.KeepTlk,
		files.Look2Dat, files.SignsDat, files.ShoppeDat, files.KarmaDAT,
		"END.DAT", "ENDMSG.DAT",
	}
	for _, name := range boundedFiles {
		if err := validateBoundedFile(ctx, directory, name, maximumReferenceBytes); err != nil {
			return err
		}
	}

	return nil
}

func validateRequiredGraphics(ctx context.Context, directory string) error {
	if _, err := introarchive.LoadAndValidateContext(ctx, directory); err != nil {
		return fmt.Errorf("validating original title data: %w", err)
	}
	if _, err := dungeonarchive.LoadAndValidate(ctx, directory); err != nil {
		return fmt.Errorf("validating original dungeon data: %w", err)
	}
	extraGraphics := []string{
		"CREATE.16", "PROPORT.PCS", "TEXT.16",
		"STORY1.16", "STORY2.16", "STORY3.16", "STORY4.16", "STORY5.16", "STORY6.16",
		"END1.16", "END2.16", "ENDSC.16",
	}
	for _, name := range extraGraphics {
		if _, err := graphicsarchive.Load(ctx, directory, name); err != nil {
			return fmt.Errorf("validating required graphics %s: %w", name, err)
		}
	}

	return nil
}

func validateBoundedFile(ctx context.Context, directory, name string, maximumSize int64) (err error) {
	path := files.Resolve(directory, name)
	info, err := os.Stat(path)
	if err != nil {
		return &fileProblemError{name: name, kind: statProblemKind(err),
			err: fmt.Errorf("required %s is unavailable: %w", name, err)}
	}
	if info.Size() <= 0 || info.Size() > maximumSize {
		return &fileProblemError{name: name, kind: problemWrongSize,
			err: fmt.Errorf("%w: %s has unsupported size %d", errRequiredFileSize, name, info.Size())}
	}
	//nolint:gosec // The player selects directory; name comes from the fixed required-file catalogue.
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening required %s: %w", name, err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	reader := newValidationReader(ctx, file, name)
	read, err := io.Copy(io.Discard, io.LimitReader(reader, maximumSize+1))
	if err != nil {
		return fmt.Errorf("reading required %s: %w", name, err)
	}
	if read != info.Size() || read > maximumSize {
		return fmt.Errorf("%w: %s changed while being read", errRequiredFileSize, name)
	}

	return nil
}

func validateExactFile(ctx context.Context, directory, name string, expectedSize int) (err error) {
	path := files.Resolve(directory, name)
	info, err := os.Stat(path)
	if err != nil {
		return &fileProblemError{name: name, kind: statProblemKind(err),
			err: fmt.Errorf("required %s is unavailable: %w", name, err)}
	}
	if info.Size() != int64(expectedSize) {
		return &fileProblemError{name: name, kind: problemWrongSize,
			err: fmt.Errorf("%w: %s has size %d, expected %d", errRequiredFileSize, name, info.Size(), expectedSize)}
	}
	//nolint:gosec // The player selects directory; name is a fixed required filename.
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening required %s: %w", name, err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	reader := newValidationReader(ctx, file, name)
	read, err := io.Copy(io.Discard, io.LimitReader(reader, int64(expectedSize)+1))
	if err != nil {
		return fmt.Errorf("reading required %s: %w", name, err)
	}
	if read != int64(expectedSize) {
		return fmt.Errorf(
			"%w: %s changed while being read: got %d bytes, expected %d",
			errRequiredFileSize, name, read, expectedSize,
		)
	}

	return nil
}

func newValidationReader(ctx context.Context, file io.Reader, name string) validationReaderFunc {
	return func(buffer []byte) (int, error) {
		if err := ctx.Err(); err != nil {
			return 0, fmt.Errorf("reading required %s: %w", name, err)
		}
		read, readErr := file.Read(buffer)
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return read, io.EOF
			}

			return read, fmt.Errorf("reading required %s: %w", name, readErr)
		}

		return read, nil
	}
}

type validationReaderFunc func([]byte) (int, error)

func (read validationReaderFunc) Read(buffer []byte) (int, error) {
	return read(buffer)
}
