package save_snapshot

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func compileCurrentNativeSaveSchema(t *testing.T) (*jsonschema.Schema, any) {
	t.Helper()
	return compileNativeSaveSchemaFile(t, fmt.Sprintf("native-save-e2-s%d.schema.json", CurrentVersion))
}

func compileNativeSaveSchemaFile(t *testing.T, filename string) (*jsonschema.Schema, any) {
	t.Helper()
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() could not locate json_schema_test.go")
	}
	path := filepath.Join(filepath.Dir(testFile), "..", "..", "docs", "schemas", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read JSON schema %q: %v", path, err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse JSON schema %q: %v", path, err)
	}
	object, ok := document.(map[string]any)
	if !ok {
		t.Fatalf("JSON schema %q is not an object", path)
	}
	resource, ok := object["$id"].(string)
	if !ok || resource == "" {
		t.Fatalf("JSON schema %q has no $id", path)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	// Frozen e2 schemas form the $ref chain that the current schema reuses.
	var formatVersion int
	if _, scanErr := fmt.Sscanf(filename, "native-save-e2-s%d.schema.json", &formatVersion); scanErr != nil {
		t.Fatalf("schema filename %q has no snapshot version: %v", filename, scanErr)
	}
	registerJournalSchemaFormats(compiler, formatVersion)
	var dependencies []string
	for version := 16; version < formatVersion; version++ {
		dependencies = append(dependencies, fmt.Sprintf("native-save-e2-s%d.schema.json", version))
	}
	for _, dependency := range dependencies {
		frozenData, readErr := os.ReadFile(filepath.Join(filepath.Dir(path), dependency))
		if readErr != nil {
			t.Fatal(readErr)
		}
		frozen, parseErr := jsonschema.UnmarshalJSON(bytes.NewReader(frozenData))
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if addErr := compiler.AddResource(frozen.(map[string]any)["$id"].(string), frozen); addErr != nil {
			t.Fatal(addErr)
		}
	}
	if err = compiler.AddResource(resource, document); err != nil {
		t.Fatalf("add standalone JSON schema %q: %v", path, err)
	}
	compiled, err := compiler.Compile(resource)
	if err != nil {
		t.Fatalf("compile standalone JSON schema %q: %v", path, err)
	}

	return compiled, document
}

func walkJSONSchema(t *testing.T, value any, visitReference func(string)) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "$ref" {
				reference, ok := child.(string)
				if !ok {
					t.Fatalf("JSON schema contains non-string $ref %#v", child)
				}
				visitReference(reference)
			}
			walkJSONSchema(t, child, visitReference)
		}
	case []any:
		for _, child := range typed {
			walkJSONSchema(t, child, visitReference)
		}
	}
}

func decodeJSONSchemaInstance(t *testing.T, data []byte) any {
	t.Helper()
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse JSON schema instance: %v", err)
	}

	return instance
}
