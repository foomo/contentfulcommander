package commanderclient

import (
	"fmt"
	"strings"
	"time"
)

// NewEntityCollection creates a new entity collection from a slice of entities
func NewEntityCollection(entities []Entity) *EntityCollection {
	return &EntityCollection{
		entities: entities,
		filters:  []EntityFilter{},
	}
}

// Count returns the number of entities in the collection
func (ec *EntityCollection) Count() int {
	return len(ec.entities)
}

// Get returns all entities in the collection
func (ec *EntityCollection) Get() []Entity {
	return ec.entities
}

// GetByID returns an entity by ID, if it exists in the collection
func (ec *EntityCollection) GetByID(id string) (Entity, bool) {
	for _, entity := range ec.entities {
		if entity.GetID() == id {
			return entity, true
		}
	}
	return nil, false
}

// Filter applies additional filters to the collection
func (ec *EntityCollection) Filter(filters ...EntityFilter) *EntityCollection {
	var filtered []Entity

	for _, entity := range ec.entities {
		matches := true
		for _, filter := range filters {
			if !filter(entity) {
				matches = false
				break
			}
		}
		if matches {
			filtered = append(filtered, entity)
		}
	}

	return &EntityCollection{
		entities: filtered,
		filters:  append(ec.filters, filters...),
	}
}

// Limit returns a new collection with at most n entities
func (ec *EntityCollection) Limit(n int) *EntityCollection {
	if n >= len(ec.entities) {
		return ec
	}

	return &EntityCollection{
		entities: ec.entities[:n],
		filters:  ec.filters,
	}
}

// Skip returns a new collection skipping the first n entities
func (ec *EntityCollection) Skip(n int) *EntityCollection {
	if n >= len(ec.entities) {
		return &EntityCollection{
			entities: []Entity{},
			filters:  ec.filters,
		}
	}

	return &EntityCollection{
		entities: ec.entities[n:],
		filters:  ec.filters,
	}
}

// ForEach applies a function to each entity in the collection
func (ec *EntityCollection) ForEach(fn func(Entity)) {
	for _, entity := range ec.entities {
		fn(entity)
	}
}

// Transform applies a transformation function to each entity and returns a new collection
func (ec *EntityCollection) Transform(fn func(Entity) Entity) *EntityCollection {
	transformed := make([]Entity, len(ec.entities))
	for i, entity := range ec.entities {
		transformed[i] = fn(entity)
	}
	return &EntityCollection{
		entities: transformed,
		filters:  ec.filters,
	}
}

// ExtractIDs returns all entity IDs
func (ec *EntityCollection) ExtractIDs() []string {
	ids := make([]string, len(ec.entities))
	for i, entity := range ec.entities {
		ids[i] = entity.GetID()
	}
	return ids
}

// ExtractContentTypes returns all unique content types
func (ec *EntityCollection) ExtractContentTypes() []string {
	contentTypes := make(map[string]bool)
	for _, entity := range ec.entities {
		if entity.GetType() == "Entry" {
			contentTypes[entity.GetContentType()] = true
		}
	}

	result := make([]string, 0, len(contentTypes))
	for contentType := range contentTypes {
		result = append(result, contentType)
	}
	return result
}

// ExtractFields extracts a specific field from all entities
func (ec *EntityCollection) ExtractFields(fieldName string) []any {
	values := make([]any, 0, len(ec.entities))
	for _, entity := range ec.entities {
		if fields := entity.GetFields(); fields != nil {
			if value, exists := fields[fieldName]; exists {
				values = append(values, value)
			}
		}
	}
	return values
}

// ExtractFieldValues extracts a specific field from all entities for a specific locale
func (ec *EntityCollection) ExtractFieldValues(fieldName string, locale Locale) []any {
	values := make([]any, 0, len(ec.entities))
	for _, entity := range ec.entities {
		value := entity.GetFieldValue(fieldName, locale)
		if value != nil {
			values = append(values, value)
		}
	}
	return values
}

// ExtractFieldValuesWithFallback extracts a specific field from all entities for a specific locale with fallback
func (ec *EntityCollection) ExtractFieldValuesWithFallback(fieldName string, locale Locale, defaultLocale Locale) []any {
	values := make([]any, 0, len(ec.entities))
	for _, entity := range ec.entities {
		value := entity.GetFieldValueWithFallback(fieldName, locale, defaultLocale)
		if value != nil {
			values = append(values, value)
		}
	}
	return values
}

