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
	updateOps := collection.ToUpdateOperations()
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
	client := newMigrationClient("test-key", "", "test-space", "master")

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

func TestGetParents(t *testing.T) {
	client := newMigrationClient("test-key", "", "test-space", "master")

	// Target entry — the one we'll look for parents of
	target := &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID:      "child-1",
				Version: 1,
				ContentType: &contentful.ContentType{
					Sys: &contentful.Sys{ID: "article"},
				},
			},
			Fields: map[string]any{},
		},
		Client: client,
	}

	// Parent with a single reference to the target
	parentSingle := &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID:      "parent-single",
				Version: 1,
				ContentType: &contentful.ContentType{
					Sys: &contentful.Sys{ID: "page"},
				},
			},
			Fields: map[string]any{
				"hero": map[string]any{
					"en-US": map[string]any{
						"sys": map[string]any{
							"id":   "child-1",
							"type": "Link",
						},
					},
				},
			},
		},
		Client: client,
	}

	// Parent with an array of references containing the target
	parentArray := &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID:      "parent-array",
				Version: 1,
				ContentType: &contentful.ContentType{
					Sys: &contentful.Sys{ID: "category"},
				},
			},
			Fields: map[string]any{
				"items": map[string]any{
					"en-US": []any{
						map[string]any{
							"sys": map[string]any{
								"id":   "unrelated-id",
								"type": "Link",
							},
						},
						map[string]any{
							"sys": map[string]any{
								"id":   "child-1",
								"type": "Link",
							},
						},
					},
				},
			},
		},
		Client: client,
	}

	// Entry that does NOT reference the target
	unrelated := &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID:      "unrelated",
				Version: 1,
				ContentType: &contentful.ContentType{
					Sys: &contentful.Sys{ID: "page"},
				},
			},
			Fields: map[string]any{
				"hero": map[string]any{
					"en-US": map[string]any{
						"sys": map[string]any{
							"id":   "other-entry",
							"type": "Link",
						},
					},
				},
			},
		},
		Client: client,
	}

	// Populate the client cache
	client.cache["child-1"] = target
	client.cache["parent-single"] = parentSingle
	client.cache["parent-array"] = parentArray
	client.cache["unrelated"] = unrelated

	// nil contentTypes — returns all parents
	t.Run("all parents", func(t *testing.T) {
		parents := target.GetParents(nil)
		if parents.Count() != 2 {
			t.Errorf("Expected 2 parents, got %d", parents.Count())
		}
	})

	// Filter by content type
	t.Run("filter by content type", func(t *testing.T) {
		parents := target.GetParents([]string{"page"})
		if parents.Count() != 1 {
			t.Errorf("Expected 1 parent of type 'page', got %d", parents.Count())
		}
		items := parents.Get()
		if len(items) != 1 || items[0].GetID() != "parent-single" {
			t.Errorf("Expected parent-single, got %v", items)
		}
	})

	// Filter with non-matching content type
	t.Run("no matching content type", func(t *testing.T) {
		parents := target.GetParents([]string{"nonexistent"})
		if parents.Count() != 0 {
			t.Errorf("Expected 0 parents, got %d", parents.Count())
		}
	})

	// Entry with no parents
	t.Run("no parents", func(t *testing.T) {
		parents := unrelated.GetParents(nil)
		if parents.Count() != 0 {
			t.Errorf("Expected 0 parents, got %d", parents.Count())
		}
	})

	// Nil client
	t.Run("nil client", func(t *testing.T) {
		orphan := &EntryEntity{
			Entry: &contentful.Entry{
				Sys: &contentful.Sys{ID: "orphan"},
			},
		}
		parents := orphan.GetParents(nil)
		if parents.Count() != 0 {
			t.Errorf("Expected 0 parents for nil client, got %d", parents.Count())
		}
	})

	// Reference in a non-default locale
	t.Run("reference in another locale", func(t *testing.T) {
		parentDE := &EntryEntity{
			Entry: &contentful.Entry{
				Sys: &contentful.Sys{
					ID:      "parent-de",
					Version: 1,
					ContentType: &contentful.ContentType{
						Sys: &contentful.Sys{ID: "page"},
					},
				},
				Fields: map[string]any{
					"hero": map[string]any{
						"de-DE": map[string]any{
							"sys": map[string]any{
								"id":   "child-1",
								"type": "Link",
							},
						},
					},
				},
			},
			Client: client,
		}
		client.cache["parent-de"] = parentDE

		parents := target.GetParents(nil)
		if parents.Count() != 3 {
			t.Errorf("Expected 3 parents (including de-DE ref), got %d", parents.Count())
		}

		// Clean up
		delete(client.cache, "parent-de")
	})
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

