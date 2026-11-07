package legacy_save

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	savedGameFilename    = "SAVED.GAM"
	savedObjectsFilename = "SAVED.OOL"
	saveDirectoryBatch   = 128
	saveDirectoryLimit   = 1_024
)

// ResolveSavedGamePathContext resolves a save pair with bounded directory work.
func ResolveSavedGamePathContext(ctx context.Context, selectedPath, homeDirectory string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("resolving selected save path: %w", err)
	}
	selectedPath = expandSaveHomePath(selectedPath, homeDirectory)
	absolute, err := filepath.Abs(selectedPath)
	if err != nil {
		return "", fmt.Errorf("resolving selected save path: %w", err)
	}
	info, err := os.Stat(absolute)
	if err == nil && info.IsDir() {
		gamPath, resolveErr := findUniqueSaveSiblingContext(ctx, absolute, savedGameFilename, gamValidationPath)
		if resolveErr != nil {
			return "", resolveErr
		}

		return validateResolvedSavePairContext(ctx, gamPath)
	}
	if err == nil && !info.IsDir() {
		if !strings.EqualFold(filepath.Base(absolute), savedGameFilename) {
			return "", newValidationError([]ValidationIssue{{
				Path: gamValidationPath, Code: "invalid_filename", Offset: unavailableOffset,
				Message: "selected file must be named " + savedGameFilename,
			}})
		}

		gamPath, resolveErr := findUniqueSaveSiblingContext(
			ctx, filepath.Dir(absolute), savedGameFilename, gamValidationPath,
		)
		if resolveErr != nil {
			return "", resolveErr
		}

		return validateResolvedSavePairContext(ctx, gamPath)
	}
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("checking selected save path: %w", err)
	}
	directory := filepath.Dir(absolute)
	if strings.EqualFold(filepath.Base(absolute), savedGameFilename) {
		gamPath, resolveErr := findUniqueSaveSiblingContext(ctx, directory, savedGameFilename, gamValidationPath)
		if resolveErr != nil {
			return "", resolveErr
		}

		return validateResolvedSavePairContext(ctx, gamPath)
	}

	return "", fmt.Errorf("checking selected save path: %w", err)
}

func expandSaveHomePath(selectedPath, homeDirectory string) string {
	selectedPath = strings.TrimSpace(selectedPath)
	if selectedPath == "~" {
		return homeDirectory
	}
	prefix := "~" + string(filepath.Separator)
	if strings.HasPrefix(selectedPath, prefix) {
		return filepath.Join(homeDirectory, strings.TrimPrefix(selectedPath, prefix))
	}

	return selectedPath
}

func validateResolvedSavePairContext(ctx context.Context, gamPath string) (string, error) {
	if _, err := findObjectsSiblingContext(ctx, filepath.Dir(gamPath)); err != nil {
		return "", err
	}

	return gamPath, nil
}

// ReadBundleContext reads a selected save pair with exact allocation bounds and
// observes cancellation between filesystem and decode steps.
func ReadBundleContext(ctx context.Context, selectedSavedGamPath string) (Bundle, error) {
	if err := ctx.Err(); err != nil {
		return Bundle{}, fmt.Errorf("reading original save bundle: %w", err)
	}
	if !strings.EqualFold(filepath.Base(selectedSavedGamPath), savedGameFilename) {
		return Bundle{}, newValidationError([]ValidationIssue{{
			Path:    gamValidationPath,
			Code:    "invalid_filename",
			Offset:  unavailableOffset,
			Message: "selected file must be named " + savedGameFilename,
		}})
	}

	gam, err := readExactSaveFile(ctx, selectedSavedGamPath, savedGameFilename, gamValidationPath, SavedGameSize)
	if err != nil {
		return Bundle{}, err
	}

	if err := ctx.Err(); err != nil {
		return Bundle{}, fmt.Errorf("reading original save bundle: %w", err)
	}
	oolPath, err := findObjectsSiblingContext(ctx, filepath.Dir(selectedSavedGamPath))
	if err != nil {
		return Bundle{}, err
	}
	ool, err := readExactSaveFile(ctx, oolPath, savedObjectsFilename, oolValidationPath, SavedObjectsSize)
	if err != nil {
		return Bundle{}, err
	}
	if err := ctx.Err(); err != nil {
		return Bundle{}, fmt.Errorf("reading original save bundle: %w", err)
	}

	return DecodeBundle(gam, ool)
}

