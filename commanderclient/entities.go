package commanderclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/foomo/contentful"
)

var (
	// ErrCircularReference is returned when a cycle is detected while walking the referrer path.
	ErrCircularReference = errors.New("circular reference detected in referrer path")
	// ErrAmbiguousPath is returned when multiple referrers are found at one step of the referrer path.
	ErrAmbiguousPath = errors.New("ambiguous referrer path: multiple referrers found at one step")
)

// PathOption configures the behavior of GetReferrerPath.
type PathOption func(*pathConfig)

type pathConfig struct {
	stopAtEntityID    string
	stopAtContentType string
}

// StopAtEntityID makes GetReferrerPath stop walking when the entity with the given ID is reached.
func StopAtEntityID(id string) PathOption {
	return func(cfg *pathConfig) {
		cfg.stopAtEntityID = id
	}
}

// StopAtContentType makes GetReferrerPath stop walking when an entity with the given content type is reached.
func StopAtContentType(ct string) PathOption {
	return func(cfg *pathConfig) {
		cfg.stopAtContentType = ct
	}
}

// getReferrerPath walks up the referrer chain through the specified fields and returns the path from root to self.
func getReferrerPath(client *MigrationClient, startEntity Entity, fieldNames []string, opts ...PathOption) ([]Entity, error) {
	if client == nil || len(fieldNames) == 0 {
		return []Entity{startEntity}, nil
	}

	var cfg pathConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	path := []Entity{startEntity}

	var err error
	client.withReferenceIndex(func(idx *referenceIndex) {
		// Entity data is read only under the cache lock, where CMA writes are applied.
		currentID := startEntity.GetID()
		visited := map[string]struct{}{currentID: {}}
		for {
			var referrer Entity
			for referrerID, slots := range idx.incoming[currentID] {
				entity, ok := client.cache[referrerID]
				if !ok || entity.GetID() == currentID || !slotsInFields(slots, fieldNames) {
					continue
				}
				if referrer != nil {
					err = ErrAmbiguousPath
					return
				}
				referrer = entity
			}
			if referrer == nil {
				return
			}

			referrerID := referrer.GetID()
			if _, seen := visited[referrerID]; seen {
				err = ErrCircularReference
				return
			}

			visited[referrerID] = struct{}{}
			path = append(path, referrer)

			if cfg.stopAtEntityID != "" && referrerID == cfg.stopAtEntityID {
				return
			}
			if cfg.stopAtContentType != "" && referrer.GetContentType() == cfg.stopAtContentType {
				return
			}

			currentID = referrerID
		}
	})
	if err != nil {
		return nil, err
	}

	slices.Reverse(path)
	return path, nil
}

func isNullOrEmpty(value any) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case string:
		return v == ""
	case map[string]any:
		return len(v) == 0
	case []any:
		return len(v) == 0
	}
	return false
}

// IsArchived reports whether e is an archived entry or asset. It is nil-safe and, for
// Entity implementations other than *EntryEntity and *AssetEntity, reads GetSys().
func IsArchived(e Entity) bool {
	if isNilEntity(e) {
		return false
	}
	switch typed := e.(type) {
	case *EntryEntity:
		return typed.Entry != nil && typed.Entry.Sys != nil && typed.IsArchived()
	case *AssetEntity:
		return typed.Asset != nil && typed.Asset.Sys != nil && typed.IsArchived()
	default:
		sys := e.GetSys()
		return sys != nil && sys.ArchivedVersion > 0
	}
}

// EntryEntity implementation

func (ee *EntryEntity) GetID() string {
	return ee.Entry.Sys.ID
}

func (ee *EntryEntity) GetType() string {
	return "Entry"
}

func (ee *EntryEntity) GetContentType() string {
	return ee.Entry.Sys.ContentType.Sys.ID
}

func (ee *EntryEntity) GetCreatedAt() time.Time {
	// Parse the ISO 8601 timestamp
	if t, err := time.Parse(time.RFC3339, ee.Entry.Sys.CreatedAt); err == nil {
		return t
	}
	return time.Time{}
}

