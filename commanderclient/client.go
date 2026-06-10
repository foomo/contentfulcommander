package commanderclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
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
	cacheMu     sync.RWMutex
	updateMu    sync.Mutex
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
	// Serialize with UpdateSpaceModel: a full reload and an incremental update
	// must not run at the same time. Readers are not blocked during the load —
	// the new model is built locally and swapped in atomically at the end.
	mc.updateMu.Lock()
	defer mc.updateMu.Unlock()

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
		if err := mc.loadEntries(gCtx, spaceModel, 0, logger); err != nil {
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
			return mc.loadCDAEntries(gCDACtx, spaceModel, 0, logger)
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

	// Build the new cache locally, then swap it in under the write lock so
	// concurrent readers never observe a partially populated cache.
	newCache := make(map[string]Entity, len(spaceModel.Entries)+len(spaceModel.Assets))
	maps.Copy(newCache, spaceModel.Entries)
	maps.Copy(newCache, spaceModel.Assets)

	mc.cacheMu.Lock()
	mc.spaceModel = spaceModel
	mc.cache = newCache
	mc.stats.TotalEntities = len(newCache)
	mc.cacheMu.Unlock()

	return nil
}

// UpdateSpaceModel incrementally updates the cached space model by fetching
// only entities that have changed since the last LoadSpaceModel or UpdateSpaceModel call.
// It uses order=-sys.updatedAt to load recently changed entities first and stops
// when reaching entities older than the previous update start time.
func (mc *MigrationClient) UpdateSpaceModel(ctx context.Context, logger *Logger) error {
	// Serialize updates: concurrent calls would race on LastUpdated and on the
	// cache map. A waiting caller proceeds with the freshly written cutoff.
	mc.updateMu.Lock()
	defer mc.updateMu.Unlock()

	mc.cacheMu.RLock()
	if mc.spaceModel == nil {
		mc.cacheMu.RUnlock()
		return fmt.Errorf("space model not loaded, call LoadSpaceModel first")
	}
	cutoff := mc.spaceModel.LastUpdated
	mc.cacheMu.RUnlock()

	updateStart := time.Now()

	// Update entries and assets concurrently
	var updatedEntries map[string]*EntryEntity
	var updatedAssets map[string]*AssetEntity
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		entries, err := mc.updateEntries(gCtx, cutoff, logger)
		if err != nil {
			return err
		}
		updatedEntries = entries
		return nil
	})
	if !mc.skipAssets {
		g.Go(func() error {
			assets, err := mc.updateAssets(gCtx, cutoff, logger)
			if err != nil {
				return err
			}
			updatedAssets = assets
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	// Update CDA views for changed entities
	var updatedCDAEntries map[string]Entity
	var updatedCDAAssets map[string]Entity
	if mc.cda != nil {
		gCDA, gCDACtx := errgroup.WithContext(ctx)
		gCDA.Go(func() error {
			entries, err := mc.updateCDAEntries(gCDACtx, cutoff, logger)
			if err != nil {
				return err
			}
			updatedCDAEntries = entries
			return nil
		})
		if !mc.skipAssets {
			gCDA.Go(func() error {
				assets, err := mc.updateCDAAssets(gCDACtx, cutoff, logger)
				if err != nil {
					return err
				}
				updatedCDAAssets = assets
				return nil
			})
		}
		if err := gCDA.Wait(); err != nil {
			return err
		}
	}

	mc.cacheMu.Lock()
	current := mc.spaceModel
	spaceModel := &SpaceModel{
		SpaceID:       current.SpaceID,
		Environment:   current.Environment,
		Locales:       append([]LocaleInfo(nil), current.Locales...),
		DefaultLocale: current.DefaultLocale,
		ContentTypes:  maps.Clone(current.ContentTypes),
		Entries:       maps.Clone(current.Entries),
		Assets:        maps.Clone(current.Assets),
		LastUpdated:   updateStart,
	}

	for id, entity := range updatedEntries {
		if previous, ok := spaceModel.Entries[id]; ok && entity.GetPublishingStatus() != StatusDraft {
			entity.cdaView = previous.CDAView()
		}
		spaceModel.Entries[id] = entity
	}
	for id, entity := range updatedAssets {
		if previous, ok := spaceModel.Assets[id]; ok && entity.GetPublishingStatus() != StatusDraft {
			entity.cdaView = previous.CDAView()
		}
		spaceModel.Assets[id] = entity
	}
	for id, cdaView := range updatedCDAEntries {
		if cmaEntity, ok := spaceModel.Entries[id]; ok {
			if entryEntity, ok := cmaEntity.(*EntryEntity); ok {
				spaceModel.Entries[id] = &EntryEntity{
					Entry:   entryEntity.Entry,
					Client:  entryEntity.Client,
					cdaView: cdaView,
				}
			}
		}
	}
	for id, cdaView := range updatedCDAAssets {
		if cmaEntity, ok := spaceModel.Assets[id]; ok {
			if assetEntity, ok := cmaEntity.(*AssetEntity); ok {
				spaceModel.Assets[id] = &AssetEntity{
					Asset:   assetEntity.Asset,
					Client:  assetEntity.Client,
					cdaView: cdaView,
				}
			}
		}
	}

	newCache := make(map[string]Entity, len(spaceModel.Entries)+len(spaceModel.Assets))
	maps.Copy(newCache, spaceModel.Entries)
	maps.Copy(newCache, spaceModel.Assets)

	mc.spaceModel = spaceModel
	mc.cache = newCache
	mc.stats.TotalEntities = len(newCache)
	mc.cacheMu.Unlock()

	return nil
}

// GetSpaceModel returns the cached space model
func (mc *MigrationClient) GetSpaceModel() *SpaceModel {
	mc.cacheMu.RLock()
	defer mc.cacheMu.RUnlock()
	return mc.spaceModel
}

// GetContentType retrieves a content type by ID from the loaded space model.
func (mc *MigrationClient) GetContentType(contentTypeID string) (*contentful.ContentType, bool) {
	if mc == nil {
		return nil, false
	}

	mc.cacheMu.RLock()
	defer mc.cacheMu.RUnlock()

	if mc.spaceModel == nil || mc.spaceModel.ContentTypes == nil {
		return nil, false
	}

	contentType, exists := mc.spaceModel.ContentTypes[contentTypeID]
	if !exists || contentType == nil {
		return nil, false
	}
	return contentType, true
}

// GetContentTypeField retrieves a field from a content type in the loaded space model.
func (mc *MigrationClient) GetContentTypeField(contentTypeID string, fieldID string) (*contentful.Field, bool) {
	contentType, exists := mc.GetContentType(contentTypeID)
	if !exists {
		return nil, false
	}

	for _, field := range contentType.Fields {
		if field != nil && field.ID == fieldID {
			return field, true
		}
	}
	return nil, false
}

// GetEntryDisplayName returns the raw locale value of the content type display field, or the entity ID.
func (mc *MigrationClient) GetEntryDisplayName(entity Entity, locale Locale) string {
	if isNilEntity(entity) {
		return ""
	}

	entityID := entity.GetID()
	contentTypeID := entity.GetContentType()
	if contentTypeID == "" {
		return entityID
	}

	contentType, exists := mc.GetContentType(contentTypeID)
	if !exists || contentType.DisplayField == "" {
		return entityID
	}

	value := entity.GetFieldValue(contentType.DisplayField, locale)
	if isNullOrEmpty(value) {
		return entityID
	}
	if stringValue, ok := value.(string); ok {
		return stringValue
	}
	return fmt.Sprintf("%v", value)
}

// GetEntity retrieves an entity by ID from cache
func (mc *MigrationClient) GetEntity(id string) (Entity, bool) {
	mc.cacheMu.RLock()
	defer mc.cacheMu.RUnlock()
	entity, exists := mc.cache[id]
	return entity, exists
}

// GetAllEntities returns all cached entities
func (mc *MigrationClient) GetAllEntities() *EntityCollection {
	mc.cacheMu.RLock()
	entities := make([]Entity, 0, len(mc.cache))
	for _, entity := range mc.cache {
		entities = append(entities, entity)
	}
	mc.cacheMu.RUnlock()
	return NewEntityCollection(entities)
}

// GetEntries returns all entry entities
func (mc *MigrationClient) GetEntries() *EntityCollection {
	mc.cacheMu.RLock()
	var entries []Entity
	for _, entity := range mc.cache {
		if entity.GetType() == "Entry" {
			entries = append(entries, entity)
		}
	}
	mc.cacheMu.RUnlock()
	return NewEntityCollection(entries)
}

// GetAssets returns all asset entities
func (mc *MigrationClient) GetAssets() *EntityCollection {
	mc.cacheMu.RLock()
	var assets []Entity
	for _, entity := range mc.cache {
		if entity.GetType() == "Asset" {
			assets = append(assets, entity)
		}
	}
	mc.cacheMu.RUnlock()
	return NewEntityCollection(assets)
}

// GetEntitiesByContentType returns entities filtered by content type
func (mc *MigrationClient) GetEntitiesByContentType(contentType string) *EntityCollection {
	mc.cacheMu.RLock()
	var entities []Entity
	for _, entity := range mc.cache {
		if entity.GetType() == "Entry" && entity.GetContentType() == contentType {
			entities = append(entities, entity)
		}
	}
	mc.cacheMu.RUnlock()
	return NewEntityCollection(entities)
}

// FilterEntities applies filters to entities and returns a collection
func (mc *MigrationClient) FilterEntities(filters ...EntityFilter) *EntityCollection {
	// Snapshot the cache under the read lock, then evaluate filters without it.
	// Filters are arbitrary callbacks that may call back into the client
	// (e.g. GetEntity), so they must not run while the lock is held.
	mc.cacheMu.RLock()
	snapshot := make([]Entity, 0, len(mc.cache))
	for _, entity := range mc.cache {
		snapshot = append(snapshot, entity)
	}
	mc.cacheMu.RUnlock()

	var filtered []Entity
	for _, entity := range snapshot {
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

// SaveDraft persists the current in-memory entity fields without publishing.
func (mc *MigrationClient) SaveDraft(ctx context.Context, entity Entity) error {
	if err := mc.validateEntityMutation(entity); err != nil {
		return err
	}

	executor := NewMigrationExecutor(mc, &MigrationOptions{DryRun: false, Confirm: false})
	_, err := executor.upsertEntity(ctx, &MigrationOperation{
		EntityID:  entity.GetID(),
		Operation: OperationUpsert,
		Entity:    entity,
	})
	if err != nil {
		return fmt.Errorf("save draft failed for %s %q: %w", entity.GetType(), entity.GetID(), err)
	}
	return nil
}

// Publish publishes the entity. It does not persist unsaved in-memory field
// edits — call SaveDraft first if the entity has been modified.
func (mc *MigrationClient) Publish(ctx context.Context, entity Entity) error {
	if err := mc.validateEntityMutation(entity); err != nil {
		return err
	}

	executor := NewMigrationExecutor(mc, &MigrationOptions{DryRun: false, Confirm: false})
	_, err := executor.publishEntity(ctx, &MigrationOperation{
		EntityID:  entity.GetID(),
		Operation: OperationPublish,
		Entity:    entity,
	})
	if err != nil {
		return fmt.Errorf("publish failed for %s %q: %w", entity.GetType(), entity.GetID(), err)
	}
	return nil
}

func (mc *MigrationClient) validateEntityMutation(entity Entity) error {
	if mc == nil {
		return fmt.Errorf("migration client is nil")
	}
	if mc.cma == nil {
		return fmt.Errorf("migration client has no CMA client")
	}
	if isNilEntity(entity) {
		return fmt.Errorf("entity is nil")
	}

	switch typed := entity.(type) {
	case *EntryEntity:
		if typed.Entry == nil {
			return fmt.Errorf("entry entity has nil entry")
		}
		if typed.Entry.Sys == nil {
			return fmt.Errorf("entry entity has nil sys")
		}
	case *AssetEntity:
		if typed.Asset == nil {
			return fmt.Errorf("asset entity has nil asset")
		}
		if typed.Asset.Sys == nil {
			return fmt.Errorf("asset entity has nil sys")
		}
	default:
		return fmt.Errorf("unsupported entity type %T", entity)
	}

	return nil
}

func isNilEntity(entity Entity) bool {
	if entity == nil {
		return true
	}
	value := reflect.ValueOf(entity)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
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
	mc.cacheMu.RLock()
	defer mc.cacheMu.RUnlock()
	if mc.spaceModel == nil {
		return []LocaleInfo{}
	}
	return mc.spaceModel.Locales
}

// GetDefaultLocale returns the default locale for the space
func (mc *MigrationClient) GetDefaultLocale() Locale {
	mc.cacheMu.RLock()
	defer mc.cacheMu.RUnlock()
	if mc.spaceModel == nil {
		return ""
	}
	return mc.spaceModel.DefaultLocale
}

// GetLocaleCodes returns all locale codes for the space
func (mc *MigrationClient) GetLocaleCodes() []Locale {
	mc.cacheMu.RLock()
	defer mc.cacheMu.RUnlock()
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
