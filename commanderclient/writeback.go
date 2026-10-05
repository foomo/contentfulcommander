package commanderclient

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/foomo/contentful"
)

// CMA writes and the cache lock
//
// The SDK decodes CMA responses into the struct it is given, and the version-conflict
// retry rewrites Sys. Doing that to a cached entity outside cacheMu would race with
// reverse queries and the reference index, which read cached entities under the lock.
// writeDetached therefore runs the write against a deep copy taken under the read lock
// and applies the result back under the write lock, re-extracting the reference edges in
// the same critical section.

// writeDetached runs write against a detached copy of entity (passed to write) and then
// applies the copy's Sys — and, when applyResponse is set, its fields and metadata — to
// entity under the cache lock. Set applyResponse for writes whose CMA response the SDK
// decodes into the struct (upserts, asset publish/unpublish); without it only the
// version refreshed by a conflict retry is carried back.
func (me *MigrationExecutor) writeDetached(ctx context.Context, entity Entity, applyResponse bool, write func(work Entity) error) error {
	work, err := me.client.detachEntity(entity)
	if err != nil {
		return err
	}
	if err := me.writeWithVersionRetry(ctx, work, func() error { return write(work) }); err != nil {
		return err
	}
	me.client.applyDetached(entity, work, applyResponse)
	return nil
}

// detachEntity returns a deep copy of the entity's Contentful data, sharing nothing the
// SDK writes into.
func (mc *MigrationClient) detachEntity(entity Entity) (Entity, error) {
	mc.cacheMu.RLock()
	defer mc.cacheMu.RUnlock()
	switch e := entity.(type) {
	case *EntryEntity:
		return &EntryEntity{Entry: &contentful.Entry{
			Sys:      cloneSys(e.Entry.Sys),
			Metadata: cloneMetadata(e.Entry.Metadata),
			Fields:   cloneFields(e.Entry.Fields),
		}}, nil
	case *AssetEntity:
		fields, err := cloneFileFields(e.Asset.Fields)
		if err != nil {
			return nil, err
		}
		return &AssetEntity{Asset: &contentful.Asset{
			Sys:      cloneSys(e.Asset.Sys),
			Metadata: cloneMetadata(e.Asset.Metadata),
			Fields:   fields,
		}}, nil
	default:
		return nil, fmt.Errorf("unsupported entity type %T", entity)
	}
}

// applyDetached copies the written copy back into entity, keeping the identity of its
// Sys struct and field map, and re-extracts its reference edges when it is cached.
//
// Only values that changed are written. Callers read entities outside the lock (every
// caller reads IDs of query results), so identity data that a write never changes — ID,
// type, content type, creation time — must never be written, and a save that returns
// the fields it sent leaves the field map untouched.
func (mc *MigrationClient) applyDetached(entity, work Entity, applyResponse bool) {
	mc.cacheMu.Lock()
	defer mc.cacheMu.Unlock()
	switch e := entity.(type) {
	case *EntryEntity:
		written := work.(*EntryEntity).Entry
		setChangedFields(e.Entry.Sys, written.Sys)
		if applyResponse {
			if !reflect.DeepEqual(e.Entry.Metadata, written.Metadata) {
				e.Entry.Metadata = written.Metadata
			}
			// The response was decoded into the copy, so it holds every field sent plus
			// every field returned, as the cached map would have after an in-place decode.
			if e.Entry.Fields == nil {
				e.Entry.Fields = written.Fields
			}
			for name, value := range written.Fields {
				if current, ok := e.Entry.Fields[name]; !ok || !reflect.DeepEqual(current, value) {
					e.Entry.Fields[name] = value
				}
			}
		}
	case *AssetEntity:
		written := work.(*AssetEntity).Asset
		setChangedFields(e.Asset.Sys, written.Sys)
		if applyResponse {
			if !reflect.DeepEqual(e.Asset.Metadata, written.Metadata) {
				e.Asset.Metadata = written.Metadata
			}
			if e.Asset.Fields == nil || written.Fields == nil {
				if e.Asset.Fields != written.Fields {
					e.Asset.Fields = written.Fields
				}
			} else {
				setChangedFields(e.Asset.Fields, written.Fields)
			}
		}
	}
	mc.reindexIfCachedLocked(entity)
}

// setChangedFields sets the fields of dst that differ from src, leaving equal fields
// unwritten.
func setChangedFields[T any](dst, src *T) {
	dstValue, srcValue := reflect.ValueOf(dst).Elem(), reflect.ValueOf(src).Elem()
	for i := range dstValue.NumField() {
		if !reflect.DeepEqual(dstValue.Field(i).Interface(), srcValue.Field(i).Interface()) {
			dstValue.Field(i).Set(srcValue.Field(i))
		}
	}
}

// cloneSys deep-copies sys. Every pointer field is cloned (TestCloneSysCoversPointerFields
// fails when the SDK adds one), so decoding into the copy never writes into the original.
func cloneSys(sys *contentful.Sys) *contentful.Sys {
	if sys == nil {
		return nil
	}
	clone := *sys
	clone.UpdatedBy = cloneSys(sys.UpdatedBy)
	clone.ArchivedBy = cloneSys(sys.ArchivedBy)
	clone.PublishedBy = cloneSys(sys.PublishedBy)
	if sys.ContentType != nil {
		contentType := *sys.ContentType
		contentType.Sys = cloneSys(sys.ContentType.Sys)
		clone.ContentType = &contentType
	}
	if sys.Space != nil {
		space := *sys.Space
		space.Sys = cloneSys(sys.Space.Sys)
		clone.Space = &space
	}
	return &clone
}

func cloneMetadata(metadata *contentful.Metadata) *contentful.Metadata {
	if metadata == nil {
		return nil
	}
	clone := &contentful.Metadata{}
	if metadata.Tags != nil {
		clone.Tags = make([]contentful.Tag, len(metadata.Tags))
		for i, tag := range metadata.Tags {
			tag.Sys = cloneSys(tag.Sys)
			clone.Tags[i] = tag
		}
	}
	return clone
}

// cloneFields deep-copies the JSON containers (maps and arrays) of entry fields. Other
// values are shared: the SDK only reads them, and decoding replaces them.
func cloneFields(fields map[string]any) map[string]any {
	if fields == nil {
		return nil
	}
	clone := make(map[string]any, len(fields))
	for name, value := range fields {
		clone[name] = cloneFieldValue(value)
	}
	return clone
}

func cloneFieldValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneFields(v)
	case []any:
		if v == nil {
			return v
		}
		clone := make([]any, len(v))
		for i, item := range v {
			clone[i] = cloneFieldValue(item)
		}
		return clone
	default:
		return value
	}
}

// cloneFileFields deep-copies asset fields through JSON: they hold strings, numbers and
// links only, so the round trip is exact.
func cloneFileFields(fields *contentful.FileFields) (*contentful.FileFields, error) {
	if fields == nil {
		return nil, nil
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("failed to copy asset fields: %w", err)
	}
	var clone contentful.FileFields
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, fmt.Errorf("failed to copy asset fields: %w", err)
	}
	return &clone, nil
}