func (ee *EntryEntity) GetUpdatedAt() time.Time {
	// Parse the ISO 8601 timestamp
	if t, err := time.Parse(time.RFC3339, ee.Entry.Sys.UpdatedAt); err == nil {
		return t
	}
	return time.Time{}
}

func (ee *EntryEntity) GetVersion() int {
	return ee.Entry.Sys.Version
}

func (ee *EntryEntity) IsPublished() bool {
	return ee.Entry.Sys.PublishedVersion > 0 && ee.Entry.Sys.Version-ee.Entry.Sys.PublishedVersion == 1
}

// IsArchived returns true if the entry is archived (Sys.ArchivedVersion > 0).
func (ee *EntryEntity) IsArchived() bool {
	return ee.Entry.Sys.ArchivedVersion > 0
}

// GetPublishingStatus returns StatusArchived, StatusDraft, StatusPublished or
// StatusChanged. Archived takes precedence over the other statuses.
func (ee *EntryEntity) GetPublishingStatus() string {
	if ee.IsArchived() {
		return StatusArchived
	}
	if ee.Entry.Sys.PublishedVersion == 0 {
		return StatusDraft
	}
	if ee.IsPublished() {
		return StatusPublished
	}
	return StatusChanged
}

func (ee *EntryEntity) GetFields() map[string]any {
	return ee.Entry.Fields
}

func (ee *EntryEntity) GetFieldValue(fieldName string, locale Locale) any {
	if fields := ee.Entry.Fields; fields != nil {
		if fieldValue, exists := fields[fieldName]; exists {
			if fieldMap, ok := fieldValue.(map[string]any); ok {
				if value, exists := fieldMap[string(locale)]; exists {
					return value
				}
			}
		}
	}
	return nil
}

func (ee *EntryEntity) GetFieldValueWithFallback(fieldName string, locale Locale, defaultLocale Locale) any {
	value := ee.GetFieldValue(fieldName, locale)
	if value != nil {
		return value
	}
	return ee.GetFieldValue(fieldName, defaultLocale)
}

func (ee *EntryEntity) GetFieldValueInto(fieldName string, locale Locale, target any) error {
	value := ee.GetFieldValue(fieldName, locale)
	if value == nil {
		return fmt.Errorf("field '%s' not found for locale '%s'", fieldName, locale)
	}

	// Use JSON marshaling/unmarshaling for type conversion
	jsonData, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal field value: %w", err)
	}

	if err := json.Unmarshal(jsonData, target); err != nil {
		return fmt.Errorf("failed to unmarshal into target: %w", err)
	}

	return nil
}

func (ee *EntryEntity) GetTitle(locale Locale) string {
	// Get the content type to find the display field
	contentTypeID := ee.GetContentType()
	if contentTypeID == "" {
		return ""
	}

	// Access the space model to get the content type definition
	// Note: This assumes the client has access to the space model
	// In a real implementation, you might need to pass the space model or content type info
	if ee.Client != nil && ee.Client.spaceModel != nil {
		if contentType, exists := ee.Client.spaceModel.ContentTypes[contentTypeID]; exists {
			if contentType.DisplayField != "" {
				return ee.GetFieldValueAsString(contentType.DisplayField, locale)
			}
		}
	}

	return ""
}

func (ee *EntryEntity) GetDescription(locale Locale) string {
	return "" // Entries don't have a standard description field
}

func (ee *EntryEntity) GetFile(locale Locale) *contentful.File {
	return nil // Entries don't have a standard file field
}

func (ee *EntryEntity) GetFieldValueAsString(fieldName string, locale Locale) string {
	value := ee.GetFieldValue(fieldName, locale)
	if strValue, ok := value.(string); ok {
		return strValue
	}
	return ""
}

func (ee *EntryEntity) GetFieldValueAsFloat64(fieldName string, locale Locale) float64 {
	value := ee.GetFieldValue(fieldName, locale)
	if floatValue, ok := value.(float64); ok {
		return floatValue
	}
	return 0.0
}

