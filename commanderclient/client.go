package commanderclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/foomo/contentful"
	"golang.org/x/sync/errgroup"
)

// MigrationClient provides a high-level interface for Contentful migrations
type MigrationClient struct {
	cma         *contentful.Contentful
	cda         *contentful.Contentful
	spaceID     string
	environment string
	spaceModel  *SpaceModel
	cache       map[string]Entity
	cacheMu     sync.Mutex
	stats       *MigrationStats
	concurrency int
	skipAssets  bool
}

// newMigrationClient creates a new migration client
func newMigrationClient(cmaKey, cdaKey, spaceID, environment string) *MigrationClient {
	if environment == "" {
		environment = "dev"
	}

	cma := contentful.NewCMA(cmaKey)
	cma.Environment = environment

	mc := &MigrationClient{
		cma:         cma,
		spaceID:     spaceID,
		environment: environment,
		cache:       make(map[string]Entity),
		stats: &MigrationStats{
			StartTime: time.Now(),
		},
		concurrency: 3,
	}

	if cdaKey != "" {
		cda := contentful.NewCDA(cdaKey)
		cda.Environment = environment
		mc.cda = cda
	}

	return mc
}

// GetSpaceID returns the space ID
func (mc *MigrationClient) GetSpaceID() string {
	return mc.spaceID
}

// GetEnvironment returns the environment
func (mc *MigrationClient) GetEnvironment() string {
	return mc.environment
}

// GetCMA returns the underlying CMA client
func (mc *MigrationClient) GetCMA() *contentful.Contentful {
	return mc.cma
}

// HasCDA returns true if a CDA client is configured
func (mc *MigrationClient) HasCDA() bool {
	return mc.cda != nil
}

// GetCDA returns the underlying CDA client (nil when no CDA key is configured)
func (mc *MigrationClient) GetCDA() *contentful.Contentful {
	return mc.cda
}

// GetStats returns migration statistics
func (mc *MigrationClient) GetStats() *MigrationStats {
	mc.stats.EndTime = time.Now()
	return mc.stats
}

