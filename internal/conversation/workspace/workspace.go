// Package workspace manages private original-conversation editing trees.
package workspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/conversation/extractor"
	conversationpatch "github.com/bradhannah/UltimaVRedux/internal/conversation/patch"
	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/internal/repositoryfs"
	"github.com/pmezard/go-difflib/difflib"
)

const (
	baseDirectory       = "base"
	editedDirectory     = "edited"
	manifestFilename    = "manifest.yaml"
	patchScaffold       = ".gitkeep"
	patchPackageFile    = "embed.go"
	privateDirectory    = 0o700
	privateFile         = 0o600
	protectedDirectory  = 0o500
	protectedFile       = 0o400
	projectDirectory    = 0o755
	projectFile         = 0o644
	contentDigestLength = 8
)

const patchPackageSource = `// Package conversationpatchdata exposes tracked conversation patches to the runtime.
package conversationpatchdata

import (
	"embed"
	"io/fs"
)

// Files contains the cumulative semantic patches. Non-patch source and scaffold
// files are ignored by the catalog loader.
//
//go:embed all:*
var Files embed.FS

var _ fs.FS = Files
`

// Options selects the original data, private workspace, and tracked patch tree.
type Options struct {
	DataDir   string
	Workspace string
	PatchDir  string
}

// InitResult summarizes a newly published private workspace.
type InitResult struct {
	ConversationCount int
	AppliedPatches    int
	Workspace         string
}

// DiffResult summarizes workspace differences written by Diff.
type DiffResult struct {
	ChangedConversations int
	DialogueLinesTouched int
}

// PatchResult summarizes an atomic cumulative patch regeneration.
type PatchResult struct {
	Added                 int
	Updated               int
	Removed               int
	Unchanged             int
	DialogueLinesTouched  int
	DialogueConversations int
	DialogueWarnings      map[string]int
}

// VerifyResult summarizes successful reproduction of edited conversations.
type VerifyResult struct {
	ConversationCount int
	AppliedPatches    int
}

type catalog struct {
	options  Options
	manifest *extractor.Manifest
	base     map[string][]byte
	edited   map[string][]byte
	parsed   map[string]*semantic.Conversation
}

type extractFunc func(extractor.Options) (*extractor.Result, error)

// Init extracts an immutable base, applies project patches to an editable copy,
// validates the complete tree, and publishes it atomically.
func Init(options Options) (*InitResult, error) {
	return initWorkspace(options, extractor.Extract)
}

//nolint:cyclop,err113,funlen,nonamedreturns // Deferred staging cleanup preserves failures of this atomic transaction.
func initWorkspace(options Options, extract extractFunc) (result *InitResult, err error) {
	if err := validateOptions(options, true, true); err != nil {
		return nil, err
	}
	if extract == nil {
		return nil, errors.New("conversation extractor is nil")
	}
	if err := requireAbsent(options.Workspace); err != nil {
		return nil, err
	}
	parent := filepath.Dir(options.Workspace)
	if err := os.MkdirAll(parent, privateDirectory); err != nil {
		return nil, fmt.Errorf("creating workspace parent %q: %w", parent, err)
	}
	if err := ensurePrivateDirectory(parent); err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(options.Workspace)+".staging-*")
	if err != nil {
		return nil, fmt.Errorf("creating conversation workspace staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			err = errors.Join(err, makeTreeRemovable(filepath.Join(stage, baseDirectory)))
			if cleanupErr := os.RemoveAll(stage); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("removing workspace staging directory %q: %w", stage, cleanupErr))
			}
		}
	}()
	if err := os.Chmod(stage, privateDirectory); err != nil {
		return nil, fmt.Errorf("setting private workspace staging permissions: %w", err)
	}
	basePath := filepath.Join(stage, baseDirectory)
	if _, err := extract(extractor.Options{DataDir: options.DataDir, Output: basePath, Force: false}); err != nil {
		return nil, fmt.Errorf("extracting conversation workspace base: %w", err)
	}
	if err := protectBase(basePath); err != nil {
		return nil, err
	}
	stagedOptions := options
	stagedOptions.Workspace = stage
	baseCatalog, err := loadBase(stagedOptions)
	if err != nil {
		return nil, err
	}
	patches, err := loadPatches(options.PatchDir, baseCatalog.manifest.ExpectedFiles)
	if err != nil {
		return nil, err
	}
	if err := createEditedTree(stage, baseCatalog, patches); err != nil {
		return nil, err
	}
	if _, err := loadCatalog(stagedOptions); err != nil {
		return nil, fmt.Errorf("validating staged conversation workspace: %w", err)
	}
	if err := syncTree(stage); err != nil {
		return nil, err
	}
	if err := os.Rename(stage, options.Workspace); err != nil {
		return nil, fmt.Errorf("publishing conversation workspace %q: %w", options.Workspace, err)
	}
	published = true
	if err := syncDirectory(parent); err != nil {
		return nil, err
	}

	return &InitResult{
		ConversationCount: len(baseCatalog.manifest.ExpectedFiles),
		AppliedPatches:    len(patches),
		Workspace:         options.Workspace,
	}, nil
}