func TestCDAView(t *testing.T) {
	t.Run("entry with CDA view", func(t *testing.T) {
		cdaEntry := &EntryEntity{
			Entry: &contentful.Entry{
				Sys: &contentful.Sys{
					ID:               "entry-1",
					Version:          2,
					PublishedVersion: 1,
					ContentType: &contentful.ContentType{
						Sys: &contentful.Sys{ID: "article"},
					},
				},
				Fields: map[string]any{
					"title": map[string]any{
						"en-US": "Published Title",
					},
				},
			},
		}

		cmaEntry := &EntryEntity{
			Entry: &contentful.Entry{
				Sys: &contentful.Sys{
					ID:               "entry-1",
					Version:          3,
					PublishedVersion: 1,
					ContentType: &contentful.ContentType{
						Sys: &contentful.Sys{ID: "article"},
					},
				},
				Fields: map[string]any{
					"title": map[string]any{
						"en-US": "Draft Title (changed)",
					},
				},
			},
			cdaView: cdaEntry,
		}

		if !cmaEntry.HasCDAView() {
			t.Error("Expected HasCDAView() to be true")
		}

		view := cmaEntry.CDAView()
		if view == nil {
			t.Fatal("Expected CDAView() to return non-nil entity")
		}

		// Compare field values between CMA and CDA views
		cmaTitle := cmaEntry.GetFieldValueAsString("title", "en-US")
		cdaTitle := view.GetFieldValueAsString("title", "en-US")
		if cmaTitle != "Draft Title (changed)" {
			t.Errorf("Expected CMA title 'Draft Title (changed)', got '%s'", cmaTitle)
		}
		if cdaTitle != "Published Title" {
			t.Errorf("Expected CDA title 'Published Title', got '%s'", cdaTitle)
		}
	})

	t.Run("entry without CDA view (draft)", func(t *testing.T) {
		draftEntry := &EntryEntity{
			Entry: &contentful.Entry{
				Sys: &contentful.Sys{
					ID:               "draft-1",
					Version:          1,
					PublishedVersion: 0,
					ContentType: &contentful.ContentType{
						Sys: &contentful.Sys{ID: "article"},
					},
				},
			},
		}

		if draftEntry.HasCDAView() {
			t.Error("Expected HasCDAView() to be false for draft entry")
		}
		if draftEntry.CDAView() != nil {
			t.Error("Expected CDAView() to be nil for draft entry")
		}
	})

	t.Run("CDA view entity has no CDA view itself", func(t *testing.T) {
		cdaEntry := &EntryEntity{
			Entry: &contentful.Entry{
				Sys: &contentful.Sys{
					ID:               "entry-1",
					Version:          2,
					PublishedVersion: 1,
					ContentType: &contentful.ContentType{
						Sys: &contentful.Sys{ID: "article"},
					},
				},
			},
		}

		cmaEntry := &EntryEntity{
			Entry: &contentful.Entry{
				Sys: &contentful.Sys{
					ID:               "entry-1",
					Version:          3,
					PublishedVersion: 1,
					ContentType: &contentful.ContentType{
						Sys: &contentful.Sys{ID: "article"},
					},
				},
			},
			cdaView: cdaEntry,
		}

		// The CDA view itself should NOT have a CDA view (no recursion)
		view := cmaEntry.CDAView()
		if view.HasCDAView() {
			t.Error("Expected CDA view entity's HasCDAView() to be false")
		}
		if view.CDAView() != nil {
			t.Error("Expected CDA view entity's CDAView() to be nil")
		}
	})

	t.Run("asset CDA view", func(t *testing.T) {
		cdaAsset := &AssetEntity{
			Asset: &contentful.Asset{
				Sys: &contentful.Sys{
					ID:               "asset-1",
					Version:          2,
					PublishedVersion: 1,
				},
				Fields: &contentful.FileFields{
					Title: map[string]string{
						"en-US": "Published Asset",
					},
				},
			},
		}

		cmaAsset := &AssetEntity{
			Asset: &contentful.Asset{
				Sys: &contentful.Sys{
					ID:               "asset-1",
					Version:          3,
					PublishedVersion: 1,
				},
				Fields: &contentful.FileFields{
					Title: map[string]string{
						"en-US": "Updated Asset",
					},
				},
			},
			cdaView: cdaAsset,
		}

		if !cmaAsset.HasCDAView() {
			t.Error("Expected asset HasCDAView() to be true")
		}

		view := cmaAsset.CDAView()
		if view == nil {
			t.Fatal("Expected asset CDAView() to return non-nil entity")
		}

		cdaTitle := view.GetTitle("en-US")
		cmaTitle := cmaAsset.GetTitle("en-US")
		if cmaTitle != "Updated Asset" {
			t.Errorf("Expected CMA asset title 'Updated Asset', got '%s'", cmaTitle)
		}
		if cdaTitle != "Published Asset" {
			t.Errorf("Expected CDA asset title 'Published Asset', got '%s'", cdaTitle)
		}

		// Asset without CDA view
		if cdaAsset.HasCDAView() {
			t.Error("Expected CDA asset view's HasCDAView() to be false")
		}
	})

	t.Run("FilterHasCDAView and FilterNoCDAView", func(t *testing.T) {
		withCDA := &EntryEntity{
			Entry: &contentful.Entry{
				Sys: &contentful.Sys{
					ID:               "with-cda",
					Version:          2,
					PublishedVersion: 1,
					ContentType: &contentful.ContentType{
						Sys: &contentful.Sys{ID: "article"},
					},
				},
			},
			cdaView: &EntryEntity{
				Entry: &contentful.Entry{
					Sys: &contentful.Sys{
						ID:               "with-cda",
						Version:          2,
						PublishedVersion: 1,
						ContentType: &contentful.ContentType{
							Sys: &contentful.Sys{ID: "article"},
						},
					},
				},
			},
		}

		withoutCDA := &EntryEntity{
			Entry: &contentful.Entry{
				Sys: &contentful.Sys{
					ID:               "without-cda",
					Version:          1,
					PublishedVersion: 0,
					ContentType: &contentful.ContentType{
						Sys: &contentful.Sys{ID: "article"},
					},
				},
			},
		}

		collection := NewEntityCollection([]Entity{withCDA, withoutCDA})

		hasCDA := collection.Filter(FilterHasCDAView())
		if hasCDA.Count() != 1 {
			t.Errorf("Expected 1 entity with CDA view, got %d", hasCDA.Count())
		}
		if hasCDA.Get()[0].GetID() != "with-cda" {
			t.Errorf("Expected entity 'with-cda', got '%s'", hasCDA.Get()[0].GetID())
		}

		noCDA := collection.Filter(FilterNoCDAView())
		if noCDA.Count() != 1 {
			t.Errorf("Expected 1 entity without CDA view, got %d", noCDA.Count())
		}
		if noCDA.Get()[0].GetID() != "without-cda" {
			t.Errorf("Expected entity 'without-cda', got '%s'", noCDA.Get()[0].GetID())
		}
	})
}