// LoadSpaceModel loads and caches the entire space model
func (mc *MigrationClient) LoadSpaceModel(ctx context.Context, logger *Logger) error {
	spaceModel := &SpaceModel{
		SpaceID:      mc.spaceID,
		Environment:  mc.environment,
		ContentTypes: make(map[string]*contentful.ContentType),
		Entries:      make(map[string]Entity),
		Assets:       make(map[string]Entity),
		LastUpdated:  time.Now(),
	}

	// Load locales first
	if err := mc.loadLocales(ctx, spaceModel); err != nil {
		return fmt.Errorf("failed to load locales: %w", err)
	}

	// Load content types
	if err := mc.loadContentTypes(ctx, spaceModel); err != nil {
		return fmt.Errorf("failed to load content types: %w", err)
	}

	// Load entries and assets concurrently
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := mc.loadEntries(gCtx, spaceModel, 512, logger); err != nil {
			return fmt.Errorf("failed to load entries: %w", err)
		}
		return nil
	})
	if !mc.skipAssets {
		g.Go(func() error {
			if err := mc.loadAssets(gCtx, spaceModel, logger); err != nil {
				return fmt.Errorf("failed to load assets: %w", err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	// Load CDA views (must run after CMA phase — needs CMA entities to attach to)
	if mc.cda != nil {
		gCDA, gCDACtx := errgroup.WithContext(ctx)
		gCDA.Go(func() error {
			return mc.loadCDAEntries(gCDACtx, spaceModel, 512, logger)
		})
		if !mc.skipAssets {
			gCDA.Go(func() error {
				return mc.loadCDAAssets(gCDACtx, spaceModel, logger)
			})
		}
		if err := gCDA.Wait(); err != nil {
			return err
		}
	}

	mc.spaceModel = spaceModel

	// Update cache
	mc.cache = make(map[string]Entity)
	maps.Copy(mc.cache, spaceModel.Entries)
	maps.Copy(mc.cache, spaceModel.Assets)

	mc.stats.TotalEntities = len(mc.cache)

	return nil
}

// GetSpaceModel returns the cached space model
func (mc *MigrationClient) GetSpaceModel() *SpaceModel {
	return mc.spaceModel
}

// GetEntity retrieves an entity by ID from cache
func (mc *MigrationClient) GetEntity(id string) (Entity, bool) {
	entity, exists := mc.cache[id]
	return entity, exists
}

// GetAllEntities returns all cached entities
func (mc *MigrationClient) GetAllEntities() *EntityCollection {
	entities := make([]Entity, 0, len(mc.cache))
	for _, entity := range mc.cache {
		entities = append(entities, entity)
	}
	return NewEntityCollection(entities)
}

// GetEntries returns all entry entities
func (mc *MigrationClient) GetEntries() *EntityCollection {
	var entries []Entity
	for _, entity := range mc.cache {
		if entity.GetType() == "Entry" {
			entries = append(entries, entity)
		}
	}
	return NewEntityCollection(entries)
}

// GetAssets returns all asset entities
func (mc *MigrationClient) GetAssets() *EntityCollection {
	var assets []Entity
	for _, entity := range mc.cache {
		if entity.GetType() == "Asset" {
			assets = append(assets, entity)
		}
	}
	return NewEntityCollection(assets)
}

// GetEntitiesByContentType returns entities filtered by content type
func (mc *MigrationClient) GetEntitiesByContentType(contentType string) *EntityCollection {
	var entities []Entity
	for _, entity := range mc.cache {
		if entity.GetType() == "Entry" && entity.GetContentType() == contentType {
			entities = append(entities, entity)
		}
	}
	return NewEntityCollection(entities)
}

// FilterEntities applies filters to entities and returns a collection
func (mc *MigrationClient) FilterEntities(filters ...EntityFilter) *EntityCollection {
	var filtered []Entity

	for _, entity := range mc.cache {
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
		filters:  filters,
	}
}

// RefreshEntity updates a single entity in the cache
func (mc *MigrationClient) RefreshEntity(ctx context.Context, id string) error {
	// Try to get as entry first
	entry, err := mc.cma.Entries.Get(ctx, mc.spaceID, id)
	if err == nil {
		entity := &EntryEntity{Entry: entry, Client: mc}
		// Fetch CDA view if available (failure is silent — entity may be draft)
		if mc.cda != nil {
			if cdaEntry, cdaErr := mc.cda.Entries.Get(ctx, mc.spaceID, id); cdaErr == nil {
				entity.cdaView = &EntryEntity{Entry: cdaEntry, Client: mc}
			}
		}
		mc.cacheMu.Lock()
		mc.cache[id] = entity
		if mc.spaceModel != nil {
			mc.spaceModel.Entries[id] = entity
		}
		mc.cacheMu.Unlock()
		return nil
	}

	// Try to get as asset
	asset, err := mc.cma.Assets.Get(ctx, mc.spaceID, id)
	if err == nil {
		entity := &AssetEntity{Asset: asset, Client: mc}
		// Fetch CDA view if available
		if mc.cda != nil {
			if cdaAsset, cdaErr := mc.cda.Assets.Get(ctx, mc.spaceID, id); cdaErr == nil {
				entity.cdaView = &AssetEntity{Asset: cdaAsset, Client: mc}
			}
		}
		mc.cacheMu.Lock()
		mc.cache[id] = entity
		if mc.spaceModel != nil {
			mc.spaceModel.Assets[id] = entity
		}
		mc.cacheMu.Unlock()
		return nil
	}

	return fmt.Errorf("entity %s not found", id)
}

// RemoveEntity removes an entity from the cache
func (mc *MigrationClient) RemoveEntity(id string) {
	mc.cacheMu.Lock()
	delete(mc.cache, id)
	if mc.spaceModel != nil {
		delete(mc.spaceModel.Entries, id)
		delete(mc.spaceModel.Assets, id)
	}
	mc.cacheMu.Unlock()
}

// loadLocales loads the locales for the space
func (mc *MigrationClient) loadLocales(ctx context.Context, spaceModel *SpaceModel) error {
	// Get locales from the space
	col, err := mc.cma.Locales.List(ctx, mc.spaceID).GetAll()
	if err != nil {
		return fmt.Errorf("failed to fetch locales: %w", err)
	}

	// Convert to our LocaleInfo format
	localeInfos := make([]LocaleInfo, len(col.Items))
	for i, item := range col.Items {
		// Marshal and unmarshal to get the proper structure
		byteArray, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("failed to marshal locale item: %w", err)
		}

		var locale struct {
			Name         string `json:"name,omitempty"`
			Code         string `json:"code,omitempty"`
			FallbackCode string `json:"fallbackCode,omitempty"`
			Default      bool   `json:"default,omitempty"`
			Optional     bool   `json:"optional,omitempty"`
		}

		err = json.NewDecoder(bytes.NewReader(byteArray)).Decode(&locale)
		if err != nil {
			return fmt.Errorf("failed to decode locale item: %w", err)
		}

		localeInfos[i] = LocaleInfo{
			Code:         Locale(locale.Code),
			Name:         locale.Name,
			FallbackCode: Locale(locale.FallbackCode),
			Optional:     locale.Optional,
			Default:      locale.Default,
		}
	}

	spaceModel.Locales = localeInfos
	spaceModel.DefaultLocale = GetDefaultLocale(localeInfos)

	return nil
}

// GetLocales returns the locales for the space
func (mc *MigrationClient) GetLocales() []LocaleInfo {
	if mc.spaceModel == nil {
		return []LocaleInfo{}
	}
	return mc.spaceModel.Locales
}

// GetDefaultLocale returns the default locale for the space
func (mc *MigrationClient) GetDefaultLocale() Locale {
	if mc.spaceModel == nil {
		return ""
	}
	return mc.spaceModel.DefaultLocale
}

// GetLocaleCodes returns all locale codes for the space
func (mc *MigrationClient) GetLocaleCodes() []Locale {
	if mc.spaceModel == nil {
		return []Locale{}
	}
	return GetLocaleCodes(mc.spaceModel.Locales)
}

// SetConcurrency sets the concurrency level for batch operations
func (mc *MigrationClient) SetConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	mc.concurrency = n
}

// GetConcurrency returns the concurrency level for batch operations
func (mc *MigrationClient) GetConcurrency() int {
	return mc.concurrency
}