// Check validates base extraction integrity and every edited conversation.
func Check(options Options) error {
	if err := validateOptions(options, false, false); err != nil {
		return err
	}
	_, err := loadCatalog(options)
	if err != nil {
		return err
	}

	return nil
}

// Diff writes standard base-to-edited unified diffs in manifest order.
//
//nolint:err113 // A nil writer is a programming-contract violation with one direct diagnostic.
func Diff(options Options, writer io.Writer) (*DiffResult, error) {
	if writer == nil {
		return nil, errors.New("conversation diff writer is nil")
	}
	if err := validateOptions(options, false, false); err != nil {
		return nil, err
	}
	catalog, err := loadCatalog(options)
	if err != nil {
		return nil, err
	}
	result := &DiffResult{ChangedConversations: 0, DialogueLinesTouched: 0}
	for _, target := range catalog.manifest.ExpectedFiles {
		base := catalog.base[target]
		edited := catalog.edited[target]
		if bytes.Equal(base, edited) {
			continue
		}
		generated, err := conversationpatch.Generate(
			target,
			catalog.manifest.SchemaVersion,
			catalog.manifest.ExtractorVersion,
			base,
			edited,
		)
		if err != nil {
			return nil, fmt.Errorf("generating workspace diff for %q: %w", target, err)
		}
		unified, err := stripPatchMetadata(generated)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write(unified); err != nil {
			return nil, fmt.Errorf("writing workspace diff for %q: %w", target, err)
		}
		result.ChangedConversations++
		result.DialogueLinesTouched += dialogueLinesTouched(catalog.parsed[target], mustLoadConversation(target, edited))
	}

	return result, nil
}

// Patches atomically regenerates the complete cumulative project patch tree.
//
//nolint:err113,funlen // Manifest-order generation keeps accounting adjacent to each validated patch.
func Patches(options Options) (*PatchResult, error) {
	if err := validateOptions(options, false, true); err != nil {
		return nil, err
	}
	catalog, err := loadCatalog(options)
	if err != nil {
		return nil, err
	}
	existing, err := loadPatches(options.PatchDir, catalog.manifest.ExpectedFiles)
	if err != nil {
		return nil, err
	}
	generated := make(map[string][]byte)
	result := &PatchResult{
		Added: 0, Updated: 0, Removed: 0, Unchanged: 0,
		DialogueLinesTouched: 0, DialogueConversations: 0,
		DialogueWarnings: make(map[string]int),
	}
	for _, target := range catalog.manifest.ExpectedFiles {
		base := catalog.base[target]
		edited := catalog.edited[target]
		oldPatch, existed := existing[target]
		if bytes.Equal(base, edited) {
			if existed {
				result.Removed++
			}

			continue
		}
		patchData, err := conversationpatch.Generate(
			target,
			catalog.manifest.SchemaVersion,
			catalog.manifest.ExtractorVersion,
			base,
			edited,
		)
		if err != nil {
			return nil, fmt.Errorf("generating cumulative patch for %q: %w", target, err)
		}
		applied, err := conversationpatch.Apply(base, patchData, patchExpectation(catalog.manifest, target))
		if err != nil {
			return nil, fmt.Errorf("validating cumulative patch for %q: %w", target, err)
		}
		if !bytes.Equal(applied, edited) {
			return nil, fmt.Errorf("generated cumulative patch for %q did not reproduce edited bytes", target)
		}
		generated[target] = patchData
		switch {
		case !existed:
			result.Added++
		case bytes.Equal(oldPatch, patchData):
			result.Unchanged++
		default:
			result.Updated++
		}
		warnings := dialogueLinesTouched(catalog.parsed[target], mustLoadConversation(target, edited))
		if warnings > 0 {
			result.DialogueWarnings[target] = warnings
			result.DialogueLinesTouched += warnings
			result.DialogueConversations++
		}
	}
	if err := publishPatches(options.PatchDir, catalog.manifest.ExpectedFiles, generated); err != nil {
		return nil, err
	}

	return result, nil
}

