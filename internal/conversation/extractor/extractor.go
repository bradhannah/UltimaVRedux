// Package extractor converts original Ultima V TLK data into semantic YAML.
package extractor

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/internal/files"
	"github.com/bradhannah/UltimaVRedux/internal/repositoryfs"
	"gopkg.in/yaml.v3"
)

const (
	compressedWordsOffset     = 0x104c
	compressedWordsLength     = 0x24e
	totalConversationCount    = 135
	directoryPermissions      = 0o700
	filePermissions           = 0o600
	manifestIndent            = 2
	castleConversationCount   = 40
	towneConversationCount    = 48
	dwellingConversationCount = 15
	keepConversationCount     = 32
)

type sourceSpec struct {
	filename  string
	directory string
	source    semantic.OriginalSource
	count     int
}

// Options configures one explicit original-data extraction.
type Options struct {
	DataDir string
	Output  string
	Force   bool
}

// Result summarizes a successful extraction.
type Result struct {
	ConversationCount int
	AuditEntryCount   int
	Output            string
	ContentSHA256     string
}

// ImportedConversation is one canonical semantic document produced in memory.
type ImportedConversation struct {
	Path         string
	Data         []byte
	Conversation *semantic.Conversation
}

// ImportResult contains the complete deterministic original conversation catalog.
type ImportResult struct {
	Conversations   []ImportedConversation
	AuditEntryCount int
}

var (
	errImportDataDirectoryRequired = errors.New("import data directory is required")
	errImportDataDirectoryInvalid  = errors.New("import data directory is invalid")
	errImportCompressedWords       = errors.New("import compressed-word data is invalid")
	errImportConversationCount     = errors.New("import conversation count is invalid")
)

// Import converts all required original files into canonical semantic documents
// without publishing extracted copyrighted content to disk.
func Import(dataDir string) (*ImportResult, error) {
	absoluteDataDir, err := validateImportDataDirectory(dataDir)
	if err != nil {
		return nil, err
	}

	inputs, _, err := readInputs(absoluteDataDir)
	if err != nil {
		return nil, err
	}
	dictionary, err := importDictionary(inputs[files.DataOvl])
	if err != nil {
		return nil, err
	}

	result := &ImportResult{Conversations: make([]ImportedConversation, 0, totalConversationCount), AuditEntryCount: 0}
	for _, spec := range conversationSources() {
		documents, auditCount, importErr := importSource(spec, inputs[spec.filename], dictionary)
		if importErr != nil {
			return nil, importErr
		}
		result.Conversations = append(result.Conversations, documents...)
		result.AuditEntryCount += auditCount
	}
	if len(result.Conversations) != totalConversationCount {
		return nil, fmt.Errorf(
			"%w: imported %d, want %d",
			errImportConversationCount,
			len(result.Conversations),
			totalConversationCount,
		)
	}

	return result, nil
}

func validateImportDataDirectory(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", errImportDataDirectoryRequired
	}
	absoluteDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return "", fmt.Errorf("resolving data directory %q: %w", dataDir, err)
	}
	info, err := os.Stat(absoluteDataDir)
	if err != nil {
		return "", fmt.Errorf("checking data directory %q: %w", absoluteDataDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %q is not a directory", errImportDataDirectoryInvalid, absoluteDataDir)
	}

	return absoluteDataDir, nil
}

func importDictionary(dataOvl []byte) (map[byte]string, error) {
	if len(dataOvl) < compressedWordsOffset+compressedWordsLength {
		return nil, fmt.Errorf(
			"%w: %s needs at least 0x%x bytes, has 0x%x",
			errImportCompressedWords,
			files.DataOvl,
			compressedWordsOffset+compressedWordsLength,
			len(dataOvl),
		)
	}
	dictionary, err := parseWordTable(dataOvl[compressedWordsOffset : compressedWordsOffset+compressedWordsLength])
	if err != nil {
		return nil, fmt.Errorf("parsing compressed words from %s offset 0x%x: %w", files.DataOvl, compressedWordsOffset, err)
	}

	return dictionary, nil
}