func TestIsFieldNullOrEmpty(t *testing.T) {
	t.Run("entry entity", func(t *testing.T) {
		entry := &EntryEntity{
			Entry: &contentful.Entry{
				Sys: &contentful.Sys{
					ID:      "test-entry",
					Version: 1,
					ContentType: &contentful.ContentType{
						Sys: &contentful.Sys{ID: "article"},
					},
				},
				Fields: map[string]any{
					"title": map[string]any{
						"en-US": "Hello",
						"de-DE": "",
					},
					"body": map[string]any{
						"en-US": map[string]any{},
					},
					"tags": map[string]any{
						"en-US": []any{},
						"de-DE": []any{"tag1"},
					},
				},
			},
		}

		// nil field (field doesn't exist)
		if !entry.IsFieldNullOrEmpty("nonexistent", "en-US") {
			t.Error("Expected nonexistent field to be null or empty")
		}

		// non-empty string
		if entry.IsFieldNullOrEmpty("title", "en-US") {
			t.Error("Expected non-empty title to not be null or empty")
		}

		// empty string
		if !entry.IsFieldNullOrEmpty("title", "de-DE") {
			t.Error("Expected empty string title to be null or empty")
		}

		// nil locale (locale doesn't exist)
		if !entry.IsFieldNullOrEmpty("title", "fr-FR") {
			t.Error("Expected missing locale to be null or empty")
		}

		// empty map
		if !entry.IsFieldNullOrEmpty("body", "en-US") {
			t.Error("Expected empty map to be null or empty")
		}

		// empty slice
		if !entry.IsFieldNullOrEmpty("tags", "en-US") {
			t.Error("Expected empty slice to be null or empty")
		}

		// non-empty slice
		if entry.IsFieldNullOrEmpty("tags", "de-DE") {
			t.Error("Expected non-empty slice to not be null or empty")
		}
	})

	t.Run("asset entity", func(t *testing.T) {
		asset := &AssetEntity{
			Asset: &contentful.Asset{
				Sys: &contentful.Sys{
					ID:      "test-asset",
					Version: 1,
				},
				Fields: &contentful.FileFields{
					Title: map[string]string{
						"en-US": "My Asset",
					},
					Description: map[string]string{
						"en-US": "",
					},
					File: map[string]*contentful.File{
						"en-US": {URL: "https://example.com/file.png"},
					},
				},
			},
		}

		// non-empty title
		if asset.IsFieldNullOrEmpty("title", "en-US") {
			t.Error("Expected non-empty title to not be null or empty")
		}

		// missing locale for title
		if !asset.IsFieldNullOrEmpty("title", "de-DE") {
			t.Error("Expected missing locale title to be null or empty")
		}

		// empty description
		if !asset.IsFieldNullOrEmpty("description", "en-US") {
			t.Error("Expected empty description to be null or empty")
		}

		// non-nil file
		if asset.IsFieldNullOrEmpty("file", "en-US") {
			t.Error("Expected non-nil file to not be null or empty")
		}

		// missing locale for file
		if !asset.IsFieldNullOrEmpty("file", "de-DE") {
			t.Error("Expected missing locale file to be null or empty")
		}

		// unknown field
		if !asset.IsFieldNullOrEmpty("unknown", "en-US") {
			t.Error("Expected unknown field to be null or empty")
		}
	})
}