// Verify strictly applies project patches to base bytes and compares every result with edited.
//
//nolint:err113 // Equality diagnostics identify the exact private-only edited target.
func Verify(options Options) (*VerifyResult, error) {
	if err := validateOptions(options, false, true); err != nil {
		return nil, err
	}
	catalog, err := loadCatalog(options)
	if err != nil {
		return nil, err
	}
	patches, err := loadPatches(options.PatchDir, catalog.manifest.ExpectedFiles)
	if err != nil {
		return nil, err
	}
	for _, target := range catalog.manifest.ExpectedFiles {
		effective := catalog.base[target]
		if patchData, exists := patches[target]; exists {
			effective, err = conversationpatch.Apply(
				effective,
				patchData,
				patchExpectation(catalog.manifest, target),
			)
			if err != nil {
				return nil, fmt.Errorf("applying project patch for %q: %w", target, err)
			}
		}
		conversation, err := semantic.Load(target, bytes.NewReader(effective))
		if err != nil {
			return nil, fmt.Errorf("validating patched conversation %q: %w", target, err)
		}
		if err := validateIdentity(target, conversation); err != nil {
			return nil, err
		}
		if !bytes.Equal(effective, catalog.edited[target]) {
			return nil, fmt.Errorf("edited conversation %q is not reproduced byte-for-byte by project patches", target)
		}
	}

	return &VerifyResult{
		ConversationCount: len(catalog.manifest.ExpectedFiles),
		AppliedPatches:    len(patches),
	}, nil
}

func loadCatalog(options Options) (*catalog, error) {
	if err := validateWorkspaceRoot(options.Workspace); err != nil {
		return nil, err
	}
	catalog, err := loadBase(options)
	if err != nil {
		return nil, err
	}
	editedRoot := filepath.Join(options.Workspace, editedDirectory)
	if err := validateConversationTree(editedRoot, catalog.manifest.ExpectedFiles, false); err != nil {
		return nil, fmt.Errorf("validating edited conversation tree: %w", err)
	}
	catalog.edited = make(map[string][]byte, len(catalog.manifest.ExpectedFiles))
	for _, target := range catalog.manifest.ExpectedFiles {
		filename := filepath.Join(editedRoot, filepath.FromSlash(target))
		data, err := os.ReadFile(filename) //nolint:gosec // Manifest paths are strictly validated by the extractor.
		if err != nil {
			return nil, fmt.Errorf("reading edited conversation %q: %w", target, err)
		}
		if err := requireTerminalNewline(target, data); err != nil {
			return nil, err
		}
		conversation, err := semantic.LoadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("validating edited conversation %q: %w", target, err)
		}
		if err := validateIdentity(target, conversation); err != nil {
			return nil, err
		}
		catalog.edited[target] = data
	}

	return catalog, nil
}

