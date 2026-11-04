package extractor

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"gopkg.in/yaml.v3"
)

const (
	// ManifestFormat identifies the extraction manifest grammar.
	ManifestFormat = "ultima5redux.conversation-extraction/v1"
	// ExtractorVersion identifies the deterministic original-data conversion rules.
	ExtractorVersion = "original-tlk/v2"
	manifestFilename = "manifest.yaml"
	schemaFilename   = "conversation-v1.schema.json"
	sha256HexLength  = 64
)

// SourceDigest identifies an original input file and its SHA-256 digest.
type SourceDigest struct {
	File   string `yaml:"file"`
	SHA256 string `yaml:"sha256"`
}

// Manifest records the exact inputs and outputs of an extraction.
//
//nolint:tagliatelle // The versioned manifest contract intentionally uses canonical snake_case YAML.
type Manifest struct {
	FormatVersion    string         `yaml:"format_version"`
	SchemaVersion    string         `yaml:"schema_version"`
	ExtractorVersion string         `yaml:"extractor_version"`
	SchemaFile       string         `yaml:"schema_file"`
	AuditFile        string         `yaml:"audit_file"`
	AuditSHA256      string         `yaml:"audit_sha256"`
	AuditEntryCount  int            `yaml:"audit_entry_count"`
	Sources          []SourceDigest `yaml:"sources"`
	ExpectedFiles    []string       `yaml:"expected_files"`
	ActualFiles      []string       `yaml:"actual_files"`
	ContentSHA256    string         `yaml:"content_sha256"`
}

// LoadManifest strictly loads and validates an extraction manifest.
func LoadManifest(filename string) (*Manifest, error) {
	return loadStrictYAML(filename, "manifest", func(manifest *Manifest) error {
		return manifest.Validate()
	})
}

//nolint:err113 // Strict document errors include the selected file and document kind.
func loadStrictYAML[T any](filename, kind string, validate func(*T) error) (*T, error) {
	file, err := os.Open(filename) //nolint:gosec // The caller explicitly selects the extraction document.
	if err != nil {
		return nil, fmt.Errorf("opening conversation extraction %s %q: %w", kind, filename, err)
	}
	defer file.Close() //nolint:errcheck // Read errors are authoritative for this read-only file.

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	document := new(T)
	if err := decoder.Decode(document); err != nil {
		return nil, fmt.Errorf("decoding conversation extraction %s %q: %w", kind, filename, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf(
			"decoding conversation extraction %s %q: multiple YAML documents are not allowed",
			kind,
			filename,
		)
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf(
			"checking conversation extraction %s %q for extra documents: %w",
			kind,
			filename,
			err,
		)
	}
	if err := validate(document); err != nil {
		return nil, fmt.Errorf("validating conversation extraction %s %q: %w", kind, filename, err)
	}

	return document, nil
}

// Validate checks the closed v1 extraction-manifest contract.
//
//nolint:cyclop,err113 // Each versioned manifest invariant has a precise diagnostic.
func (manifest *Manifest) Validate() error {
	if manifest == nil {
		return errors.New("manifest is nil")
	}
	if manifest.FormatVersion != ManifestFormat {
		return fmt.Errorf("format_version must equal %q", ManifestFormat)
	}
	if manifest.SchemaVersion != semantic.SchemaVersion {
		return fmt.Errorf("schema_version must equal %q", semantic.SchemaVersion)
	}
	if manifest.ExtractorVersion != ExtractorVersion {
		return fmt.Errorf("extractor_version must equal %q", ExtractorVersion)
	}
	if manifest.SchemaFile != schemaFilename {
		return fmt.Errorf("schema_file must equal %q", schemaFilename)
	}
	if manifest.AuditFile != auditFilename {
		return fmt.Errorf("audit_file must equal %q", auditFilename)
	}
	if !isLowerHexDigest(manifest.AuditSHA256) {
		return errors.New("audit_sha256 must be a lowercase SHA-256 digest")
	}
	if manifest.AuditEntryCount < 0 {
		return errors.New("audit_entry_count must not be negative")
	}
	wantSources := sourceFilenames()
	if len(manifest.Sources) != len(wantSources) {
		return fmt.Errorf("sources has %d entries, want %d", len(manifest.Sources), len(wantSources))
	}
	for index, source := range manifest.Sources {
		if source.File != wantSources[index] {
			return fmt.Errorf("sources[%d].file is %q, want %q", index, source.File, wantSources[index])
		}
		if !isLowerHexDigest(source.SHA256) {
			return fmt.Errorf("sources[%d].sha256 must be a lowercase SHA-256 digest", index)
		}
	}
	if err := validateManifestFiles("expected_files", manifest.ExpectedFiles); err != nil {
		return err
	}
	if err := validateManifestFiles("actual_files", manifest.ActualFiles); err != nil {
		return err
	}
	if !slices.Equal(manifest.ExpectedFiles, expectedConversationFiles()) {
		return errors.New("expected_files must exactly match the v1 extractor file list in order")
	}
	if !slices.Equal(manifest.ExpectedFiles, manifest.ActualFiles) {
		return errors.New("actual_files must exactly match expected_files in order")
	}
	if !isLowerHexDigest(manifest.ContentSHA256) {
		return errors.New("content_sha256 must be a lowercase SHA-256 digest")
	}

	return nil
}

//nolint:err113 // Dynamic field/index context is required for strict manifest diagnostics.
func validateManifestFiles(field string, files []string) error {
	if files == nil {
		return fmt.Errorf("%s is required", field)
	}
	seen := make(map[string]struct{}, len(files))
	for index, name := range files {
		// Entries are portable slash paths: clean them with package path, because
		// filepath.Clean would rewrite them with backslashes on Windows.
		if name == "" || filepath.IsAbs(name) || path.IsAbs(name) || path.Clean(name) != name ||
			name == ".." || strings.HasPrefix(name, "../") || strings.ContainsRune(name, '\\') {
			return fmt.Errorf("%s[%d] is not a clean relative slash path", field, index)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("%s[%d] duplicates %q", field, index, name)
		}
		seen[name] = struct{}{}
	}

	return nil
}

func isLowerHexDigest(value string) bool {
	if len(value) != sha256HexLength {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}

	return true
}
