package commanderclient

import (
	"testing"

	"github.com/foomo/contentful"
)

func TestEntityInterface(t *testing.T) {
	// Test EntryEntity
	entry := &contentful.Entry{
		Sys: &contentful.Sys{
			ID:               "test-entry-id",
			Version:          2,
			CreatedAt:        "2024-01-01T00:00:00Z",
			UpdatedAt:        "2024-01-01T00:00:00Z",
			PublishedVersion: 1,
			ContentType: &contentful.ContentType{
				Sys: &contentful.Sys{
					ID: "test-content-type",
				},
			},
		},
		Fields: map[string]any{
			"title": "Test Entry",
		},
	}

	entryEntity := &EntryEntity{Entry: entry}

	if entryEntity.GetID() != "test-entry-id" {
		t.Errorf("Expected ID 'test-entry-id', got '%s'", entryEntity.GetID())
	}

	if entryEntity.GetType() != "Entry" {
		t.Errorf("Expected type 'Entry', got '%s'", entryEntity.GetType())
	}

	if entryEntity.GetContentType() != "test-content-type" {
		t.Errorf("Expected content type 'test-content-type', got '%s'", entryEntity.GetContentType())
	}

	if entryEntity.GetVersion() != 2 {
		t.Errorf("Expected version 2, got %d", entryEntity.GetVersion())
	}

	if !entryEntity.IsPublished() {
		t.Error("Expected entry to be published")
	}

	if entryEntity.GetPublishingStatus() != StatusPublished {
		t.Errorf("Expected publishing status '%s', got '%s'", StatusPublished, entryEntity.GetPublishingStatus())
	}

	// Test AssetEntity - draft (Version 0, PublishedVersion 0)
	asset := &contentful.Asset{
		Sys: &contentful.Sys{
			ID:               "test-asset-id",
			Version:          0,
			CreatedAt:        "2024-01-01T00:00:00Z",
			UpdatedAt:        "2024-01-01T00:00:00Z",
			PublishedVersion: 0,
		},
		Fields: &contentful.FileFields{
			Title: map[string]string{
				"en-US": "Test Asset",
			},
		},
	}

	assetEntity := &AssetEntity{Asset: asset}

	if assetEntity.GetID() != "test-asset-id" {
		t.Errorf("Expected ID 'test-asset-id', got '%s'", assetEntity.GetID())
	}

	if assetEntity.GetType() != "Asset" {
		t.Errorf("Expected type 'Asset', got '%s'", assetEntity.GetType())
	}

	if assetEntity.GetContentType() != "" {
		t.Errorf("Expected empty content type for asset, got '%s'", assetEntity.GetContentType())
	}

	if assetEntity.IsPublished() {
		t.Error("Expected asset to be unpublished")
	}

	if assetEntity.GetPublishingStatus() != StatusDraft {
		t.Errorf("Expected publishing status '%s', got '%s'", StatusDraft, assetEntity.GetPublishingStatus())
	}
}

func TestEntityCollection(t *testing.T) {
	// Create test entities
	entry1 := &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID: "entry-1",
				ContentType: &contentful.ContentType{
					Sys: &contentful.Sys{ID: "product"},
				},
				Version:          2,
				PublishedVersion: 1,
			},
		},
	}

	entry2 := &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID: "entry-2",
				ContentType: &contentful.ContentType{
					Sys: &contentful.Sys{ID: "category"},
				},
				Version:          0,
				PublishedVersion: 0,
			},
		},
	}

	asset1 := &AssetEntity{
		Asset: &contentful.Asset{
			Sys: &contentful.Sys{
				ID:               "asset-1",
				Version:          0,
				PublishedVersion: 0,
			},
		},
	}

	entities := []Entity{entry1, entry2, asset1}
	collection := NewEntityCollection(entities)

	// Test basic operations
	if collection.Count() != 3 {
		t.Errorf("Expected count 3, got %d", collection.Count())
	}

	// Test filtering
	entryCollection := collection.Filter(FilterByType("Entry"))
	if entryCollection.Count() != 2 {
		t.Errorf("Expected 2 entries, got %d", entryCollection.Count())
	}

	productCollection := collection.Filter(FilterByContentType("product"))
	if productCollection.Count() != 1 {
		t.Errorf("Expected 1 product, got %d", productCollection.Count())
	}

	// Test grouping
	groups := collection.GroupBy(func(entity Entity) string {
		if entity.GetType() == "Entry" {
			return entity.GetContentType()
		}
		return "Asset"
	})

	if len(groups) != 3 {
		t.Errorf("Expected 3 groups, got %d", len(groups))
	}

	if len(groups["product"]) != 1 {
		t.Errorf("Expected 1 product, got %d", len(groups["product"]))
	}

	if len(groups["category"]) != 1 {
		t.Errorf("Expected 1 category, got %d", len(groups["category"]))
	}

	if len(groups["Asset"]) != 1 {
		t.Errorf("Expected 1 asset, got %d", len(groups["Asset"]))
	}

	// Test new collection methods
	ids := collection.ExtractIDs()
	if len(ids) != 3 {
		t.Errorf("Expected 3 IDs, got %d", len(ids))
	}

	contentTypes := collection.ExtractContentTypes()
	if len(contentTypes) != 2 {
		t.Errorf("Expected 2 content types, got %d", len(contentTypes))
	}

	// Test grouping by content type
	contentTypeGroups := collection.GroupByContentType()
	if len(contentTypeGroups) != 2 {
		t.Errorf("Expected 2 content type groups, got %d", len(contentTypeGroups))
	}

	// Test grouping by publishing status
	statusGroups := collection.GroupByPublishingStatus()
	if len(statusGroups) != 2 {
		t.Errorf("Expected 2 status groups, got %d", len(statusGroups))
	}

	// Test counting methods
	contentTypeCounts := collection.CountByContentType()
	if contentTypeCounts["product"] != 1 {
		t.Errorf("Expected 1 product, got %d", contentTypeCounts["product"])
	}
	if contentTypeCounts["category"] != 1 {
		t.Errorf("Expected 1 category, got %d", contentTypeCounts["category"])
	}

	// Test stats
	stats := collection.GetStats()
	if stats.TotalCount != 3 {
		t.Errorf("Expected total count 3, got %d", stats.TotalCount)
	}
	if stats.EntryCount != 2 {
		t.Errorf("Expected entry count 2, got %d", stats.EntryCount)
	}
	if stats.AssetCount != 1 {
		t.Errorf("Expected asset count 1, got %d", stats.AssetCount)
	}

	// Test migration operations
	updateOps := collection.ToUpdateOperations(map[string]any{"test": "value"})
	if len(updateOps) != 3 {
		t.Errorf("Expected 3 update operations, got %d", len(updateOps))
	}

	publishOps := collection.ToPublishOperations()
	if len(publishOps) != 3 {
		t.Errorf("Expected 3 publish operations, got %d", len(publishOps))
	}
}

