package commanderclient

import (
	"fmt"
	"time"

	"github.com/foomo/contentful"
)

// Publishing status constants
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusChanged   = "changed"
)

// Migration operation constants
const (
	OperationCreate    = "create"
	OperationUpsert    = "upsert"
	OperationUpdate    = "update"
	OperationDelete    = "delete"
	OperationPublish   = "publish"
	OperationUnpublish = "unpublish"
)

// Locale represents a Contentful locale code
type Locale string

// LocaleInfo represents information about a Contentful locale
type LocaleInfo struct {
	Code         Locale `json:"code"`
	Name         string `json:"name"`
	FallbackCode Locale `json:"fallbackCode"`
	Optional     bool   `json:"optional"`
	Default      bool   `json:"default"`
}

// Entity represents either a Contentful entry or asset
type Entity interface {
	// GetID returns the unique identifier of the entity
	GetID() string

	// GetType returns the type of entity ("Entry" or "Asset")
	GetType() string

	// GetContentType returns the content type ID for entries, empty string for assets
	GetContentType() string

	// GetCreatedAt returns the creation timestamp
	GetCreatedAt() time.Time

	// GetUpdatedAt returns the last update timestamp
	GetUpdatedAt() time.Time

	// GetVersion returns the current version number
	GetVersion() int

	// IsPublished returns true if the entity is published
	IsPublished() bool

	// GetPublishingStatus returns the publishing status of the entity
	GetPublishingStatus() string

	// GetFields returns the raw fields data (always locale maps)
	GetFields() map[string]any

	// GetFieldValue returns the value of a field for a specific locale
	GetFieldValue(fieldName string, locale Locale) any

	// GetFieldValueWithFallback returns the field value for the specified locale, falling back to defaultLocale if not found
	GetFieldValueWithFallback(fieldName string, locale Locale, defaultLocale Locale) any

	// GetFieldValueAsString returns the field value as string if found and is string type
	GetFieldValueAsString(fieldName string, locale Locale) string

	// GetFieldValueAsFloat64 returns the field value as float64 if found and is float64 type
	GetFieldValueAsFloat64(fieldName string, locale Locale) float64

	// GetFieldValueAsBool returns the field value as bool if found and is bool type
	GetFieldValueAsBool(fieldName string, locale Locale) bool

	// GetFieldValueAsReference unmarshals the field value into a contentful.Entry
	GetFieldValueAsReference(fieldName string, locale Locale) *contentful.Entry

	// GetFieldValueAsReferencedEntity returns the actual entity referenced by the field value
	GetFieldValueAsReferencedEntity(fieldName string, locale Locale) (Entity, bool)

	// GetFieldValueAsReferencedEntities returns a collection of entities referenced by the field value
	// Broken references are silently skipped and not included in the returned collection
	GetFieldValueAsReferencedEntities(fieldName string, locale Locale) *EntityCollection

	// GetFieldValueAsReferences returns a slice of contentful.Entry from the field value
	GetFieldValueAsReferences(fieldName string, locale Locale) []*contentful.Entry

	// GetFieldValueInto unmarshals the field value into a target variable using a pointer
	// Note: This method is primarily useful for entries with variable field structures
	GetFieldValueInto(fieldName string, locale Locale, target any) error

	// GetTitle returns the title of the entity for the specified locale
	GetTitle(locale Locale) string

	// GetDescription returns the description of the entity for the specified locale
	GetDescription(locale Locale) string

	// GetFile returns the file information of the entity for the specified locale
	GetFile(locale Locale) *contentful.File

	// SetFieldValue sets the value of a field for a specific locale
	SetFieldValue(fieldName string, locale Locale, value any)

	// GetSys returns the system metadata
	GetSys() *contentful.Sys

	// IsEntry returns true if this entity is an Entry
	IsEntry() bool

	// IsAsset returns true if this entity is an Asset
	IsAsset() bool
}

// EntryEntity wraps a Contentful entry
type EntryEntity struct {
	Entry  *contentful.Entry
	Client *MigrationClient
}

// AssetEntity wraps a Contentful asset
type AssetEntity struct {
	Asset  *contentful.Asset
	Client *MigrationClient
}

// EntityCollection represents a collection of entities with filtering capabilities
type EntityCollection struct {
	entities []Entity
	filters  []EntityFilter
}

// EntityFilter is a function that filters entities
type EntityFilter func(Entity) bool

// SpaceModel represents the structure of a Contentful space
type SpaceModel struct {
	SpaceID       string
	Environment   string
	Locales       []LocaleInfo
	DefaultLocale Locale
	ContentTypes  map[string]*contentful.ContentType
	Entries       map[string]Entity // ID -> Entity
	Assets        map[string]Entity // ID -> Entity
	LastUpdated   time.Time
}

// MigrationStats tracks migration statistics
type MigrationStats struct {
	TotalEntities    int
	ProcessedEntries int
	ProcessedAssets  int
	Errors           int
	StartTime        time.Time
	EndTime          time.Time
	Duration         time.Duration
}

// Printf returns a formatted string with migration statistics
func (ms *MigrationStats) Printf() string {
	ms.EndTime = time.Now()
	ms.Duration = ms.EndTime.Sub(ms.StartTime)
	return fmt.Sprintf("Completed: 📄 %d/%d entities processed (%d entries, %d assets) 🚨 %d errors ⏱️ duration: %v",
		ms.ProcessedEntries+ms.ProcessedAssets, ms.TotalEntities, ms.ProcessedEntries, ms.ProcessedAssets, ms.Errors, ms.Duration)
}

// MigrationOptions configures migration behavior
type MigrationOptions struct {
	DryRun bool
}

// CollectionStats provides statistics about a collection
type CollectionStats struct {
	TotalCount             int
	EntryCount             int
	AssetCount             int
	ContentTypeCounts      map[string]int
	PublishingStatusCounts map[string]int
	TypeCounts             map[string]int
	OldestEntity           time.Time
	NewestEntity           time.Time
}

// DefaultMigrationOptions returns sensible defaults
func DefaultMigrationOptions() *MigrationOptions {
	return &MigrationOptions{
		DryRun: true,
	}
}

// Locale utility functions

// IsValidLocale checks if a locale code is valid
func (l Locale) IsValid() bool {
	return l != ""
}

// String returns the string representation of the locale
func (l Locale) String() string {
	return string(l)
}

// GetDefaultLocale returns the default locale from a list of locales
func GetDefaultLocale(locales []LocaleInfo) Locale {
	for _, locale := range locales {
		if locale.Default {
			return locale.Code
		}
	}
	// Fallback to first locale if no default is set
	if len(locales) > 0 {
		return locales[0].Code
	}
	return ""
}

// GetLocaleCodes returns all locale codes from locale info
func GetLocaleCodes(locales []LocaleInfo) []Locale {
	codes := make([]Locale, len(locales))
	for i, locale := range locales {
		codes[i] = locale.Code
	}
	return codes
}