//nolint:err113,funlen // Manifest, audit, schema, content hash, tree, and semantics form one integrity check.
func loadBase(options Options) (*catalog, error) {
	baseRoot := filepath.Join(options.Workspace, baseDirectory)
	manifest, err := extractor.LoadManifest(filepath.Join(baseRoot, manifestFilename))
	if err != nil {
		return nil, fmt.Errorf("loading conversation workspace base manifest: %w", err)
	}
	if err := validateBaseTree(baseRoot, manifest); err != nil {
		return nil, err
	}
	auditPath := filepath.Join(baseRoot, manifest.AuditFile)
	auditData, err := os.ReadFile(auditPath) //nolint:gosec // Strict manifest validation fixes the relative audit path.
	if err != nil {
		return nil, fmt.Errorf("reading conversation workspace base audit: %w", err)
	}
	auditDigest := sha256.Sum256(auditData)
	if actual := hex.EncodeToString(auditDigest[:]); actual != manifest.AuditSHA256 {
		return nil, fmt.Errorf("base audit SHA-256 is %q, manifest records %q", actual, manifest.AuditSHA256)
	}
	audit, err := extractor.LoadAudit(auditPath)
	if err != nil {
		return nil, fmt.Errorf("loading conversation workspace base audit: %w", err)
	}
	if len(audit.Entries) != manifest.AuditEntryCount {
		return nil, fmt.Errorf(
			"base audit contains %d entries, manifest records %d",
			len(audit.Entries),
			manifest.AuditEntryCount,
		)
	}
	schemaPath := filepath.Join(baseRoot, manifest.SchemaFile)
	schemaData, err := os.ReadFile(schemaPath) //nolint:gosec // Strict manifest fixes path.
	if err != nil {
		return nil, fmt.Errorf("reading conversation workspace base schema: %w", err)
	}
	if !bytes.Equal(schemaData, semantic.JSONSchema()) {
		return nil, errors.New("conversation workspace base schema differs from embedded semantic schema")
	}
	hasher := sha256.New()
	hashContentEntry(hasher, manifest.SchemaFile, schemaData)
	base := make(map[string][]byte, len(manifest.ExpectedFiles))
	parsed := make(map[string]*semantic.Conversation, len(manifest.ExpectedFiles))
	for _, target := range manifest.ExpectedFiles {
		filename := filepath.Join(baseRoot, filepath.FromSlash(target))
		data, err := os.ReadFile(filename) //nolint:gosec // Manifest paths are strictly validated by the extractor.
		if err != nil {
			return nil, fmt.Errorf("reading base conversation %q: %w", target, err)
		}
		if err := requireTerminalNewline(target, data); err != nil {
			return nil, err
		}
		conversation, err := semantic.LoadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("validating base conversation %q: %w", target, err)
		}
		if err := validateIdentity(target, conversation); err != nil {
			return nil, err
		}
		hashContentEntry(hasher, target, data)
		base[target] = data
		parsed[target] = conversation
	}
	hashContentEntry(hasher, manifest.AuditFile, auditData)
	if actual := hex.EncodeToString(hasher.Sum(nil)); actual != manifest.ContentSHA256 {
		return nil, fmt.Errorf("base content SHA-256 is %q, manifest records %q", actual, manifest.ContentSHA256)
	}

	return &catalog{options: options, manifest: manifest, base: base, edited: nil, parsed: parsed}, nil
}

//nolint:err113 // Workspace ownership validation is a closed root-directory contract.
func validateWorkspaceRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("checking conversation workspace %q: %w", root, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("conversation workspace %q is not a directory or is a symlink", root)
	}
	if sharedWithOthers(info.Mode()) {
		return fmt.Errorf("conversation workspace %q is accessible by group or other users", root)
	}
	allowed := map[string]struct{}{baseDirectory: {}, editedDirectory: {}}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("reading conversation workspace %q: %w", root, err)
	}
	for _, entry := range entries {
		if _, exists := allowed[entry.Name()]; !exists {
			return fmt.Errorf("conversation workspace contains unrecognized path %q", entry.Name())
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			return fmt.Errorf("conversation workspace path %q is not a directory or is a symlink", entry.Name())
		}
		delete(allowed, entry.Name())
	}
	if len(allowed) != 0 {
		return fmt.Errorf("conversation workspace is missing required directories: %v", sortedKeys(allowed))
	}

	return nil
}

func validateBaseTree(root string, manifest *extractor.Manifest) error {
	allowedFiles := map[string]struct{}{
		manifestFilename: {}, manifest.SchemaFile: {}, manifest.AuditFile: {},
	}
	for _, target := range manifest.ExpectedFiles {
		allowedFiles[filepath.FromSlash(target)] = struct{}{}
	}
	allowedDirectories := expectedDirectories(manifest.ExpectedFiles)
	if err := validateExactTree(root, allowedFiles, allowedDirectories, true); err != nil {
		return fmt.Errorf("validating protected conversation base: %w", err)
	}

	return nil
}

func validateConversationTree(root string, expected []string, protected bool) error {
	allowedFiles := make(map[string]struct{}, len(expected))
	for _, target := range expected {
		allowedFiles[filepath.FromSlash(target)] = struct{}{}
	}
	if err := validateExactTree(root, allowedFiles, expectedDirectories(expected), protected); err != nil {
		return err
	}

	return nil
}