func TestMigrationClient(t *testing.T) {
	// Test client creation
	client := newMigrationClient("test-key", "test-space", "master")

	if client.GetSpaceID() != "test-space" {
		t.Errorf("Expected space ID 'test-space', got '%s'", client.GetSpaceID())
	}

	if client.GetEnvironment() != "master" {
		t.Errorf("Expected environment 'master', got '%s'", client.GetEnvironment())
	}

	// Test stats
	stats := client.GetStats()
	if stats.TotalEntities != 0 {
		t.Errorf("Expected 0 total entities, got %d", stats.TotalEntities)
	}
}

func TestFilters(t *testing.T) {
	// Create test entities
	entry1 := &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID: "test-entry",
				ContentType: &contentful.ContentType{
					Sys: &contentful.Sys{ID: "product"},
				},
				Version:          2,
				PublishedVersion: 1,
				CreatedAt:        "2024-01-01T00:00:00Z",
			},
			Fields: map[string]any{
				"title": "Test Product",
			},
		},
	}

	entry2 := &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID: "draft-entry",
				ContentType: &contentful.ContentType{
					Sys: &contentful.Sys{ID: "category"},
				},
				Version:          0,
				PublishedVersion: 0,
				CreatedAt:        "2024-01-01T00:00:00Z",
			},
			Fields: map[string]any{
				"title": "Draft Category",
			},
		},
	}

	entities := []Entity{entry1, entry2}
	collection := NewEntityCollection(entities)

	// Test published filter
	published := collection.Filter(FilterPublished())
	if published.Count() != 1 {
		t.Errorf("Expected 1 published entity, got %d", published.Count())
	}

	// Test drafts filter
	drafts := collection.Filter(FilterDrafts())
	if drafts.Count() != 1 {
		t.Errorf("Expected 1 draft entity, got %d", drafts.Count())
	}

	// Test content type filter
	products := collection.Filter(FilterByContentType("product"))
	if products.Count() != 1 {
		t.Errorf("Expected 1 product, got %d", products.Count())
	}

	// Test field value filter
	titleFilter := collection.Filter(FilterByFieldValue("title", "Test Product"))
	if titleFilter.Count() != 1 {
		t.Errorf("Expected 1 entity with title 'Test Product', got %d", titleFilter.Count())
	}

	// Test field exists filter
	titleExists := collection.Filter(FilterByFieldExists("title"))
	if titleExists.Count() != 2 {
		t.Errorf("Expected 2 entities with title field, got %d", titleExists.Count())
	}
}

func TestPublishingStatus(t *testing.T) {
	// Test draft status (PublishedVersion = 0)
	draftEntry := &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID:               "draft-entry",
				Version:          0,
				PublishedVersion: 0,
			},
		},
	}

	if draftEntry.GetPublishingStatus() != StatusDraft {
		t.Errorf("Expected draft status, got '%s'", draftEntry.GetPublishingStatus())
	}

	if draftEntry.IsPublished() {
		t.Error("Expected draft entry to not be published")
	}

	// Test published status (Version = PublishedVersion + 1)
	publishedEntry := &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID:               "published-entry",
				Version:          2,
				PublishedVersion: 1,
			},
		},
	}

	if publishedEntry.GetPublishingStatus() != StatusPublished {
		t.Errorf("Expected published status, got '%s'", publishedEntry.GetPublishingStatus())
	}

	if !publishedEntry.IsPublished() {
		t.Error("Expected published entry to be published")
	}

	// Test changed status (Version > PublishedVersion + 1)
	changedEntry := &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID:               "changed-entry",
				Version:          3,
				PublishedVersion: 1,
			},
		},
	}

	if changedEntry.GetPublishingStatus() != StatusChanged {
		t.Errorf("Expected changed status, got '%s'", changedEntry.GetPublishingStatus())
	}

	if changedEntry.IsPublished() {
		t.Error("Expected changed entry to not be published")
	}
}
