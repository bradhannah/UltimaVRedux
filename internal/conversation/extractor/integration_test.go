package extractor

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/test/originaldata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyntheticExtractionIntegration(t *testing.T) {
	dataDir := syntheticDataDirectory(t)
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")

	firstResult, err := Extract(Options{DataDir: dataDir, Output: first})
	require.NoError(t, err)
	secondResult, err := Extract(Options{DataDir: dataDir, Output: second})
	require.NoError(t, err)
	assert.Equal(t, totalConversationCount, firstResult.ConversationCount)
	assert.Zero(t, firstResult.AuditEntryCount)
	assert.Equal(t, firstResult.AuditEntryCount, secondResult.AuditEntryCount)
	assert.Equal(t, firstResult.ContentSHA256, secondResult.ContentSHA256)

	firstDigest, err := treeDigest(first)
	require.NoError(t, err)
	secondDigest, err := treeDigest(second)
	require.NoError(t, err)
	assert.Equal(t, firstDigest, secondDigest)
	t.Logf("content SHA-256: %s; complete tree SHA-256: %s", firstResult.ContentSHA256, firstDigest)

	_, err = Extract(Options{DataDir: dataDir, Output: first})
	require.ErrorContains(t, err, "already exists; use --force")
	forcedResult, err := Extract(Options{DataDir: dataDir, Output: first, Force: true})
	require.NoError(t, err)
	assert.Equal(t, firstResult.ContentSHA256, forcedResult.ContentSHA256)
	forcedDigest, err := treeDigest(first)
	require.NoError(t, err)
	assert.Equal(t, firstDigest, forcedDigest)

	manifest, err := LoadManifest(filepath.Join(first, manifestFilename))
	require.NoError(t, err)
	require.Len(t, manifest.ActualFiles, totalConversationCount)
	assert.Equal(t, firstResult.AuditEntryCount, manifest.AuditEntryCount)
	audit, err := LoadAudit(filepath.Join(first, auditFilename))
	require.NoError(t, err)
	assert.Empty(t, audit.Entries)
	for _, name := range manifest.ActualFiles {
		_, err := semantic.LoadFile(filepath.Join(first, filepath.FromSlash(name)))
		require.NoError(t, err, name)
	}
	// Windows has no Unix permission bits; Go reports every file there as 0666.
	if runtime.GOOS == "windows" {
		return
	}
	auditInfo, err := os.Stat(filepath.Join(first, auditFilename))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(filePermissions), auditInfo.Mode().Perm())
	for _, name := range manifest.ActualFiles {
		info, err := os.Stat(filepath.Join(first, filepath.FromSlash(name)))
		require.NoError(t, err)
		assert.Equal(t, fs.FileMode(filePermissions), info.Mode().Perm())
	}
	rootInfo, err := os.Stat(first)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(directoryPermissions), rootInfo.Mode().Perm())
}

func TestSyntheticImportMatchesExtraction(t *testing.T) {
	dataDir := syntheticDataDirectory(t)
	imported, err := Import(dataDir)
	require.NoError(t, err)
	require.Len(t, imported.Conversations, totalConversationCount)
	assert.Zero(t, imported.AuditEntryCount)
	assert.Equal(t, "castle/001.yaml", imported.Conversations[0].Path)
	assert.Equal(t, "original.castle.001", imported.Conversations[0].Conversation.ID)
	assert.Equal(t, "keep/032.yaml", imported.Conversations[len(imported.Conversations)-1].Path)

	for _, document := range imported.Conversations {
		canonical, marshalErr := semantic.Marshal(document.Conversation)
		require.NoError(t, marshalErr)
		assert.Equal(t, document.Data, canonical, document.Path)
	}
}