func importSource(spec sourceSpec, data []byte, dictionary map[byte]string) ([]ImportedConversation, int, error) {
	rawConversations, err := parseTLK(spec.filename, data, spec.count)
	if err != nil {
		return nil, 0, err
	}
	documents := make([]ImportedConversation, 0, len(rawConversations))
	auditCount := 0
	for _, raw := range rawConversations {
		conversation, auditEntries, convertErr := convertConversation(raw, spec.source, dictionary)
		if convertErr != nil {
			return nil, 0, convertErr
		}
		data, marshalErr := semantic.Marshal(conversation)
		if marshalErr != nil {
			return nil, 0, fmt.Errorf("marshaling %s conversation %d: %w", spec.filename, raw.id, marshalErr)
		}
		relativeName := fmt.Sprintf("%s/%03d.yaml", spec.directory, raw.id)
		validated, loadErr := semantic.Load(relativeName, strings.NewReader(string(data)))
		if loadErr != nil {
			return nil, 0, fmt.Errorf("validating generated %s conversation %d: %w", spec.filename, raw.id, loadErr)
		}
		documents = append(documents, ImportedConversation{
			Path: relativeName, Data: data, Conversation: validated,
		})
		auditCount += len(auditEntries)
	}

	return documents, auditCount, nil
}

// Extract imports all four original TLK files and DATA.OVL into a private tree.
//
//nolint:gocognit,cyclop,gocyclo,err113,funlen,nonamedreturns // Staging and cleanup form one transaction.
func Extract(options Options) (result *Result, err error) {
	dataDir, output, err := validateOptions(options)
	if err != nil {
		return nil, err
	}
	if err := prepareDestination(output, options.Force); err != nil {
		return nil, err
	}

	inputs, sourceDigests, err := readInputs(dataDir)
	if err != nil {
		return nil, err
	}
	dataOvl := inputs[files.DataOvl]
	if len(dataOvl) < compressedWordsOffset+compressedWordsLength {
		return nil, fmt.Errorf(
			"reading compressed words from %s: need at least 0x%x bytes, have 0x%x",
			files.DataOvl,
			compressedWordsOffset+compressedWordsLength,
			len(dataOvl),
		)
	}
	dictionary, err := parseWordTable(dataOvl[compressedWordsOffset : compressedWordsOffset+compressedWordsLength])
	if err != nil {
		return nil, fmt.Errorf("parsing compressed words from %s offset 0x%x: %w", files.DataOvl, compressedWordsOffset, err)
	}

	parent := filepath.Dir(output)
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(output)+".staging-*")
	if err != nil {
		return nil, fmt.Errorf("creating extraction staging directory beside %q: %w", output, err)
	}
	stagePublished := false
	defer func() {
		if !stagePublished {
			if cleanupErr := os.RemoveAll(stage); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("removing extraction staging directory %q: %w", stage, cleanupErr))
			}
		}
	}()
	if err := os.Chmod(stage, directoryPermissions); err != nil {
		return nil, fmt.Errorf("setting private permissions on staging directory %q: %w", stage, err)
	}

	expectedFiles := expectedConversationFiles()
	actualFiles := make([]string, 0, totalConversationCount)
	auditEntries := make([]AuditEntry, 0)
	contentHasher := sha256.New()
	if err := writeHashedFile(stage, schemaFilename, semantic.JSONSchema(), contentHasher); err != nil {
		return nil, err
	}
	for _, spec := range conversationSources() {
		directory := filepath.Join(stage, spec.directory)
		if err := os.Mkdir(directory, directoryPermissions); err != nil {
			return nil, fmt.Errorf("creating private conversation directory %q: %w", directory, err)
		}
		conversations, parseErr := parseTLK(spec.filename, inputs[spec.filename], spec.count)
		if parseErr != nil {
			return nil, parseErr
		}
		for _, raw := range conversations {
			conversation, conversationAudit, convertErr := convertConversation(raw, spec.source, dictionary)
			if convertErr != nil {
				return nil, convertErr
			}
			auditEntries = append(auditEntries, conversationAudit...)
			data, marshalErr := semantic.Marshal(conversation)
			if marshalErr != nil {
				return nil, fmt.Errorf("marshaling %s conversation %d: %w", spec.filename, raw.id, marshalErr)
			}
			relativeName := filepath.ToSlash(filepath.Join(spec.directory, fmt.Sprintf("%03d.yaml", raw.id)))
			if writeErr := writeHashedFile(stage, relativeName, data, contentHasher); writeErr != nil {
				return nil, writeErr
			}
			if _, loadErr := semantic.LoadFile(filepath.Join(stage, filepath.FromSlash(relativeName))); loadErr != nil {
				return nil, fmt.Errorf("validating generated semantic conversation %q: %w", relativeName, loadErr)
			}
			actualFiles = append(actualFiles, relativeName)
		}
	}
	if !slices.Equal(expectedFiles, actualFiles) {
		return nil, fmt.Errorf(
			"generated conversation file list does not match expected list: got %d files, want %d",
			len(actualFiles),
			len(expectedFiles),
		)
	}
	audit := &Audit{
		FormatVersion: AuditFormat, ExtractorVersion: ExtractorVersion, Entries: auditEntries,
	}
	if err := audit.Validate(); err != nil {
		return nil, fmt.Errorf("validating generated extraction audit: %w", err)
	}
	auditData, err := marshalAudit(audit)
	if err != nil {
		return nil, err
	}
	auditDigestBytes := sha256.Sum256(auditData)
	auditDigest := hex.EncodeToString(auditDigestBytes[:])
	if err := writeHashedFile(stage, auditFilename, auditData, contentHasher); err != nil {
		return nil, err
	}
	contentDigest := hex.EncodeToString(contentHasher.Sum(nil))
	manifest := &Manifest{
		FormatVersion: ManifestFormat, SchemaVersion: semantic.SchemaVersion,
		ExtractorVersion: ExtractorVersion, SchemaFile: schemaFilename, AuditFile: auditFilename,
		AuditSHA256: auditDigest, AuditEntryCount: len(auditEntries),
		Sources: sourceDigests, ExpectedFiles: expectedFiles, ActualFiles: actualFiles,
		ContentSHA256: contentDigest,
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("validating generated extraction manifest: %w", err)
	}
	manifestData, err := marshalManifest(manifest)
	if err != nil {
		return nil, err
	}
	if err := writePrivateFile(filepath.Join(stage, manifestFilename), manifestData); err != nil {
		return nil, err
	}
	loadedManifest, err := LoadManifest(filepath.Join(stage, manifestFilename))
	if err != nil {
		return nil, err
	}
	if len(loadedManifest.ActualFiles) != totalConversationCount {
		return nil, fmt.Errorf(
			"generated manifest contains %d conversation files, want %d",
			len(loadedManifest.ActualFiles),
			totalConversationCount,
		)
	}
	if err := validateManifestAudit(stage, loadedManifest); err != nil {
		return nil, err
	}
	if err := syncDirectoryTree(stage); err != nil {
		return nil, err
	}
	if err := publishStage(stage, output, options.Force); err != nil {
		return nil, err
	}
	stagePublished = true

	return &Result{
		ConversationCount: len(actualFiles), AuditEntryCount: len(auditEntries),
		Output: output, ContentSHA256: contentDigest,
	}, nil
}

