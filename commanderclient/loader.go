package commanderclient

import (
	"context"
)

// loadContentTypes loads all content types from the space
func (mc *MigrationClient) loadContentTypes(ctx context.Context, spaceModel *SpaceModel) error {
	contentTypesCollection := mc.cma.ContentTypes.List(ctx, mc.spaceID)
	contentTypes, err := contentTypesCollection.GetAll()
	if err != nil {
		return err
	}
	for _, contentType := range contentTypes.Items {
		spaceModel.ContentTypes[contentType.Sys.ID] = &contentType
	}
	return nil
}

// loadEntries loads all entries from the space
func (mc *MigrationClient) loadEntries(ctx context.Context, spaceModel *SpaceModel, limit uint16, logger *Logger) error {
	if limit == 0 {
		limit = 512
	}
	entriesCollection := mc.cma.Entries.List(ctx, mc.spaceID)
	entriesCollection.Query.Locale("*").Include(0).Limit(limit)
	entries, err := entriesCollection.GetAll()
	if err != nil {
		return err
	}
	for _, entry := range entries.Items {
		spaceModel.Entries[entry.Sys.ID] = &EntryEntity{Entry: &entry, Client: mc}
	}
	mc.stats.ProcessedEntries += len(entries.Items)
	logger.Info("Loaded %d entries", mc.stats.ProcessedEntries)
	return nil
}

// loadAssets loads all assets from the space
func (mc *MigrationClient) loadAssets(ctx context.Context, spaceModel *SpaceModel, logger *Logger) error {
	assetsCollection := mc.cma.Assets.List(ctx, mc.spaceID)
	assetsCollection.Query.Locale("*").Limit(1000) // Use reasonable batch size
	assets, err := assetsCollection.GetAll()
	if err != nil {
		return err
	}
	for _, asset := range assets.Items {
		spaceModel.Assets[asset.Sys.ID] = &AssetEntity{Asset: &asset, Client: mc}
		mc.stats.ProcessedAssets++
	}
	logger.Info("Loaded %d assets", mc.stats.ProcessedAssets)
	return nil
}