// GroupBy groups entities by a key function
func (ec *EntityCollection) GroupBy(keyFn func(Entity) string) map[string][]Entity {
	groups := make(map[string][]Entity)

	for _, entity := range ec.entities {
		key := keyFn(entity)
		groups[key] = append(groups[key], entity)
	}

	return groups
}

// GroupByContentType groups entities by content type
func (ec *EntityCollection) GroupByContentType() map[string]*EntityCollection {
	groups := make(map[string]*EntityCollection)

	for _, entity := range ec.entities {
		if entity.GetType() == "Entry" {
			contentType := entity.GetContentType()
			if groups[contentType] == nil {
				groups[contentType] = &EntityCollection{
					entities: []Entity{},
					filters:  ec.filters,
				}
			}
			groups[contentType].entities = append(groups[contentType].entities, entity)
		}
	}

	return groups
}

// GroupByPublishingStatus groups entities by publishing status
func (ec *EntityCollection) GroupByPublishingStatus() map[string]*EntityCollection {
	groups := make(map[string]*EntityCollection)

	for _, entity := range ec.entities {
		status := entity.GetPublishingStatus()
		if groups[status] == nil {
			groups[status] = &EntityCollection{
				entities: []Entity{},
				filters:  ec.filters,
			}
		}
		groups[status].entities = append(groups[status].entities, entity)
	}

	return groups
}

// CountByContentType returns counts by content type
func (ec *EntityCollection) CountByContentType() map[string]int {
	counts := make(map[string]int)

	for _, entity := range ec.entities {
		if entity.GetType() == "Entry" {
			contentType := entity.GetContentType()
			counts[contentType]++
		}
	}

	return counts
}

// CountByPublishingStatus returns counts by publishing status
func (ec *EntityCollection) CountByPublishingStatus() map[string]int {
	counts := make(map[string]int)

	for _, entity := range ec.entities {
		status := entity.GetPublishingStatus()
		counts[status]++
	}

	return counts
}

// GetStats returns comprehensive statistics about the collection
func (ec *EntityCollection) GetStats() *CollectionStats {
	stats := &CollectionStats{
		TotalCount:             len(ec.entities),
		ContentTypeCounts:      make(map[string]int),
		PublishingStatusCounts: make(map[string]int),
		TypeCounts:             make(map[string]int),
		OldestEntity:           time.Time{},
		NewestEntity:           time.Time{},
	}

	if len(ec.entities) == 0 {
		return stats
	}

	// Initialize with first entity's timestamps
	firstEntity := ec.entities[0]
	stats.OldestEntity = firstEntity.GetCreatedAt()
	stats.NewestEntity = firstEntity.GetCreatedAt()

	for _, entity := range ec.entities {
		// Count by type
		entityType := entity.GetType()
		stats.TypeCounts[entityType]++

		switch entityType {
		case "Entry":
			stats.EntryCount++
			// Count by content type
			contentType := entity.GetContentType()
			stats.ContentTypeCounts[contentType]++
		case "Asset":
			stats.AssetCount++
		}

		// Count by publishing status
		status := entity.GetPublishingStatus()
		stats.PublishingStatusCounts[status]++

		// Track oldest and newest
		createdAt := entity.GetCreatedAt()
		if createdAt.Before(stats.OldestEntity) {
			stats.OldestEntity = createdAt
		}
		if createdAt.After(stats.NewestEntity) {
			stats.NewestEntity = createdAt
		}
	}

	return stats
}

// ToMigrationOperations converts entities to migration operations
func (ec *EntityCollection) ToMigrationOperations(operation string) []MigrationOperation {
	operations := make([]MigrationOperation, len(ec.entities))
	for i, entity := range ec.entities {
		operations[i] = MigrationOperation{
			EntityID:  entity.GetID(),
			Operation: operation,
			Entity:    entity,
		}
	}
	return operations
}

// ToUpdateOperations creates update operations for all entities
func (ec *EntityCollection) ToUpdateOperations() []MigrationOperation {
	return ec.ToMigrationOperations(OperationUpdate)
}

// ToPublishOperations creates publish operations for all entities
func (ec *EntityCollection) ToPublishOperations() []MigrationOperation {
	return ec.ToMigrationOperations(OperationPublish)
}

// ToUnpublishOperations creates unpublish operations for all entities
func (ec *EntityCollection) ToUnpublishOperations() []MigrationOperation {
	return ec.ToMigrationOperations(OperationUnpublish)
}

// ToDeleteOperations creates delete operations for all entities
func (ec *EntityCollection) ToDeleteOperations() []MigrationOperation {
	return ec.ToMigrationOperations(OperationDelete)
}

// Common filter functions