//nolint:cyclop,err113,funlen // Closed-tree ownership and permission diagnostics identify exact paths.
func validateExactTree(
	root string,
	allowedFiles map[string]struct{},
	allowedDirectories map[string]struct{},
	protected bool,
) error {
	seenFiles := make(map[string]struct{}, len(allowedFiles))
	seenDirectories := make(map[string]struct{}, len(allowedDirectories))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("reading path metadata %q: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("tree contains symlink %q", path)
		}
		if sharedWithOthers(info.Mode()) {
			return fmt.Errorf("tree path %q is accessible by group or other users", path)
		}
		if protected && info.Mode().Perm()&0o222 != 0 {
			return fmt.Errorf("protected base path %q is writable", path)
		}
		if path == root {
			if !entry.IsDir() {
				return fmt.Errorf("tree root %q is not a directory", root)
			}

			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolving tree path %q: %w", path, err)
		}
		if entry.IsDir() {
			if _, exists := allowedDirectories[relative]; !exists {
				return fmt.Errorf("tree contains unrecognized directory %q", relative)
			}
			seenDirectories[relative] = struct{}{}

			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("tree contains non-regular file %q", relative)
		}
		if _, exists := allowedFiles[relative]; !exists {
			return fmt.Errorf("tree contains unrecognized file %q", relative)
		}
		seenFiles[relative] = struct{}{}

		return nil
	})
	if err != nil {
		return fmt.Errorf("walking exact conversation tree %q: %w", root, err)
	}
	if len(seenFiles) != len(allowedFiles) {
		return errors.New("tree does not contain the exact expected file list")
	}
	if len(seenDirectories) != len(allowedDirectories) {
		return errors.New("tree does not contain the exact expected directory list")
	}

	return nil
}

func expectedDirectories(expected []string) map[string]struct{} {
	directories := make(map[string]struct{})
	for _, target := range expected {
		directories[filepath.Dir(filepath.FromSlash(target))] = struct{}{}
	}

	return directories
}

func createEditedTree(root string, catalog *catalog, patches map[string][]byte) error {
	editedRoot := filepath.Join(root, editedDirectory)
	if err := os.Mkdir(editedRoot, privateDirectory); err != nil {
		return fmt.Errorf("creating edited conversation directory: %w", err)
	}
	for directory := range expectedDirectories(catalog.manifest.ExpectedFiles) {
		if err := os.Mkdir(filepath.Join(editedRoot, directory), privateDirectory); err != nil {
			return fmt.Errorf("creating edited source directory %q: %w", directory, err)
		}
	}
	for _, target := range catalog.manifest.ExpectedFiles {
		data := catalog.base[target]
		if patchData, exists := patches[target]; exists {
			var err error
			data, err = conversationpatch.Apply(data, patchData, patchExpectation(catalog.manifest, target))
			if err != nil {
				return fmt.Errorf("applying project patch for %q: %w", target, err)
			}
		}
		conversation, err := semantic.Load(target, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("validating effective conversation %q: %w", target, err)
		}
		if err := validateIdentity(target, conversation); err != nil {
			return err
		}
		if err := writeExclusive(filepath.Join(editedRoot, filepath.FromSlash(target)), data, privateFile); err != nil {
			return err
		}
	}

	return nil
}

