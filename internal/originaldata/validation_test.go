package originaldata

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/files"
	testoriginaldata "github.com/bradhannah/UltimaVRedux/test/originaldata"
)

func TestValidateInstallationRequiresExactInitialTemplates(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, files.DataOvl), make([]byte, originalDataOvlSize), 0o600); err != nil {
		t.Fatal(err)
	}

	err := ValidateInstallation(context.Background(), directory)
	if err == nil || !strings.Contains(err.Error(), files.InitGam) {
		t.Fatalf("ValidateInstallation() error = %v, want missing %s", err, files.InitGam)
	}
	if got, want := PlainReason(err), files.InitGam+" is missing from it."; got != want {
		t.Fatalf("PlainReason() = %q, want %q", got, want)
	}

	if err := os.WriteFile(filepath.Join(directory, files.InitGam), make([]byte, originalInitGamSize), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, files.InitOol), make([]byte, originalInitObjectsSize-1), 0o600); err != nil {
		t.Fatal(err)
	}
	err = ValidateInstallation(context.Background(), directory)
	if err == nil || !strings.Contains(err.Error(), files.InitOol) {
		t.Fatalf("ValidateInstallation() error = %v, want invalid %s", err, files.InitOol)
	}
	if got, want := PlainReason(err), files.InitOol+" is not the right size."; got != want {
		t.Fatalf("PlainReason() = %q, want %q", got, want)
	}
}

func TestValidateInstallationRequiresLazyGameplayCatalogue(t *testing.T) {
	directory := t.TempDir()
	fixtures := map[string]int{
		files.DataOvl: originalDataOvlSize,
		files.InitGam: originalInitGamSize,
		files.InitOol: originalInitObjectsSize,
	}
	for name, size := range fixtures {
		if err := os.WriteFile(filepath.Join(directory, name), make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	err := ValidateInstallation(context.Background(), directory)
	if err == nil || !strings.Contains(err.Error(), files.BritDat) {
		t.Fatalf("ValidateInstallation() error = %v, want missing %s", err, files.BritDat)
	}
}

func TestValidateInstallationWithAvailableLocalOriginalData(t *testing.T) {
	directory := testoriginaldata.Directory(t, files.DataOvl)
	if err := ValidateInstallation(context.Background(), directory); err != nil {
		t.Fatalf("ValidateInstallation() error = %v", err)
	}
}