func TestOriginalDataExtractionIsDeterministic(t *testing.T) {
	dataDir := originaldata.Directory(t)
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")

	firstResult, err := Extract(Options{DataDir: dataDir, Output: first})
	require.NoError(t, err)
	secondResult, err := Extract(Options{DataDir: dataDir, Output: second})
	require.NoError(t, err)
	assert.Equal(t, totalConversationCount, firstResult.ConversationCount)
	assert.Equal(t, firstResult.ConversationCount, secondResult.ConversationCount)
	assert.Equal(t, firstResult.AuditEntryCount, secondResult.AuditEntryCount)
	assert.Equal(t, firstResult.ContentSHA256, secondResult.ContentSHA256)
	firstDigest, err := treeDigest(first)
	require.NoError(t, err)
	secondDigest, err := treeDigest(second)
	require.NoError(t, err)
	assert.Equal(t, firstDigest, secondDigest)
	t.Logf(
		"content SHA-256: %s; complete tree SHA-256: %s; audit entries: %d",
		firstResult.ContentSHA256,
		firstDigest,
		firstResult.AuditEntryCount,
	)
}

func TestOriginalWeblockUnreachablePairPreservesReachableTopic(t *testing.T) {
	dataDir := originaldata.Directory(t)
	weblock, auditEntries := loadOriginalConversation(
		t,
		dataDir,
		"CASTLE.TLK",
		castleConversationCount,
		14,
		semantic.OriginalSourceCastle,
	)
	require.Len(t, auditEntries, 1)
	entry := auditEntries[0]
	assert.Equal(t, 0x1b39, entry.KeyOffset)
	assert.Equal(t, AuditReasonUnreachableMatchKey, entry.Reason)
	assert.Equal(t, "87", entry.KeyPrefixHex)

	var gornResponse semantic.Body
	for _, topic := range weblock.Topics {
		for _, keyword := range topic.Keywords {
			if keyword == "gorn" {
				gornResponse = topic.Response
			}
		}
	}
	require.NotNil(t, gornResponse, "Weblock's reachable gorn topic must remain in runtime YAML")
	assert.Contains(t, strings.ToLower(semanticBodyText(gornResponse)), "hass")
}

func TestOriginalMarioMalformedTopicsPreserveReachableBehavior(t *testing.T) {
	dataDir := originaldata.Directory(t)
	mario, auditEntries := loadOriginalConversation(
		t,
		dataDir,
		"TOWNE.TLK",
		towneConversationCount,
		23,
		semantic.OriginalSourceTowne,
	)
	require.Len(t, auditEntries, 3)
	assert.Equal(t, []int{0x3635, 0x366d, 0x367f}, []int{
		auditEntries[0].KeyOffset,
		auditEntries[1].KeyOffset,
		auditEntries[2].KeyOffset,
	})
	assert.Equal(t, AuditReasonUnreachableMatchKey, auditEntries[0].Reason)
	assert.Equal(t, AuditReasonUnreachableMatchKey, auditEntries[1].Reason)
	assert.Equal(t, AuditReasonUnrenderableControlOutput, auditEntries[2].Reason)
	assert.Equal(t, "90", auditEntries[2].KeyPrefixHex)

	topics := make(map[string]semantic.Body)
	for _, topic := range mario.Topics {
		for _, keyword := range topic.Keywords {
			topics[keyword] = topic.Response
		}
	}
	assert.NotContains(t, topics, "caug")
	assert.NotContains(t, topics, "tort")
	crim, exists := topics["crim"]
	require.True(t, exists)
	assert.True(t, strings.HasSuffix(strings.ToLower(semanticBodyText(crim)), "charity!"))
	dontAsk, exists := topics["Don't ask!"]
	require.True(t, exists)
	require.Len(t, dontAsk, 1)
	ask, ok := dontAsk[0].(*semantic.AskAction)
	require.True(t, ok)
	assert.Equal(t, "question_01", ask.Question)
}

func TestOriginalDwellingTerminalOrphanAnswerIsAudited(t *testing.T) {
	dataDir := originaldata.Directory(t)
	conversation, auditEntries := loadOriginalConversation(
		t,
		dataDir,
		"DWELLING.TLK",
		dwellingConversationCount,
		15,
		semantic.OriginalSourceDwelling,
	)
	question, exists := conversation.Questions["question_03"]
	require.True(t, exists)
	assert.Empty(t, question.Answers)
	assert.NotEmpty(t, question.Default)

	var found bool
	for _, entry := range auditEntries {
		if entry.Reason == AuditReasonUnreachableOrphanAnswerKey && entry.KeyOffset == 0x1f0d {
			found = true
			assert.Equal(t, 26, entry.KeyLength)
			assert.Equal(t, 0x1f28, entry.ResponseOffset)
		}
	}
	assert.True(t, found, "DWELLING.TLK conversation 15 terminal orphan answer key must be audited")
}