//nolint:gocognit,cyclop,funlen // Conversion keeps target resolution adjacent to semantic construction.
func convertConversation(
	raw rawConversation,
	source semantic.OriginalSource,
	dictionary map[byte]string,
) (*semantic.Conversation, []AuditEntry, error) {
	auditEntries := make([]AuditEntry, 0)
	questionNames := make(map[byte]string, len(raw.questions))
	for _, question := range raw.questions {
		questionNames[question.id] = questionName(question.id)
	}
	resolveTarget := func(entry instruction) (semantic.Body, error) {
		if entry.target == opReturnTopics {
			return semantic.Body{&semantic.ReturnToTopicsAction{}}, nil
		}
		if name, exists := questionNames[entry.target]; exists {
			return semantic.Body{&semantic.AskAction{Question: name}}, nil
		}

		return nil, entry.where.errorAt(entry.offset, "target references missing question block 0x%02x", entry.target)
	}
	roles := [...]semantic.ActionKind{
		semantic.ActionKindSay,
		semantic.ActionKindNarrate,
		semantic.ActionKindSay,
		semantic.ActionKindSay,
		semantic.ActionKindSay,
	}
	fixed := make([]semantic.Body, fixedFieldCount)
	for index, field := range raw.fixed {
		instructions, err := parseExecutable(field, dictionary, false)
		if err != nil {
			return nil, nil, err
		}
		body, err := instructionsToBody(instructions, roles[index], resolveTarget)
		if err != nil {
			return nil, nil, field.where.errorAt(field.offset, "converting fixed field %d: %v", index+1, err)
		}
		fixed[index] = body
	}
	topicGroups, topicAudit, err := convertResponseGroups(raw.topics, dictionary, resolveTarget)
	if err != nil {
		return nil, nil, err
	}
	auditEntries = append(auditEntries, topicAudit...)
	topics := make([]semantic.Topic, 0, len(topicGroups))
	seenTopicKeywords := make(map[string]struct{})
	for _, group := range topicGroups {
		keywords := removeShadowedKeywords(group.keywords, seenTopicKeywords)
		if len(keywords) > 0 {
			topics = append(topics, semantic.Topic{Keywords: keywords, Response: group.body})
		}
	}
	questions := make(map[string]semantic.Question, len(raw.questions))
	for _, rawQuestion := range raw.questions {
		prompt, promptErr := convertStandaloneField(rawQuestion.prompt, dictionary, resolveTarget, semantic.ActionKindSay)
		if promptErr != nil {
			return nil, nil, promptErr
		}
		if !rawQuestion.hasInput {
			if !bodyIsTerminal(prompt) {
				return nil, nil, rawQuestion.prompt.where.errorAt(
					rawQuestion.prompt.offset,
					"question 0x%02x omits fallback and answers but its prompt is not terminal",
					rawQuestion.id,
				)
			}
			questions[questionName(rawQuestion.id)] = semantic.Question{
				Prompt: prompt, Answers: []semantic.Answer{}, Default: semantic.Body{},
			}

			continue
		}
		fallback, fallbackErr := convertStandaloneField(
			rawQuestion.fallback,
			dictionary,
			resolveTarget,
			semantic.ActionKindSay,
		)
		if fallbackErr != nil {
			return nil, nil, fallbackErr
		}
		answerGroups, answerAudit, answerErr := convertResponseGroups(
			rawQuestion.answers,
			dictionary,
			resolveTarget,
		)
		if answerErr != nil {
			return nil, nil, answerErr
		}
		auditEntries = append(auditEntries, answerAudit...)
		if rawQuestion.unreachableOrphanAnswer != nil {
			auditEntries = append(
				auditEntries,
				newUnreachableOrphanAnswerKeyAudit(*rawQuestion.unreachableOrphanAnswer),
			)
		}
		answers := make([]semantic.Answer, 0, len(answerGroups))
		seenAnswerKeywords := make(map[string]struct{})
		for _, group := range answerGroups {
			keywords := removeShadowedKeywords(group.keywords, seenAnswerKeywords)
			if len(keywords) > 0 {
				answers = append(answers, semantic.Answer{Keywords: keywords, Response: group.body})
			}
		}
		questions[questionName(rawQuestion.id)] = semantic.Question{
			Prompt: prompt, Answers: answers, Default: fallback,
		}
	}

	return &semantic.Conversation{
		Schema: semantic.SchemaVersion, Protocol: semantic.ProtocolUltima5,
		ID:     fmt.Sprintf("original.%s.%03d", source, raw.id),
		Origin: &semantic.OriginalOrigin{Source: source, Dialog: raw.id},
		Name:   fixed[0], Description: fixed[1], Greeting: fixed[2], Job: fixed[3], Farewell: fixed[4],
		Topics: topics, Questions: questions,
	}, auditEntries, nil
}