//nolint:cyclop,gocognit,funlen,err113 // Patch-root validation intentionally accepts only catalog-derived paths.
func loadPatches(root string, expected []string) (map[string][]byte, error) {
	patches := make(map[string][]byte)
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return patches, nil
	}
	if err != nil {
		return nil, fmt.Errorf("checking conversation patch root %q: %w", root, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("conversation patch root %q is not a directory or is a symlink", root)
	}
	targetByPatch := make(map[string]string, len(expected))
	allowedDirectories := make(map[string]struct{})
	for _, target := range expected {
		patchName := patchPath(target)
		targetByPatch[filepath.FromSlash(patchName)] = target
		allowedDirectories[filepath.Dir(filepath.FromSlash(patchName))] = struct{}{}
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("conversation patch root contains symlink %q", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolving conversation patch path %q: %w", path, err)
		}
		if entry.IsDir() {
			if _, exists := allowedDirectories[relative]; !exists {
				return fmt.Errorf("conversation patch root contains unrecognized directory %q", relative)
			}

			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("reading conversation patch metadata %q: %w", relative, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("conversation patch root contains non-regular file %q", relative)
		}
		if relative == patchScaffold {
			data, err := os.ReadFile(path) //nolint:gosec // Path was verified inside the selected patch root.
			if err != nil {
				return fmt.Errorf("reading conversation patch scaffold: %w", err)
			}
			if len(data) != 0 {
				return errors.New("conversation patch scaffold .gitkeep must be empty")
			}

			return nil
		}
		if relative == patchPackageFile {
			data, err := os.ReadFile(path) //nolint:gosec // Path was verified inside the selected patch root.
			if err != nil {
				return fmt.Errorf("reading conversation patch package file: %w", err)
			}
			if string(data) != patchPackageSource {
				return errors.New("conversation patch package embed.go does not match the managed source")
			}

			return nil
		}
		target, exists := targetByPatch[relative]
		if !exists {
			return fmt.Errorf("conversation patch root contains unrecognized file %q", relative)
		}
		data, err := os.ReadFile(path) //nolint:gosec // Path is one exact catalog-derived patch location.
		if err != nil {
			return fmt.Errorf("reading conversation patch %q: %w", relative, err)
		}
		metadata, err := conversationpatch.ParseMetadata(data)
		if err != nil {
			return fmt.Errorf("validating conversation patch %q: %w", relative, err)
		}
		if metadata.Target != target {
			return fmt.Errorf("conversation patch %q targets %q, want %q", relative, metadata.Target, target)
		}
		if metadata.SchemaVersion != semantic.SchemaVersion {
			return fmt.Errorf(
				"conversation patch %q schema is %q, want %q",
				relative,
				metadata.SchemaVersion,
				semantic.SchemaVersion,
			)
		}
		if metadata.ExtractorVersion != extractor.ExtractorVersion {
			return fmt.Errorf(
				"conversation patch %q extractor is %q, want %q",
				relative,
				metadata.ExtractorVersion,
				extractor.ExtractorVersion,
			)
		}
		if _, duplicate := patches[target]; duplicate {
			return fmt.Errorf("multiple conversation patches target %q", target)
		}
		patches[target] = data

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("validating conversation patch root %q: %w", root, err)
	}

	return patches, nil
}

func publishPatches(root string, expected []string, patches map[string][]byte) (err error) {
	parent := filepath.Dir(root)
	if err := os.MkdirAll(parent, projectDirectory); err != nil {
		return fmt.Errorf("creating conversation patch parent %q: %w", parent, err)
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(root)+".staging-*")
	if err != nil {
		return fmt.Errorf("creating conversation patch staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			if cleanupErr := os.RemoveAll(stage); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("removing patch staging directory %q: %w", stage, cleanupErr))
			}
		}
	}()
	if err := os.Chmod(stage, projectDirectory); err != nil {
		return fmt.Errorf("setting conversation patch staging permissions: %w", err)
	}
	if err := writeExclusive(filepath.Join(stage, patchScaffold), nil, projectFile); err != nil {
		return err
	}
	if err := writeExclusive(filepath.Join(stage, patchPackageFile), []byte(patchPackageSource), projectFile); err != nil {
		return err
	}
	for _, target := range expected {
		data, exists := patches[target]
		if !exists {
			continue
		}
		filename := filepath.Join(stage, filepath.FromSlash(patchPath(target)))
		if err := os.MkdirAll(filepath.Dir(filename), projectDirectory); err != nil {
			return fmt.Errorf("creating staged conversation patch directory: %w", err)
		}
		if err := writeExclusive(filename, data, projectFile); err != nil {
			return err
		}
	}
	if _, err := loadPatches(stage, expected); err != nil {
		return fmt.Errorf("validating staged conversation patches: %w", err)
	}
	if err := syncTree(stage); err != nil {
		return err
	}
	if err := replaceDirectory(stage, root); err != nil {
		return err
	}
	published = true

	return nil
}

func replaceDirectory(stage, destination string) error {
	_, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(stage, destination); err != nil {
			return fmt.Errorf("publishing conversation patch root %q: %w", destination, err)
		}

		return syncDirectory(filepath.Dir(destination))
	}
	if err != nil {
		return fmt.Errorf("checking conversation patch root before publication: %w", err)
	}
	backup := stage + ".previous"
	if err := os.Rename(destination, backup); err != nil {
		return fmt.Errorf("moving existing conversation patch root aside: %w", err)
	}
	if err := os.Rename(stage, destination); err != nil {
		rollbackErr := os.Rename(backup, destination)
		if rollbackErr != nil {
			return fmt.Errorf("publishing patches and restoring previous root: %w", errors.Join(err, rollbackErr))
		}

		return fmt.Errorf("publishing conversation patch root: %w", err)
	}
	if err := syncDirectory(filepath.Dir(destination)); err != nil {
		return err
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("removing previous conversation patch root: %w", err)
	}

	return syncDirectory(filepath.Dir(destination))
}