func TestOriginalBalinorDescriptionPreservesQuestionBoundary(t *testing.T) {
	dataDir := originaldata.Directory(t)
	balinor, _ := loadOriginalConversation(
		t,
		dataDir,
		"KEEP.TLK",
		keepConversationCount,
		10,
		semantic.OriginalSourceKeep,
	)
	require.Len(t, balinor.Description, 2)
	description, ok := balinor.Description[0].(*semantic.NarrateAction)
	require.True(t, ok)
	require.Len(t, description.Content, 3)
	descriptionText, ok := description.Content[0].(*semantic.TextPart)
	require.True(t, ok)
	assert.Equal(t, "a leather-winged abomination", descriptionText.Text)
	assert.IsType(t, &semantic.LineBreakPart{}, description.Content[1])
	assert.IsType(t, &semantic.LineBreakPart{}, description.Content[2])
	ask, ok := balinor.Description[1].(*semantic.AskAction)
	require.True(t, ok)
	assert.Equal(t, "question_01", ask.Question)
}

func loadOriginalConversation(
	t *testing.T,
	dataDir string,
	filename string,
	expectedCount int,
	conversationID int,
	source semantic.OriginalSource,
) (*semantic.Conversation, []AuditEntry) {
	t.Helper()

	tlkData, err := os.ReadFile(filepath.Join(dataDir, filename))
	require.NoError(t, err)
	dataOvl, err := os.ReadFile(filepath.Join(dataDir, "DATA.OVL"))
	require.NoError(t, err)
	dictionary, err := parseWordTable(dataOvl[compressedWordsOffset : compressedWordsOffset+compressedWordsLength])
	require.NoError(t, err)
	conversations, err := parseTLK(filename, tlkData, expectedCount)
	require.NoError(t, err)
	conversation, auditEntries, err := convertConversation(conversations[conversationID-1], source, dictionary)
	require.NoError(t, err)
	require.NoError(t, semantic.Validate(conversation))

	return conversation, auditEntries
}

func semanticBodyText(body semantic.Body) string {
	var builder strings.Builder
	for _, action := range body {
		var content semantic.RichText
		switch value := action.(type) {
		case *semantic.SayAction:
			content = value.Content
		case *semantic.NarrateAction:
			content = value.Content
		}
		for _, part := range content {
			switch value := part.(type) {
			case *semantic.TextPart:
				builder.WriteString(value.Text)
			case *semantic.RunicPart:
				builder.WriteString(value.Text)
			}
		}
	}

	return builder.String()
}

func syntheticDataDirectory(t *testing.T) string {
	t.Helper()

	directory := t.TempDir()
	dataOvl := make([]byte, compressedWordsOffset+compressedWordsLength)
	copy(dataOvl[compressedWordsOffset:], syntheticWordTable())
	require.NoError(t, os.WriteFile(filepath.Join(directory, "DATA.OVL"), dataOvl, filePermissions))
	for _, spec := range conversationSources() {
		ids := make([]int, spec.count)
		records := make([][]byte, spec.count)
		for index := range spec.count {
			ids[index] = index + 1
			record := fixedFields(
				fmt.Sprintf("Name %d", index+1),
				"An invented description.",
				"Greetings.",
				"I test extraction.",
				"Farewell.",
			)
			record = appendField(record, upper("test"))
			record = appendField(record, upper("Synthetic response."))
			records[index] = append(record, questionMarker, lastQuestionID, recordEndByte)
		}
		require.NoError(t, os.WriteFile(
			filepath.Join(directory, spec.filename),
			buildTLK(ids, records...),
			filePermissions,
		))
	}

	return directory
}

func treeDigest(root string) (string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	hasher := sha256.New()
	for _, filename := range files {
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return "", err
		}
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(relative)))
		if _, err := hasher.Write(length[:]); err != nil {
			return "", err
		}
		if _, err := hasher.Write([]byte(relative)); err != nil {
			return "", err
		}
		binary.BigEndian.PutUint64(length[:], uint64(len(data)))
		if _, err := hasher.Write(length[:]); err != nil {
			return "", err
		}
		if _, err := hasher.Write(data); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