func (ee *EntryEntity) GetFieldValueAsBool(fieldName string, locale Locale) bool {
	value := ee.GetFieldValue(fieldName, locale)
	if boolValue, ok := value.(bool); ok {
		return boolValue
	}
	return false
}

func (ee *EntryEntity) GetFieldValueAsReferencedEntity(fieldName string, locale Locale) (Entity, bool) {
	reference := ee.GetFieldValueAsReference(fieldName, locale)
	if reference == nil || reference.Sys == nil {
		return nil, false
	}

	// Use the client to get the actual entity
	return ee.Client.GetEntity(reference.Sys.ID)
}

func (ee *EntryEntity) GetFieldValueAsReferencedEntities(fieldName string, locale Locale) *EntityCollection {
	references := ee.GetFieldValueAsReferences(fieldName, locale)
	if references == nil {
		return NewEntityCollection([]Entity{})
	}

	var entities []Entity
	for _, reference := range references {
		if reference != nil && reference.Sys != nil {
			if entity, found := ee.Client.GetEntity(reference.Sys.ID); found {
				entities = append(entities, entity)
			}
			// Silently skip broken references - they won't be added to the collection
		}
	}

	return NewEntityCollection(entities)
}

func (ee *EntryEntity) GetFieldValueAsReference(fieldName string, locale Locale) *contentful.Entry {
	value := ee.GetFieldValue(fieldName, locale)
	if value == nil {
		return nil
	}
	return ee.convertToReference(value)
}

func (ee *EntryEntity) GetFieldValueAsReferences(fieldName string, locale Locale) []*contentful.Entry {
	value := ee.GetFieldValue(fieldName, locale)
	if value == nil {
		return nil
	}

	var entries []*contentful.Entry

	// Handle slice of references
	if sliceValue, ok := value.([]any); ok {
		for _, item := range sliceValue {
			if entry := ee.convertToReference(item); entry != nil {
				entries = append(entries, entry)
			}
		}
	} else if singleEntry := ee.convertToReference(value); singleEntry != nil {
		// Single reference
		entries = append(entries, singleEntry)
	}

	return entries
}

// Helper method to convert any value to contentful.Entry
func (ee *EntryEntity) convertToReference(value any) *contentful.Entry {
	switch v := value.(type) {
	case map[string]any:
		entry := &contentful.Entry{}
		if sysData, ok := v["sys"].(map[string]any); ok {
			if id, ok := sysData["id"].(string); ok {
				if entryType, ok := sysData["type"].(string); ok {
					entry.Sys = &contentful.Sys{
						ID:       id,
						LinkType: entryType,
						Type:     "Link",
					}
					return entry
				}
			}
		}
	}
	return nil
}

func (ee *EntryEntity) IsFieldNullOrEmpty(fieldName string, locale Locale) bool {
	return isNullOrEmpty(ee.GetFieldValue(fieldName, locale))
}

// SetFieldValue sets the value of a field for a specific locale. When the entry is the
// cached one, its reference edges are re-extracted. The value is stored as given: later
// mutation of it is not tracked (see ReindexEntity).
func (ee *EntryEntity) SetFieldValue(fieldName string, locale Locale, value any) {
	if ee.Client == nil {
		ee.setFieldValue(fieldName, locale, value)
		return
	}
	// Mutate under the cache lock: the reference index reads cached entries' fields.
	ee.Client.cacheMu.Lock()
	defer ee.Client.cacheMu.Unlock()
	ee.setFieldValue(fieldName, locale, value)
	ee.Client.reindexIfCachedLocked(ee)
}

func (ee *EntryEntity) setFieldValue(fieldName string, locale Locale, value any) {
	if ee.Entry.Fields == nil {
		ee.Entry.Fields = make(map[string]any)
	}

	// Ensure the field exists as a locale map
	if _, exists := ee.Entry.Fields[fieldName]; !exists {
		ee.Entry.Fields[fieldName] = make(map[string]any)
	}

	if fieldMap, ok := ee.Entry.Fields[fieldName].(map[string]any); ok {
		fieldMap[string(locale)] = value
	} else {
		// Convert to locale map format
		fieldMap := make(map[string]any)
		fieldMap[string(locale)] = value
		ee.Entry.Fields[fieldName] = fieldMap
	}
}

