package commanderclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/foomo/contentful"
)

// CreateEntry creates a draft entry of contentType with the given fields and adds it
// to the cache, so GetEntity, filters and reverse queries see it right away. It never
// publishes: call Publish when the entry should go live.
//
// Pass an empty id to let Contentful generate one. Fields map field IDs to their
// values per locale, the shape SetFieldValue writes; nil creates an entry without
// fields. Contentful validates the field IDs and locales; required fields and other
// validations only apply when the entry is published.
//
// Creating an entry with an ID that already exists is an error — use SaveDraft to
// modify an existing entry.
func (mc *MigrationClient) CreateEntry(ctx context.Context, id, contentType string, fields map[string]map[Locale]any) (*EntryEntity, error) {
	if mc == nil {
		return nil, fmt.Errorf("migration client is nil")
	}
	if mc.cma == nil {
		return nil, fmt.Errorf("migration client has no CMA client")
	}
	if contentType == "" {
		return nil, fmt.Errorf("content type is required")
	}

	entryFields := make(map[string]any, len(fields))
	for fieldID, values := range fields {
		localized := make(map[string]any, len(values))
		for locale, value := range values {
			localized[string(locale)] = value
		}
		entryFields[fieldID] = localized
	}
	entry := &contentful.Entry{
		Sys: &contentful.Sys{
			ID:          id,
			ContentType: &contentful.ContentType{Sys: &contentful.Sys{ID: contentType}},
		},
		Fields: entryFields,
	}

	// Upsert POSTs without an ID and PUTs with one, and decodes the response into
	// entry, so it picks up the generated ID and the new version.
	if err := mc.cma.Entries.Upsert(ctx, mc.spaceID, entry); err != nil {
		var mismatch contentful.VersionMismatchError
		if errors.As(err, &mismatch) {
			return nil, fmt.Errorf("entry %q already exists", id)
		}
		return nil, fmt.Errorf("failed to create %s entry %q: %w", contentType, id, err)
	}

	entity := &EntryEntity{Entry: entry, Client: mc}

	mc.cacheMu.Lock()
	mc.cache[entity.GetID()] = entity
	if mc.spaceModel != nil {
		mc.spaceModel.Entries[entity.GetID()] = entity
	}
	mc.reindexLocked(entity.GetID())
	mc.cacheMu.Unlock()

	return entity, nil
}