func removeShadowedKeywords(keywords []string, seen map[string]struct{}) []string {
	visible := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		normalized := strings.ToLower(strings.Join(strings.Fields(keyword), " "))
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		visible = append(visible, keyword)
	}

	return visible
}

func convertStandaloneField(
	field rawField,
	dictionary map[byte]string,
	resolveTarget func(instruction) (semantic.Body, error),
	role semantic.ActionKind,
) (semantic.Body, error) {
	instructions, err := parseExecutable(field, dictionary, false)
	if err != nil {
		return nil, err
	}

	return instructionsToBody(instructions, role, resolveTarget)
}

func bodyIsTerminal(body semantic.Body) bool {
	if len(body) == 0 {
		return false
	}
	switch body[len(body)-1].(type) {
	case *semantic.AskAction, *semantic.JoinPartyAction, *semantic.ReturnToTopicsAction, *semantic.EndAction:
		return true
	default:
		return false
	}
}

//nolint:err113 // CLI option errors include user-selected paths.
func validateOptions(options Options) (string, string, error) {
	if strings.TrimSpace(options.DataDir) == "" {
		return "", "", errors.New("data directory is required")
	}
	if strings.TrimSpace(options.Output) == "" {
		return "", "", errors.New("output directory is required")
	}
	dataDir, err := filepath.Abs(options.DataDir)
	if err != nil {
		return "", "", fmt.Errorf("resolving data directory %q: %w", options.DataDir, err)
	}
	output, err := filepath.Abs(options.Output)
	if err != nil {
		return "", "", fmt.Errorf("resolving output directory %q: %w", options.Output, err)
	}
	if dataDir == output {
		return "", "", errors.New("output directory must differ from the original data directory")
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		return "", "", fmt.Errorf("checking data directory %q: %w", dataDir, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("data path %q is not a directory", dataDir)
	}
	parent := filepath.Dir(output)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return "", "", fmt.Errorf("checking output parent %q: %w", parent, err)
	}
	if !parentInfo.IsDir() {
		return "", "", fmt.Errorf("output parent %q is not a directory", parent)
	}

	return dataDir, output, nil
}

