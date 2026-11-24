package catalog

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
)

// New builds an immutable catalog from semantic definitions. Original-origin
// definitions are also indexed by source and dialog.
func New(definitions ...*semantic.Conversation) (*Catalog, error) {
	catalog := &Catalog{
		originals: make(map[OriginalKey]*Program, len(definitions)),
		byID:      make(map[string]*Program, len(definitions)),
	}
	for _, definition := range definitions {
		data, err := semantic.Marshal(definition)
		if err != nil {
			return nil, fmt.Errorf("serializing semantic conversation: %w", err)
		}
		if _, exists := catalog.byID[definition.ID]; exists {
			return nil, fmt.Errorf("%w: %q", errDuplicateID, definition.ID)
		}
		program := &Program{id: definition.ID, data: data, patched: false}
		catalog.byID[definition.ID] = program
		origin, original := definition.Origin.(*semantic.OriginalOrigin)
		if !original {
			continue
		}
		key := OriginalKey{Source: origin.Source, Dialog: origin.Dialog}
		if _, exists := catalog.originals[key]; exists {
			return nil, fmt.Errorf("%w: %s/%d", errDuplicateOriginal, key.Source, key.Dialog)
		}
		catalog.originals[key] = program
	}

	return catalog, nil
}