func readExactSaveFile(
	ctx context.Context,
	path, filename, validationPath string,
	expectedSize int,
) (_ []byte, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filename, err)
	}
	if info.Size() != int64(expectedSize) {
		return nil, invalidSaveLength(validationPath, filename, info.Size(), expectedSize)
	}
	//nolint:gosec // Paths are either explicitly selected or exact resolved siblings.
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filename, err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()

	data := make([]byte, expectedSize)
	const chunkSize = 4096
	for offset := 0; offset < expectedSize; {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("reading %s: %w", filename, err)
		}
		end := min(offset+chunkSize, expectedSize)
		read, readErr := io.ReadFull(file, data[offset:end])
		offset += read
		if readErr != nil {
			return nil, invalidSaveLength(validationPath, filename, int64(offset), expectedSize)
		}
	}
	var extra [1]byte
	if read, readErr := file.Read(extra[:]); read != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, fmt.Errorf("checking %s length: %w", filename, readErr)
		}

		return nil, invalidSaveLength(validationPath, filename, int64(expectedSize+read), expectedSize)
	}

	return data, nil
}

func invalidSaveLength(validationPath, filename string, actualSize int64, expectedSize int) error {
	return newValidationError([]ValidationIssue{{
		Path: validationPath, Code: invalidLengthCode, Offset: unavailableOffset,
		Message: fmt.Sprintf("%s must be exactly %d bytes; got %d", filename, expectedSize, actualSize),
	}})
}

func findObjectsSiblingContext(ctx context.Context, directory string) (string, error) {
	return findUniqueSaveSiblingContext(ctx, directory, savedObjectsFilename, oolValidationPath)
}

func findUniqueSaveSiblingContext(
	ctx context.Context,
	directory, filename, validationPath string,
) (_ string, err error) {
	//nolint:gosec // The directory is explicitly selected or comes from a bounded discovery seed.
	handle, err := os.Open(directory)
	if err != nil {
		return "", fmt.Errorf("listing save bundle directory: %w", err)
	}
	defer func() { err = errors.Join(err, handle.Close()) }()

	matches := make([]string, 0, 1)
	entryCount := 0
	for {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("listing save bundle directory: %w", err)
		}
		entries, readErr := handle.ReadDir(saveDirectoryBatch)
		entryCount += len(entries)
		if entryCount > saveDirectoryLimit {
			return "", newValidationError([]ValidationIssue{{
				Path: validationPath, Code: "directory_too_large", Offset: unavailableOffset,
				Message: fmt.Sprintf("selected directory contains more than %d entries", saveDirectoryLimit),
			}})
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(entry.Name(), filename) {
				matches = append(matches, entry.Name())
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("listing save bundle directory: %w", readErr)
		}
	}
	if len(matches) == 0 {
		message := filename + " was not found in the selected directory"
		code := "missing_file"
		if filename == savedObjectsFilename {
			code = "missing_sibling"
			message = savedObjectsFilename + " must be an exact sibling of the selected " + savedGameFilename
		}

		return "", newValidationError([]ValidationIssue{{
			Path:    validationPath,
			Code:    code,
			Offset:  unavailableOffset,
			Message: message,
		}})
	}
	if len(matches) > 1 {
		return "", newValidationError([]ValidationIssue{{
			Path:    validationPath,
			Code:    "ambiguous_sibling",
			Offset:  unavailableOffset,
			Message: "multiple case-insensitive " + filename + " files were found",
		}})
	}

	return filepath.Join(directory, matches[0]), nil
}
