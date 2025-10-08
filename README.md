[![Build Status](https://github.com/foomo/contentfulcommander/actions/workflows/pr.yml/badge.svg?branch=main&event=push)](https://github.com/foomo/contentfulcommander/actions/workflows/pr.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/foomo/contentfulcommander)](https://goreportcard.com/report/github.com/foomo/contentfulcommander)
[![Coverage Status](https://coveralls.io/repos/github/foomo/contentfulcommander/badge.svg?branch=main&)](https://coveralls.io/github/foomo/contentfulcommander?branch=main)
[![GoDoc](https://godoc.org/github.com/foomo/contentfulcommander?status.svg)](https://godoc.org/github.com/foomo/contentfulcommander)

<p align="center">
  <img alt="sesamy" src=".github/assets/contentfulcommander.png"/>
</p>

# Contentful Commander

A Go library for Contentful migrations that provides a high-level interface for working with Contentful spaces, entities, and performing bulk operations.

## Features

- **Unified Entity Interface**: Work with both Contentful entries and assets through a common interface
- **Space Model Caching**: Load and cache entire space models for efficient operations
- **Locale-Aware Operations**: Native support for Contentful's localization system with locale-specific field access
- **Type-Safe Field Access**: Specialized methods for different field types (string, float64, bool, references)
- **Reference Resolution**: Direct access to referenced entities with automatic broken reference handling
- **Asset-Specific Methods**: Dedicated methods for asset title, description, and file access
- **Flexible Filtering**: Filter entities by content type, publication status, timestamps, and custom criteria
- **Collection Operations**: Chain operations like filtering, mapping, grouping, and reducing
- **Migration Execution**: Execute batch operations with dry-run support and comprehensive error handling
- **Configuration Management**: Load configuration from environment variables or `.contentfulrc.json` files
- **Portable Design**: Only depends on `github.com/foomo/contentful` and standard library

## Quick Start

```go
package main

import (
    "log"
    
    "github.com/bestbytes/globus/cmd/migrations/migrationlib"
)

func main() {
    // Load config from environment variables
    config := migrationlib.LoadConfigFromEnv()
    
    // Initialize ready-to-use client with logger and loaded space model
    client, logger, err := migrationlib.Init(config)
    if err != nil {
        log.Fatal(err)
    }
    
    // Filter entities
    entries := client.FilterEntities(
        migrationlib.FilterByContentType("product"),
        migrationlib.FilterPublished(),
    )
    
    // Process entities
    entries.ForEach(func(entity migrationlib.Entity) {
        logger.Info("Processing %s", entity.GetID())
    })
}
```

## Core Concepts

### Entity Interface

All Contentful entries and assets implement the `Entity` interface:

```go
type Entity interface {
    // Basic entity information
    GetID() string
    GetType() string          // "Entry" or "Asset"
    GetContentType() string   // Content type ID for entries
    GetCreatedAt() time.Time
    GetUpdatedAt() time.Time
    GetVersion() int
    IsPublished() bool
    GetPublishingStatus() string  // "draft", "published", or "changed"
    
    // Field access methods
    GetFields() map[string]any
    GetFieldValue(fieldName string, locale Locale) any
    GetFieldValueWithFallback(fieldName string, locale Locale, defaultLocale Locale) any
    
    // Type-safe field access (primarily for entries)
    GetFieldValueAsString(fieldName string, locale Locale) string
    GetFieldValueAsFloat64(fieldName string, locale Locale) float64
    GetFieldValueAsBool(fieldName string, locale Locale) bool
    
    // Reference handling
    GetFieldValueAsReference(fieldName string, locale Locale) *contentful.Entry
    GetFieldValueAsReferencedEntity(fieldName string, locale Locale) (Entity, bool)
    GetFieldValueAsReferences(fieldName string, locale Locale) []*contentful.Entry
    GetFieldValueAsReferencedEntities(fieldName string, locale Locale) *EntityCollection
    
    // Advanced field access
    GetFieldValueInto(fieldName string, locale Locale, target any) error
    
    // Entity-specific methods
    GetTitle(locale Locale) string
    GetDescription(locale Locale) string
    GetFile(locale Locale) *contentful.File
    
    // Utility methods
    SetFieldValue(fieldName string, locale Locale, value any)
    GetSys() *contentful.Sys
    GetNewFields() map[string]any
    IsEntry() bool
    IsAsset() bool
}
```

### Publishing Status

The library provides accurate publishing status detection based on Contentful's versioning system:

- **Draft**: `PublishedVersion == 0` (never been published)
- **Published**: `Version - PublishedVersion == 1` (current published version)
- **Changed**: `Version - PublishedVersion > 1` (has unpublished changes)

```go
entity := client.GetEntity("some-id")
status := entity.GetPublishingStatus() // "draft", "published", or "changed"
isPublished := entity.IsPublished()     // true only if status == "published"
```

The `MigrationClient` provides the main interface for working with Contentful spaces:

```go
// Initialize ready-to-use client
config := migrationlib.LoadConfigFromEnv()
client, logger, err := migrationlib.Init(config)
if err != nil {
    log.Fatal(err)
}

// Get entities (all return EntityCollection for consistency)
allEntities := client.GetAllEntities()
entries := client.GetEntries()
assets := client.GetAssets()
specificEntries := client.GetEntitiesByContentType("product")

// Filter entities
filtered := client.FilterEntities(
    migrationlib.FilterByContentType("product", "category"),
    migrationlib.FilterPublished(),
    migrationlib.FilterByUpdatedAfter(time.Now().AddDate(0, -1, 0)),
)
```

### Collection Operations

Collections provide powerful operations for working with groups of entities:

```go
collection := client.FilterEntities(filters...)

// Basic operations
count := collection.Count()
entities := collection.Get()
entity, exists := collection.GetByID("entity-id")

// Chaining operations
result := collection.
    Filter(migrationlib.FilterPublished()).
    Limit(100).
    Skip(50)

// Data extraction
ids := collection.ExtractIDs()
contentTypes := collection.ExtractContentTypes()
fieldValues := collection.ExtractFields("title")

// Grouping operations
contentTypeGroups := collection.GroupByContentType()
statusGroups := collection.GroupByPublishingStatus()
customGroups := collection.GroupBy(func(entity Entity) string {
    return entity.GetContentType()
})

// Counting operations
contentTypeCounts := collection.CountByContentType()
statusCounts := collection.CountByPublishingStatus()

// Statistics
stats := collection.GetStats()
fmt.Printf("Total: %d, Entries: %d, Assets: %d\n", 
    stats.TotalCount, stats.EntryCount, stats.AssetCount)

// Migration operations
updateOps := collection.ToUpdateOperations(map[string]any{
    "newField": "newValue",
})
publishOps := collection.ToPublishOperations()
deleteOps := collection.ToDeleteOperations()
```

## Field Access Methods

The library provides multiple ways to access field values, each optimized for different use cases:

### Basic Field Access

```go
// Get raw field value
value := entity.GetFieldValue("title", migrationlib.Locale("en"))

// Get field value with fallback to default locale
value := entity.GetFieldValueWithFallback("title", migrationlib.Locale("fr"), defaultLocale)
```

### Type-Safe Field Access

```go
// Get field as specific types (returns zero value if not found or wrong type)
title := entity.GetFieldValueAsString("title", migrationlib.Locale("en"))
price := entity.GetFieldValueAsFloat64("price", migrationlib.Locale("en"))
isActive := entity.GetFieldValueAsBool("isActive", migrationlib.Locale("en"))
```

### Reference Handling

```go
// Get reference as contentful.Entry
reference := entity.GetFieldValueAsReference("category", migrationlib.Locale("en"))

// Get actual referenced entity (resolves the reference)
if categoryEntity, found := entity.GetFieldValueAsReferencedEntity("category", migrationlib.Locale("en")); found {
    categoryTitle := categoryEntity.GetFieldValueAsString("title", migrationlib.Locale("en"))
}

// Get multiple references as slice
references := entity.GetFieldValueAsReferences("tags", migrationlib.Locale("en"))

// Get multiple referenced entities as collection (broken references automatically skipped)
tagEntities := entity.GetFieldValueAsReferencedEntities("tags", migrationlib.Locale("en"))
tagEntities.ForEach(func(tagEntity migrationlib.Entity) {
    fmt.Printf("Tag: %s\n", tagEntity.GetFieldValueAsString("name", migrationlib.Locale("en")))
})
```

### Advanced Field Access

```go
// Unmarshal field value directly into a struct (entries only). Useful for JSON/Object fields.
type Query struct {
    Operation string        `json:"operation"`
    Elements []ElementType  `json:"elements"`
}

var myQuery Query
if err := categoryEntity.GetFieldValueInto("catalogueQuery", migrationlib.Locale("en"), &myQuery); err != nil {
    log.Printf("Error: %v", err)
}
```

### Entity-Specific Methods

```go
// Get title (uses content type display field for entries, asset title for assets)
title := entity.GetTitle(migrationlib.Locale("en"))

// Get description (assets only, returns empty string for entries)
description := entity.GetDescription(migrationlib.Locale("en"))

// Get file information (assets only, returns nil for entries)
file := entity.GetFile(migrationlib.Locale("en"))
if file != nil {
    fmt.Printf("File: %s (%s)\n", file.Name, file.ContentType)
    fmt.Printf("URL: %s\n", file.URL)
}
```

## Locale Support

### Working with Locales

```go
// Get space locales
locales := client.GetLocales()
defaultLocale := client.GetDefaultLocale()

// Access field values for specific locales
entity := entries[0]
value := entity.GetFieldValue("title", migrationlib.Locale("en"))

// Access field values with fallback to default locale
value := entity.GetFieldValueWithFallback("title", migrationlib.Locale("fr"), defaultLocale)

// Set field values for specific locales
entity.SetFieldValue("title", migrationlib.Locale("de"), "Deutscher Titel")

// Get all fields (always locale maps)
fields := entity.GetFields()
```

### Locale-Aware Filtering

```go
// Filter by field value for a specific locale
englishEntries := client.FilterEntities(
    migrationlib.FilterByFieldValueWithLocale("title", migrationlib.Locale("en"), "Welcome"),
)

// Filter by field value with fallback to default locale
entriesWithWelcome := client.FilterEntities(
    migrationlib.FilterByFieldValueWithFallback("title", migrationlib.Locale("fr"), defaultLocale, "Welcome"),
)

// Filter by locale availability
multiLocaleEntries := client.FilterEntities(
    migrationlib.FilterByLocaleAvailability([]migrationlib.Locale{
        migrationlib.Locale("en"),
        migrationlib.Locale("de"),
    }),
)
```

### Locale-Aware Operations

```go
// Extract field values for a specific locale
collection := migrationlib.NewEntityCollection(entries)
englishTitles := collection.ExtractFieldValues("title", migrationlib.Locale("en"))

// Extract field values with fallback to default locale
frenchTitles := collection.ExtractFieldValuesWithFallback("title", migrationlib.Locale("fr"), defaultLocale)

// Create field updates for multiple locales
fields := map[string]any{
    "description": map[string]any{
        "en": "Updated description in English",
        "de": "Aktualisierte Beschreibung auf Deutsch",
    },
}

// Create migration operations
operations := collection.ToUpdateOperations(fields)
```

### Migration with Locale Targeting

```go
// Configure migration to target specific locales
options := migrationlib.DefaultMigrationOptions()
options.TargetLocales = []migrationlib.Locale{
    migrationlib.Locale("en"),
    migrationlib.Locale("de"),
}

executor := migrationlib.NewMigrationExecutor(client, options)
```

### Asset-Specific Usage

Assets have a fixed structure with only title, description, and file fields. The library provides dedicated methods for these:

```go
// Get all assets
assets := client.GetAssets()

// Access asset-specific fields
assets.ForEach(func(asset migrationlib.Entity) {
    // Get asset title for different locales
    titleEN := asset.GetTitle(migrationlib.Locale("en"))
    titleDE := asset.GetTitle(migrationlib.Locale("de"))
    
    // Get asset description
    description := asset.GetDescription(migrationlib.Locale("en"))
    
    // Get file information
    file := asset.GetFile(migrationlib.Locale("en"))
    if file != nil {
        fmt.Printf("Asset: %s\n", titleEN)
        fmt.Printf("File: %s (%s)\n", file.Name, file.ContentType)
        fmt.Printf("URL: %s\n", file.URL)
        if file.Detail != nil {
            fmt.Printf("Size: %d bytes\n", file.Detail.Size)
        }
    }
})

// Generic field methods return safe defaults for assets
value := asset.GetFieldValue("title", migrationlib.Locale("en")) // Returns nil
title := asset.GetFieldValueAsString("title", migrationlib.Locale("en")) // Returns ""
```

## Migration Operations

The library supports the following migration operations, each defined as a constant for type safety:

### Available Operations

```go
// Migration operation constants
const (
    OperationCreate    = "create"     // Create a new entity
    OperationUpsert     = "upsert"     // Create or update an entity
    OperationUpdate     = "update"     // Update an existing entity (preserves publishing status)
    OperationDelete     = "delete"     // Delete an entity
    OperationPublish    = "publish"    // Publish an entity
    OperationUnpublish  = "unpublish"  // Unpublish an entity
)
```

### Operation Details

- **`OperationCreate`**: Creates a new entity (not commonly used as entities are typically created through Contentful UI)
- **`OperationUpsert`**: Creates a new entity or updates an existing one with new fields
- **`OperationUpdate`**: Updates an existing entity with new fields and preserves its current publishing status (if published, it will be republished)
- **`OperationDelete`**: Permanently deletes an entity from Contentful
- **`OperationPublish`**: Publishes an entity (makes it available in the delivery API)
- **`OperationUnpublish`**: Unpublishes an entity (removes it from the delivery API but keeps it in the space)

### Usage Examples

Execute batch operations with comprehensive error handling:

```go
operations := []migrationlib.MigrationOperation{
    {
        EntityID:  "entity-id",
        Operation: migrationlib.OperationUpdate,
        Entity:    entity,
        NewFields: map[string]any{
            "newField": "newValue",
        },
    },
}

options := migrationlib.DefaultMigrationOptions()
options.DryRun = true

executor := migrationlib.NewMigrationExecutor(client, options)
results := executor.ExecuteBatch(ctx, operations)

// Check results
successCount := executor.GetSuccessCount()
errorCount := executor.GetErrorCount()
```

### Creating Different Types of Operations

```go
// Update operation (most common)
updateOp := &migrationlib.MigrationOperation{
    EntityID:  "product-123",
    Operation: migrationlib.OperationUpdate,
    Entity:    productEntity,
    NewFields: map[string]any{
        "title": map[string]any{
            "en": "Updated Product Title",
            "de": "Aktualisierter Produkttitel",
        },
    },
}

// Publish operation
publishOp := &migrationlib.MigrationOperation{
    EntityID:  "product-123",
    Operation: migrationlib.OperationPublish,
    Entity:    productEntity,
}

// Delete operation
deleteOp := &migrationlib.MigrationOperation{
    EntityID:  "old-product-456",
    Operation: migrationlib.OperationDelete,
    Entity:    oldProductEntity,
}

// Using collection methods to create operations
products := client.FilterEntities(
    migrationlib.FilterByContentType("product"),
    migrationlib.FilterDrafts(),
)

// Create update operations for all draft products
updateOps := products.ToUpdateOperations(map[string]any{
    "status": map[string]any{
        "en": "active",
    },
})

// Create publish operations for all products
publishOps := products.ToPublishOperations()

// Create delete operations for old products
oldProducts := client.FilterEntities(
    migrationlib.FilterByContentType("product"),
    migrationlib.FilterByUpdatedBefore(time.Now().AddDate(-2, 0, 0)),
)
deleteOps := oldProducts.ToDeleteOperations()
```

## Built-in Filters

The library provides many built-in filters:

```go
// Content type filters
migrationlib.FilterByContentType("product", "category")
migrationlib.FilterByType("Entry")  // or "Asset"

// Publication status
migrationlib.FilterPublished()
migrationlib.FilterDrafts()

// Timestamp filters
migrationlib.FilterByCreatedAfter(time)
migrationlib.FilterByUpdatedAfter(time)

// Field filters
migrationlib.FilterByFieldValue("status", "active")
migrationlib.FilterByFieldExists("description")
migrationlib.FilterByFieldContains("title", "important")

// ID patterns
migrationlib.FilterByIDPattern("prod-")
```

## Configuration

Load configuration from environment variables and initialize a ready-to-use client:

```go
// From environment variables
config := migrationlib.LoadConfigFromEnv()

// Or create custom config
config := &migrationlib.Config{
    CMAToken:    "your-cma-key",
    SpaceID:     "your-space-id",
    Environment: "master",
    DryRun:      true,
    Verbose:     true,
}

// Initialize ready-to-use client with logger and loaded space model
client, logger, err := migrationlib.Init(config)
if err != nil {
    log.Fatal(err)
}
```

Environment variables:
- `CONTENTFUL_CMAKEY`: CMA API key (mandatory)
- `CONTENTFUL_SPACE_ID`: Space ID (mandatory)
- `CONTENTFUL_ENVIRONMENT`: Environment (default: "dev")
- `CONTENTFUL_DRY_RUN`: Enable dry run mode
- `CONTENTFUL_VERBOSE`: Enable verbose logging

## Example Usage

See the `example/` directory for complete examples that demonstrate:

- Loading space models
- Filtering entities by various criteria
- Type-safe field access
- Reference resolution and handling
- Asset-specific operations
- Collection operations and chaining
- Creating and executing migration operations
- Handling results and statistics

### Basic Example

```go
package main

import (
    "log"
    
    "github.com/bestbytes/globus/cmd/migrations/migrationlib"
)

func main() {
    // Load config and initialize ready-to-use client
    config := migrationlib.LoadConfigFromEnv()
    client, logger, err := migrationlib.Init(config)
    if err != nil {
        log.Fatal(err)
    }
    
    // Get entities as collections
    allEntities := client.GetAllEntities()
    entries := client.GetEntries()
    assets := client.GetAssets()
    
    // Filter entities
    products := client.FilterEntities(
        migrationlib.FilterByContentType("product"),
        migrationlib.FilterPublished(),
    )
    
    // Process entries with type-safe field access
    products.ForEach(func(entity migrationlib.Entity) {
        title := entity.GetFieldValueAsString("title", migrationlib.Locale("en"))
        price := entity.GetFieldValueAsFloat64("price", migrationlib.Locale("en"))
        
        // Handle references
        if categoryEntity, found := entity.GetFieldValueAsReferencedEntity("category", migrationlib.Locale("en")); found {
            categoryName := categoryEntity.GetFieldValueAsString("name", migrationlib.Locale("en"))
            logger.Info("Product: %s (Category: %s, Price: %.2f)", title, categoryName, price)
        }
    })
    
    // Process assets
    assets.ForEach(func(asset migrationlib.Entity) {
        title := asset.GetTitle(migrationlib.Locale("en"))
        file := asset.GetFile(migrationlib.Locale("en"))
        if file != nil {
            logger.Info("Asset: %s (%s)", title, file.Name)
        }
    })
}
```

## Error Handling

The library provides comprehensive error handling:

```go
// Check operation results
for _, result := range results {
    if !result.Success {
        log.Printf("Failed to %s %s: %v", 
            result.Operation, result.EntityID, result.Error)
    }
}

// Get summary statistics
stats := client.GetStats()
log.Printf("Processed %d entities with %d errors", 
    stats.TotalEntities, stats.Errors)
```

## Performance Considerations

- The library loads entire space models into memory for efficient operations
- Use appropriate batch sizes for large operations
- Consider using dry-run mode for testing
- Filter entities early to reduce memory usage
- Use pagination for very large spaces

## Dependencies

- `github.com/foomo/contentful`: Contentful Go SDK
- Standard Go library only

## License

Distributed under MIT License, please see license file within the code for more details.

_Made with ♥ [foomo](https://www.foomo.org) by [bestbytes](https://www.bestbytes.com)_