//nolint:err113 // Refusal errors include the exact destination path.
func prepareDestination(output string, force bool) error {
	info, err := os.Lstat(output)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking output directory %q: %w", output, err)
	}
	if !force {
		return fmt.Errorf("output directory %q already exists; use --force to replace an importer-owned tree", output)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to replace non-directory or symlink output %q", output)
	}
	manifest, err := LoadManifest(filepath.Join(output, manifestFilename))
	if err != nil {
		return fmt.Errorf("refusing to replace output not owned by this extractor: %w", err)
	}

	return validateOwnedTree(output, manifest)
}

//nolint:err113 // Ownership failures identify the exact unexpected path.
func validateOwnedTree(output string, manifest *Manifest) error {
	allowed := map[string]struct{}{manifestFilename: {}, schemaFilename: {}, auditFilename: {}}
	for _, name := range manifest.ActualFiles {
		allowed[filepath.FromSlash(name)] = struct{}{}
	}
	for _, spec := range conversationSources() {
		allowed[spec.directory] = struct{}{}
	}
	err := filepath.WalkDir(output, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == output {
			return nil
		}
		relative, err := filepath.Rel(output, path)
		if err != nil {
			return fmt.Errorf("resolving path %q relative to importer-owned tree: %w", path, err)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("owned tree contains symlink %q", relative)
		}
		if _, exists := allowed[relative]; !exists {
			return fmt.Errorf("owned tree contains unrecognized path %q", relative)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("refusing to replace modified importer-owned tree %q: %w", output, err)
	}

	return validateManifestAudit(output, manifest)
}

//nolint:err113 // Integrity failures identify the manifest field and selected audit file.
func validateManifestAudit(output string, manifest *Manifest) error {
	auditPath := filepath.Join(output, manifest.AuditFile)
	data, err := os.ReadFile(auditPath) //nolint:gosec // The validated manifest fixes the relative audit filename.
	if err != nil {
		return fmt.Errorf("reading manifest audit file %q: %w", auditPath, err)
	}
	digest := sha256.Sum256(data)
	if actual := hex.EncodeToString(digest[:]); actual != manifest.AuditSHA256 {
		return fmt.Errorf("audit_sha256 is %q, but audit file digest is %q", manifest.AuditSHA256, actual)
	}
	audit, err := LoadAudit(auditPath)
	if err != nil {
		return err
	}
	if len(audit.Entries) != manifest.AuditEntryCount {
		return fmt.Errorf(
			"audit_entry_count is %d, but audit file contains %d entries",
			manifest.AuditEntryCount,
			len(audit.Entries),
		)
	}

	return nil
}

func readInputs(dataDir string) (map[string][]byte, []SourceDigest, error) {
	inputs := make(map[string][]byte, len(sourceFilenames()))
	digests := make([]SourceDigest, 0, len(sourceFilenames()))
	for _, filename := range sourceFilenames() {
		path := filepath.Join(dataDir, filename)
		data, err := os.ReadFile(path) //nolint:gosec // The explicit data directory supplies fixed original filenames.
		if err != nil {
			return nil, nil, fmt.Errorf("reading required original file %q: %w", path, err)
		}
		digest := sha256.Sum256(data)
		inputs[filename] = data
		digests = append(digests, SourceDigest{File: filename, SHA256: hex.EncodeToString(digest[:])})
	}

	return inputs, digests, nil
}

func sourceFilenames() []string {
	return []string{files.DataOvl, files.CastleTlk, files.TowneTlk, files.DwellingTlk, files.KeepTlk}
}

func conversationSources() []sourceSpec {
	return []sourceSpec{
		{
			filename: files.CastleTlk, directory: "castle",
			source: semantic.OriginalSourceCastle, count: castleConversationCount,
		},
		{
			filename: files.TowneTlk, directory: "towne",
			source: semantic.OriginalSourceTowne, count: towneConversationCount,
		},
		{
			filename: files.DwellingTlk, directory: "dwelling",
			source: semantic.OriginalSourceDwelling, count: dwellingConversationCount,
		},
		{
			filename: files.KeepTlk, directory: "keep",
			source: semantic.OriginalSourceKeep, count: keepConversationCount,
		},
	}
}

func expectedConversationFiles() []string {
	files := make([]string, 0, totalConversationCount)
	for _, spec := range conversationSources() {
		for id := 1; id <= spec.count; id++ {
			files = append(files, fmt.Sprintf("%s/%03d.yaml", spec.directory, id))
		}
	}

	return files
}

func writeHashedFile(stage, relativeName string, data []byte, hasher io.Writer) error {
	if err := writePrivateFile(filepath.Join(stage, filepath.FromSlash(relativeName)), data); err != nil {
		return err
	}
	pathBytes := []byte(relativeName)
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(pathBytes)))
	if _, err := hasher.Write(length[:]); err != nil {
		return fmt.Errorf("hashing output path %q: %w", relativeName, err)
	}
	if _, err := hasher.Write(pathBytes); err != nil {
		return fmt.Errorf("hashing output path %q: %w", relativeName, err)
	}
	binary.BigEndian.PutUint64(length[:], uint64(len(data)))
	if _, err := hasher.Write(length[:]); err != nil {
		return fmt.Errorf("hashing output length %q: %w", relativeName, err)
	}
	if _, err := hasher.Write(data); err != nil {
		return fmt.Errorf("hashing output content %q: %w", relativeName, err)
	}

	return nil
}