func (ee *EntryEntity) GetSys() *contentful.Sys {
	return ee.Entry.Sys
}

func (ee *EntryEntity) IsEntry() bool {
	return true
}

func (ee *EntryEntity) IsAsset() bool {
	return false
}

func (ee *EntryEntity) HasCDAView() bool {
	return ee.cdaView != nil
}

func (ee *EntryEntity) CDAView() Entity {
	return ee.cdaView
}

// GetReferrerPath walks up the referrer chain through the specified fields and returns the path from root to self.
func (ee *EntryEntity) GetReferrerPath(fieldNames []string, opts ...PathOption) ([]Entity, error) {
	return getReferrerPath(ee.Client, ee, fieldNames, opts...)
}

// GetParents returns all entities that reference this entry through any field and
// locale, excluding the entry itself, sorted by ID.
// If contentTypes is non-nil, only parents matching those content types are returned.
func (ee *EntryEntity) GetParents(contentTypes []string) *EntityCollection {
	return getParents(ee.Client, ee, nil, contentTypes)
}

// AssetEntity implementation

func (ae *AssetEntity) GetID() string {
	return ae.Asset.Sys.ID
}

func (ae *AssetEntity) GetType() string {
	return "Asset"
}

func (ae *AssetEntity) GetContentType() string {
	return "" // Assets don't have content types
}

func (ae *AssetEntity) GetCreatedAt() time.Time {
	// Parse the ISO 8601 timestamp
	if t, err := time.Parse(time.RFC3339, ae.Asset.Sys.CreatedAt); err == nil {
		return t
	}
	return time.Time{}
}

func (ae *AssetEntity) GetUpdatedAt() time.Time {
	// Parse the ISO 8601 timestamp
	if t, err := time.Parse(time.RFC3339, ae.Asset.Sys.UpdatedAt); err == nil {
		return t
	}
	return time.Time{}
}

func (ae *AssetEntity) GetVersion() int {
	return ae.Asset.Sys.Version
}

func (ae *AssetEntity) IsPublished() bool {
	return ae.Asset.Sys.PublishedVersion > 0 && ae.Asset.Sys.Version-ae.Asset.Sys.PublishedVersion == 1
}

// IsArchived returns true if the asset is archived (Sys.ArchivedVersion > 0).
func (ae *AssetEntity) IsArchived() bool {
	return ae.Asset.Sys.ArchivedVersion > 0
}

// GetPublishingStatus returns StatusArchived, StatusDraft, StatusPublished or
// StatusChanged. Archived takes precedence over the other statuses.
func (ae *AssetEntity) GetPublishingStatus() string {
	if ae.IsArchived() {
		return StatusArchived
	}
	if ae.Asset.Sys.PublishedVersion == 0 {
		return StatusDraft
	}
	if ae.IsPublished() {
		return StatusPublished
	}
	return StatusChanged
}

func (ae *AssetEntity) GetFields() map[string]any {
	// Convert asset fields to generic map with locale structure
	fields := make(map[string]any)
	if ae.Asset.Fields.Title != nil {
		fields["title"] = ae.Asset.Fields.Title
	}
	if ae.Asset.Fields.Description != nil {
		fields["description"] = ae.Asset.Fields.Description
	}
	if ae.Asset.Fields.File != nil {
		fields["file"] = ae.Asset.Fields.File
	}
	return fields
}

func (ae *AssetEntity) GetFieldValue(fieldName string, locale Locale) any {
	return nil // Assets don't support generic field access
}

func (ae *AssetEntity) GetFieldValueWithFallback(fieldName string, locale Locale, defaultLocale Locale) any {
	return nil // Assets don't support generic field access
}

func (ae *AssetEntity) GetFieldValueAsString(fieldName string, locale Locale) string {
	return "" // Assets don't support generic field access
}

func (ae *AssetEntity) GetFieldValueAsFloat64(fieldName string, locale Locale) float64 {
	return 0.0 // Assets don't support generic field access
}

