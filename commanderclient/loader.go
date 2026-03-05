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

// loadCDAEntries loads all published entries via CDA and attaches them as cdaView on matching CMA entities.
func (mc *MigrationClient) loadCDAEntries(ctx context.Context, spaceModel *SpaceModel, limit uint16, logger *Logger) error {
	if limit == 0 {
		limit = 512
	}
	entriesCollection := mc.cda.Entries.List(ctx, mc.spaceID)
	entriesCollection.Query.Locale("*").Include(0).Limit(limit)
	entries, err := entriesCollection.GetAll()
	if err != nil {
		return err
	}
	matched := 0
	for _, entry := range entries.Items {
		if cmaEntity, ok := spaceModel.Entries[entry.Sys.ID]; ok {
			if ee, ok := cmaEntity.(*EntryEntity); ok {
				ee.cdaView = &EntryEntity{Entry: &entry, Client: mc}
				matched++
			}
		}
	}
	logger.Info("Loaded %d CDA entries (%d matched CMA entries)", len(entries.Items), matched)
	return nil
}

// loadCDAAssets loads all published assets via CDA and attaches them as cdaView on matching CMA assets.
func (mc *MigrationClient) loadCDAAssets(ctx context.Context, spaceModel *SpaceModel, logger *Logger) error {
	assetsCollection := mc.cda.Assets.List(ctx, mc.spaceID)
	assetsCollection.Query.Locale("*").Limit(1000)
	assets, err := assetsCollection.GetAll()
	if err != nil {
		return err
	}
	matched := 0
	for _, asset := range assets.Items {
		if cmaEntity, ok := spaceModel.Assets[asset.Sys.ID]; ok {
			if ae, ok := cmaEntity.(*AssetEntity); ok {
				ae.cdaView = &AssetEntity{Asset: &asset, Client: mc}
				matched++
			}
		}
	}
	logger.Info("Loaded %d CDA assets (%d matched CMA assets)", len(assets.Items), matched)
	return nil
}