//nolint:err113 // Protection diagnostics identify exact unexpected base paths.
func protectBase(root string) error {
	files := make([]string, 0)
	directories := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to protect base containing symlink %q", path)
		}
		if entry.IsDir() {
			directories = append(directories, path)
		} else {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("walking conversation base for protection: %w", err)
	}
	for _, filename := range files {
		if err := os.Chmod(filename, protectedFile); err != nil {
			return fmt.Errorf("protecting conversation base file %q: %w", filename, err)
		}
	}
	slices.Reverse(directories)
	for _, directory := range directories {
		if err := os.Chmod(directory, protectedDirectory); err != nil {
			return fmt.Errorf("protecting conversation base directory %q: %w", directory, err)
		}
	}

	return nil
}

func makeTreeRemovable(root string) error {
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}

			return walkErr
		}
		if entry.IsDir() {
			return os.Chmod(path, privateDirectory) //nolint:gosec // Cleanup tree was created and owned by this transaction.
		}

		return os.Chmod(path, privateFile) //nolint:gosec // Cleanup tree was created and owned by this transaction.
	})
	if err != nil {
		return fmt.Errorf("making failed workspace staging tree removable: %w", err)
	}

	return nil
}

//nolint:err113 // Identity diagnostics include the exact target, id, and origin mismatch.
func validateIdentity(target string, conversation *semantic.Conversation) error {
	slashTarget := filepath.ToSlash(target)
	parts := strings.Split(slashTarget, "/")
	if len(parts) != 2 || filepath.Ext(parts[1]) != ".yaml" {
		return fmt.Errorf("conversation path %q does not identify source/dialog YAML", target)
	}
	dialogText := strings.TrimSuffix(parts[1], ".yaml")
	dialog, err := strconv.Atoi(dialogText)
	if err != nil {
		return fmt.Errorf("conversation path %q has invalid dialog number: %w", target, err)
	}
	wantID := fmt.Sprintf("original.%s.%03d", parts[0], dialog)
	if conversation.ID != wantID {
		return fmt.Errorf("conversation %q has id %q, want %q", target, conversation.ID, wantID)
	}
	origin, ok := conversation.Origin.(*semantic.OriginalOrigin)
	if !ok {
		return fmt.Errorf("conversation %q does not have original origin", target)
	}
	if string(origin.Source) != parts[0] || origin.Dialog != dialog {
		return fmt.Errorf(
			"conversation %q origin is source %q dialog %d, want source %q dialog %d",
			target,
			origin.Source,
			origin.Dialog,
			parts[0],
			dialog,
		)
	}

	return nil
}

func dialogueLinesTouched(base, edited *semantic.Conversation) int {
	baseLines := dialogueLines(base)
	editedLines := dialogueLines(edited)
	matcher := difflib.NewMatcher(baseLines, editedLines)
	count := 0
	for _, operation := range matcher.GetOpCodes() {
		if operation.Tag == 'e' {
			continue
		}
		oldCount := operation.I2 - operation.I1
		newCount := operation.J2 - operation.J1
		count += max(oldCount, newCount)
	}

	return count
}

func dialogueLines(conversation *semantic.Conversation) []string {
	lines := make([]string, 0)
	appendBodyDialogue(&lines, conversation.Name)
	appendBodyDialogue(&lines, conversation.Description)
	appendBodyDialogue(&lines, conversation.Greeting)
	appendBodyDialogue(&lines, conversation.Job)
	appendBodyDialogue(&lines, conversation.Farewell)
	for _, topic := range conversation.Topics {
		appendBodyDialogue(&lines, topic.Response)
	}
	questionNames := make([]string, 0, len(conversation.Questions))
	for name := range conversation.Questions {
		questionNames = append(questionNames, name)
	}
	sort.Strings(questionNames)
	for _, name := range questionNames {
		question := conversation.Questions[name]
		appendBodyDialogue(&lines, question.Prompt)
		for _, answer := range question.Answers {
			appendBodyDialogue(&lines, answer.Response)
		}
		appendBodyDialogue(&lines, question.Default)
	}

	return lines
}

func appendBodyDialogue(lines *[]string, body semantic.Body) {
	for _, action := range body {
		switch value := action.(type) {
		case *semantic.SayAction:
			appendRichTextLines(lines, value.Content)
		case *semantic.NarrateAction:
			appendRichTextLines(lines, value.Content)
		case *semantic.BranchAction:
			appendBodyDialogue(lines, value.Then)
			if value.Otherwise != nil {
				appendBodyDialogue(lines, *value.Otherwise)
			}
		}
	}
}