func writePrivateFile(filename string, data []byte) error {
	file, err := os.OpenFile( //nolint:gosec // Private mode is intentional.
		filename,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		filePermissions,
	)
	if err != nil {
		return fmt.Errorf("creating private extraction file %q: %w", filename, err)
	}
	_, writeErr := file.Write(data)
	if writeErr != nil {
		writeErr = fmt.Errorf("writing extraction file %q: %w", filename, writeErr)
	} else if syncErr := file.Sync(); syncErr != nil {
		writeErr = fmt.Errorf("syncing extraction file %q: %w", filename, syncErr)
	}
	closeErr := file.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("closing extraction file %q: %w", filename, closeErr)
	}

	return errors.Join(writeErr, closeErr)
}

func marshalManifest(manifest *Manifest) ([]byte, error) {
	var builder strings.Builder
	encoder := yaml.NewEncoder(&builder)
	encoder.SetIndent(manifestIndent)
	if err := encoder.Encode(manifest); err != nil {
		return nil, fmt.Errorf("encoding extraction manifest: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("closing extraction manifest encoder: %w", err)
	}

	return []byte(builder.String()), nil
}

func syncDirectoryTree(root string) error {
	directories := make([]string, 0, len(conversationSources())+1)
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
		return fmt.Errorf("walking extraction staging tree %q: %w", root, err)
	}
	slices.Reverse(directories)
	for _, directory := range directories {
		if err := repositoryfs.SyncDirectory(directory); err != nil {
			return fmt.Errorf("syncing extraction directory %q: %w", directory, err)
		}
	}

	return nil
}

func publishStage(stage, output string, force bool) error {
	if !force {
		if err := os.Rename(stage, output); err != nil {
			return fmt.Errorf("publishing extraction tree %q: %w", output, err)
		}

		return syncParent(output)
	}
	backup := stage + ".previous"
	if err := os.Rename(output, backup); err != nil {
		return fmt.Errorf("moving existing importer-owned tree %q aside: %w", output, err)
	}
	if err := os.Rename(stage, output); err != nil {
		rollbackErr := os.Rename(backup, output)
		if rollbackErr != nil {
			return fmt.Errorf(
				"publishing replacement tree %q and restoring previous tree: %w",
				output,
				errors.Join(err, rollbackErr),
			)
		}

		return fmt.Errorf("publishing replacement tree %q: %w", output, err)
	}
	if err := syncParent(output); err != nil {
		return err
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("removing replaced importer-owned tree %q: %w", backup, err)
	}

	return syncParent(output)
}

func syncParent(path string) error {
	parent := filepath.Dir(path)
	if err := repositoryfs.SyncDirectory(parent); err != nil {
		return fmt.Errorf("syncing output parent %q: %w", parent, err)
	}

	return nil
}