func (ae *AssetEntity) GetFieldValueAsBool(fieldName string, locale Locale) bool {
	return false // Assets don't support generic field access
}

func (ae *AssetEntity) GetFieldValueAsReferencedEntity(fieldName string, locale Locale) (Entity, bool) {
	return nil, false // Assets don't support generic field access
}

func (ae *AssetEntity) GetFieldValueAsReferencedEntities(fieldName string, locale Locale) *EntityCollection {
	return NewEntityCollection([]Entity{}) // Assets don't support generic field access
}

func (ae *AssetEntity) GetFieldValueAsReference(fieldName string, locale Locale) *contentful.Entry {
	return nil // Assets don't support generic field access
}

func (ae *AssetEntity) GetFieldValueAsReferences(fieldName string, locale Locale) []*contentful.Entry {
	return nil // Assets don't support generic field access
}

func (ae *AssetEntity) GetFieldValueInto(fieldName string, locale Locale, target any) error {
	return fmt.Errorf("GetFieldValueInto is not supported for assets - assets have fixed field structure (title, description, file)")
}

func (ae *AssetEntity) GetTitle(locale Locale) string {
	if ae.Asset.Fields.Title != nil {
		if title, exists := ae.Asset.Fields.Title[string(locale)]; exists {
			return title
		}
	}
	return ""
}

func (ae *AssetEntity) GetDescription(locale Locale) string {
	if ae.Asset.Fields.Description != nil {
		if description, exists := ae.Asset.Fields.Description[string(locale)]; exists {
			return description
		}
	}
	return ""
}

func (ae *AssetEntity) GetFile(locale Locale) *contentful.File {
	if ae.Asset.Fields.File != nil {
		if file, exists := ae.Asset.Fields.File[string(locale)]; exists {
			return file
		}
	}
	return nil
}

func (ae *AssetEntity) IsFieldNullOrEmpty(fieldName string, locale Locale) bool {
	switch fieldName {
	case "title":
		return ae.GetTitle(locale) == ""
	case "description":
		return ae.GetDescription(locale) == ""
	case "file":
		return ae.GetFile(locale) == nil
	default:
		return true
	}
}

func (ae *AssetEntity) SetFieldValue(fieldName string, locale Locale, value any) {
	switch fieldName {
	case "title":
		if ae.Asset.Fields.Title == nil {
			ae.Asset.Fields.Title = make(map[string]string)
		}
		if strValue, ok := value.(string); ok {
			ae.Asset.Fields.Title[string(locale)] = strValue
		}
	case "description":
		if ae.Asset.Fields.Description == nil {
			ae.Asset.Fields.Description = make(map[string]string)
		}
		if strValue, ok := value.(string); ok {
			ae.Asset.Fields.Description[string(locale)] = strValue
		}
	case "file":
		// File field is typically not localized, but we'll store it for the specified locale
		if ae.Asset.Fields.File == nil {
			ae.Asset.Fields.File = make(map[string]*contentful.File)
		}
		// Note: File field handling would need more specific logic based on the file structure
	}
}

func (ae *AssetEntity) GetSys() *contentful.Sys {
	return ae.Asset.Sys
}

func (ae *AssetEntity) IsEntry() bool {
	return false
}

func (ae *AssetEntity) IsAsset() bool {
	return true
}

func (ae *AssetEntity) HasCDAView() bool {
	return ae.cdaView != nil
}

func (ae *AssetEntity) CDAView() Entity {
	return ae.cdaView
}

// GetReferrerPath walks up the referrer chain through the specified fields and returns the path from root to self.
func (ae *AssetEntity) GetReferrerPath(fieldNames []string, opts ...PathOption) ([]Entity, error) {
	return getReferrerPath(ae.Client, ae, fieldNames, opts...)
}

// GetParents returns all entities that reference this asset through any field and
// locale, excluding the asset itself, sorted by ID.
// If contentTypes is non-nil, only parents matching those content types are returned.
func (ae *AssetEntity) GetParents(contentTypes []string) *EntityCollection {
	return getParents(ae.Client, ae, nil, contentTypes)
}