func appendRichTextLines(lines *[]string, content semantic.RichText) {
	current := strings.Builder{}
	for _, part := range content {
		switch inline := part.(type) {
		case *semantic.TextPart:
			current.WriteString(inline.Text)
		case *semantic.RunicPart:
			current.WriteString(inline.Text)
		case *semantic.LineBreakPart:
			*lines = append(*lines, current.String())
			current.Reset()
		}
	}
	*lines = append(*lines, current.String())
}

func patchExpectation(manifest *extractor.Manifest, target string) conversationpatch.Expectation {
	return conversationpatch.Expectation{
		Target:           target,
		SchemaVersion:    manifest.SchemaVersion,
		ExtractorVersion: manifest.ExtractorVersion,
	}
}

func patchPath(target string) string {
	return strings.TrimSuffix(target, filepath.Ext(target)) + ".patch"
}

//nolint:err113 // Generated-patch corruption is a programming error with one direct diagnostic.
func stripPatchMetadata(data []byte) ([]byte, error) {
	position := 0
	for range 5 {
		next := bytes.IndexByte(data[position:], '\n')
		if next < 0 {
			return nil, errors.New("generated conversation patch is missing metadata lines")
		}
		position += next + 1
	}

	return data[position:], nil
}

//nolint:err113 // Diagnostic identifies the exact non-canonical conversation.
func requireTerminalNewline(target string, data []byte) error {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return fmt.Errorf("conversation %q must end with a newline", target)
	}

	return nil
}

func mustLoadConversation(target string, data []byte) *semantic.Conversation {
	conversation, err := semantic.Load(target, bytes.NewReader(data))
	if err != nil {
		panic(fmt.Sprintf("validated conversation %q failed to reload: %v", target, err))
	}

	return conversation
}

func hashContentEntry(writer hash.Hash, name string, data []byte) {
	var length [contentDigestLength]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(name)))
	_, _ = writer.Write(length[:])
	_, _ = writer.Write([]byte(name))
	binary.BigEndian.PutUint64(length[:], uint64(len(data)))
	_, _ = writer.Write(length[:])
	_, _ = writer.Write(data)
}

func writeExclusive(filename string, data []byte, permissions fs.FileMode) error {
	file, err := os.OpenFile( //nolint:gosec // Caller selects the permissions for private or tracked output.
		filename,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		permissions,
	)
	if err != nil {
		return fmt.Errorf("creating file %q: %w", filename, err)
	}
	_, writeErr := file.Write(data)
	if writeErr != nil {
		writeErr = fmt.Errorf("writing file %q: %w", filename, writeErr)
	} else if syncErr := file.Sync(); syncErr != nil {
		writeErr = fmt.Errorf("syncing file %q: %w", filename, syncErr)
	}
	closeErr := file.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("closing file %q: %w", filename, closeErr)
	}

	return errors.Join(writeErr, closeErr)
}

func syncTree(root string) error {
	directories := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("walking directory tree %q for sync: %w", root, err)
	}
	slices.Reverse(directories)
	for _, directory := range directories {
		if err := syncDirectory(directory); err != nil {
			return err
		}
	}

	return nil
}

func syncDirectory(directory string) error {
	if err := repositoryfs.SyncDirectory(directory); err != nil {
		return fmt.Errorf("syncing directory %q: %w", directory, err)
	}

	return nil
}

//nolint:err113 // Refusal diagnostic identifies the exact existing workspace path.
func requireAbsent(path string) error {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking conversation workspace destination %q: %w", path, err)
	}

	return fmt.Errorf("conversation workspace %q already exists; refusing to overwrite it", path)
}

//nolint:err113 // Permission diagnostics identify the selected workspace parent.
func ensurePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("checking workspace parent %q: %w", path, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace parent %q is not a directory or is a symlink", path)
	}
	if sharedWithOthers(info.Mode()) {
		return fmt.Errorf("workspace parent %q is accessible by group or other users", path)
	}

	return nil
}

//nolint:err113 // CLI option diagnostics directly identify each missing required path.
func validateOptions(options Options, requireData, requirePatch bool) error {
	if strings.TrimSpace(options.Workspace) == "" {
		return errors.New("conversation workspace path is required")
	}
	if requireData && strings.TrimSpace(options.DataDir) == "" {
		return errors.New("original conversation data directory is required")
	}
	if requirePatch && strings.TrimSpace(options.PatchDir) == "" {
		return errors.New("conversation patch directory is required")
	}

	return nil
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}