// FilterByContentType returns a filter for specific content types
func FilterByContentType(contentTypes ...string) EntityFilter {
	return func(entity Entity) bool {
		if entity.GetType() != "Entry" {
			return false
		}

		entityContentType := entity.GetContentType()
		for _, contentType := range contentTypes {
			if entityContentType == contentType {
				return true
			}
		}
		return false
	}
}

// FilterByType returns a filter for entity types (Entry/Asset)
func FilterByType(entityType string) EntityFilter {
	return func(entity Entity) bool {
		return entity.GetType() == entityType
	}
}

// FilterPublished returns a filter for published entities
func FilterPublished() EntityFilter {
	return func(entity Entity) bool {
		return entity.IsPublished()
	}
}

// FilterDrafts returns a filter for draft entities
func FilterDrafts() EntityFilter {
	return func(entity Entity) bool {
		return !entity.IsPublished()
	}
}

// FilterByCreatedAfter returns a filter for entities created after a specific time
func FilterByCreatedAfter(t time.Time) EntityFilter {
	return func(entity Entity) bool {
		return entity.GetCreatedAt().After(t)
	}
}

// FilterByUpdatedAfter returns a filter for entities updated after a specific time
func FilterByUpdatedAfter(t time.Time) EntityFilter {
	return func(entity Entity) bool {
		return entity.GetUpdatedAt().After(t)
	}
}

// FilterByID returns a filter for entities matching an ID pattern
func FilterByID(entityID string) EntityFilter {
	return func(entity Entity) bool {
		return entity.GetID() == entityID
	}
}

// FilterByFieldValue returns a filter for entities with specific field values
func FilterByFieldValue(fieldName string, expectedValue any) EntityFilter {
	return func(entity Entity) bool {
		fields := entity.GetFields()
		if value, exists := fields[fieldName]; exists {
			return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", expectedValue)
		}
		return false
	}
}

// FilterByFieldValueWithLocale returns a filter for entities with specific field values for a locale
func FilterByFieldValueWithLocale(fieldName string, locale Locale, expectedValue any) EntityFilter {
	return func(entity Entity) bool {
		value := entity.GetFieldValue(fieldName, locale)
		if value != nil {
			return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", expectedValue)
		}
		return false
	}
}

// FilterByFieldValueWithFallback returns a filter for entities with specific field values using fallback locale
func FilterByFieldValueWithFallback(fieldName string, locale Locale, defaultLocale Locale, expectedValue any) EntityFilter {
	return func(entity Entity) bool {
		value := entity.GetFieldValueWithFallback(fieldName, locale, defaultLocale)
		if value != nil {
			return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", expectedValue)
		}
		return false
	}
}

// FilterByFieldExists returns a filter for entities that have a specific field
func FilterByFieldExists(fieldName string) EntityFilter {
	return func(entity Entity) bool {
		fields := entity.GetFields()
		_, exists := fields[fieldName]
		return exists
	}
}

// FilterByFieldContains returns a filter for entities where a field contains a substring
func FilterByFieldContains(fieldName, substring string) EntityFilter {
	return func(entity Entity) bool {
		fields := entity.GetFields()
		if value, exists := fields[fieldName]; exists {
			return strings.Contains(fmt.Sprintf("%v", value), substring)
		}
		return false
	}
}

// FilterByFieldContainsWithLocale returns a filter for entities where a field contains a substring for a specific locale
func FilterByFieldContainsWithLocale(fieldName string, locale Locale, substring string) EntityFilter {
	return func(entity Entity) bool {
		value := entity.GetFieldValue(fieldName, locale)
		if value != nil {
			return strings.Contains(fmt.Sprintf("%v", value), substring)
		}
		return false
	}
}

// FilterByFieldExistsWithLocale returns a filter for entities that have a specific field for a locale
func FilterByFieldExistsWithLocale(fieldName string, locale Locale) EntityFilter {
	return func(entity Entity) bool {
		value := entity.GetFieldValue(fieldName, locale)
		return value != nil
	}
}

// FilterByLocaleAvailability returns a filter for entities that have content in specific locales
func FilterByLocaleAvailability(requiredLocales []Locale) EntityFilter {
	return func(entity Entity) bool {
		fields := entity.GetFields()
		if len(fields) == 0 {
			return false
		}

		// Check if all required locales have at least one field with content
		for _, requiredLocale := range requiredLocales {
			hasContent := false
			for _, fieldValue := range fields {
				if fieldMap, ok := fieldValue.(map[string]any); ok {
					if _, exists := fieldMap[string(requiredLocale)]; exists {
						hasContent = true
						break
					}
				}
			}
			if !hasContent {
				return false
			}
		}
		return true
	}
}
