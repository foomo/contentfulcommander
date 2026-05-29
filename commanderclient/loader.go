package commanderclient

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/foomo/contentful"
	"golang.org/x/sync/errgroup"
)

const (
	initialEntryPageSize        uint16 = 1000
	initialEntryLoadConcurrency int    = 3
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
		limit = initialEntryPageSize
	}
	entries, err := mc.loadEntriesByContentType(ctx, mc.cma, contentTypeIDs(spaceModel), limit, logger, "CMA")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		spaceModel.Entries[entry.Sys.ID] = &EntryEntity{Entry: &entry, Client: mc}
	}
	mc.stats.ProcessedEntries += len(entries)
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
		limit = initialEntryPageSize
	}
	entries, err := mc.loadEntriesByContentType(ctx, mc.cda, contentTypeIDs(spaceModel), limit, logger, "CDA")
	if err != nil {
		return err
	}
	matched := 0
	for _, entry := range entries {
		if cmaEntity, ok := spaceModel.Entries[entry.Sys.ID]; ok {
			if ee, ok := cmaEntity.(*EntryEntity); ok {
				ee.cdaView = &EntryEntity{Entry: &entry, Client: mc}
				matched++
			}
		}
	}
	logger.Info("Loaded %d CDA entries (%d matched CMA entries)", len(entries), matched)
	return nil
}

func (mc *MigrationClient) loadEntriesByContentType(ctx context.Context, client *contentful.Contentful, contentTypes []string, limit uint16, logger *Logger, source string) ([]contentful.Entry, error) {
	if len(contentTypes) == 0 {
		return nil, nil
	}

	contentTypeCh := make(chan string)
	var mu sync.Mutex
	var entries []contentful.Entry

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		defer close(contentTypeCh)
		for _, contentType := range contentTypes {
			select {
			case contentTypeCh <- contentType:
			case <-gCtx.Done():
				return gCtx.Err()
			}
		}
		return nil
	})

	for range initialEntryLoadConcurrency {
		g.Go(func() error {
			for contentType := range contentTypeCh {
				contentTypeEntries, pageSize, err := mc.loadEntriesForContentType(gCtx, client, contentType, limit)
				if err != nil {
					return fmt.Errorf("failed to load %s entries for content type %s: %w", source, contentType, err)
				}
				mu.Lock()
				entries = append(entries, contentTypeEntries...)
				mu.Unlock()
				logger.Info("Loaded %d %s entries for content type %s (page size %d)", len(contentTypeEntries), source, contentType, pageSize)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (mc *MigrationClient) loadEntriesForContentType(ctx context.Context, client *contentful.Contentful, contentType string, limit uint16) ([]contentful.Entry, uint16, error) {
	if limit == 0 {
		limit = initialEntryPageSize
	}

	entries, err := mc.loadEntriesForContentTypeAtLimit(ctx, client, contentType, limit)
	if err == nil {
		return entries, limit, nil
	}
	if !isEntryPageSizeError(err) || limit <= 1 {
		return nil, limit, err
	}
	return mc.loadEntriesForContentType(ctx, client, contentType, limit/2)
}

func (mc *MigrationClient) loadEntriesForContentTypeAtLimit(ctx context.Context, client *contentful.Contentful, contentType string, limit uint16) ([]contentful.Entry, error) {
	col := client.Entries.List(ctx, mc.spaceID)
	col.Query.ContentType(contentType).Locale("*").Include(0).Limit(limit)

	var entries []contentful.Entry
	for {
		page, err := col.Next()
		if err != nil {
			return nil, err
		}
		entries = append(entries, page.Items...)
		if uint16(len(page.Items)) < limit {
			break
		}
		col = page
	}
	return entries, nil
}

func contentTypeIDs(spaceModel *SpaceModel) []string {
	contentTypes := make([]string, 0, len(spaceModel.ContentTypes))
	for contentType := range spaceModel.ContentTypes {
		contentTypes = append(contentTypes, contentType)
	}
	sort.Strings(contentTypes)
	return contentTypes
}

func isEntryPageSizeError(err error) bool {
	var contentfulErr contentful.ErrorResponse
	if errors.As(err, &contentfulErr) && isEntryPageSizeErrorMessage(contentfulErr.Message) {
		return true
	}
	return isEntryPageSizeErrorMessage(err.Error())
}

func isEntryPageSizeErrorMessage(msg string) bool {
	return strings.Contains(msg, "Response size too big") ||
		strings.Contains(msg, "Too many links")
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

// updateEntries incrementally loads entries ordered by -sys.updatedAt, stopping
// when it reaches an entry older than the cutoff.
func (mc *MigrationClient) updateEntries(ctx context.Context, cutoff time.Time, logger *Logger) (map[string]*EntryEntity, error) {
	col := mc.cma.Entries.List(ctx, mc.spaceID)
	col.Query.Locale("*").Include(0).Limit(100).Order("sys.updatedAt", true)

	updated := make(map[string]*EntryEntity)
	done := false
	for !done {
		page, err := col.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch entries: %w", err)
		}
		if len(page.Items) == 0 {
			break
		}

		for _, entry := range page.Items {
			updatedAt, parseErr := time.Parse(time.RFC3339, entry.Sys.UpdatedAt)
			if parseErr != nil {
				return nil, fmt.Errorf("failed to parse updatedAt for entry %s: %w", entry.Sys.ID, parseErr)
			}
			if updatedAt.Before(cutoff) {
				done = true
				break
			}
			updated[entry.Sys.ID] = &EntryEntity{Entry: &entry, Client: mc}
		}

		if done || len(page.Items) < 100 {
			break
		}

		col = page
	}

	logger.Info("Updated %d entries incrementally", len(updated))
	return updated, nil
}

// updateAssets incrementally loads assets ordered by -sys.updatedAt, stopping
// when it reaches an asset older than the cutoff.
func (mc *MigrationClient) updateAssets(ctx context.Context, cutoff time.Time, logger *Logger) (map[string]*AssetEntity, error) {
	col := mc.cma.Assets.List(ctx, mc.spaceID)
	col.Query.Locale("*").Limit(100).Order("sys.updatedAt", true)

	updated := make(map[string]*AssetEntity)
	done := false
	for !done {
		page, err := col.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch assets: %w", err)
		}
		if len(page.Items) == 0 {
			break
		}

		for _, asset := range page.Items {
			updatedAt, parseErr := time.Parse(time.RFC3339, asset.Sys.UpdatedAt)
			if parseErr != nil {
				return nil, fmt.Errorf("failed to parse updatedAt for asset %s: %w", asset.Sys.ID, parseErr)
			}
			if updatedAt.Before(cutoff) {
				done = true
				break
			}
			updated[asset.Sys.ID] = &AssetEntity{Asset: &asset, Client: mc}
		}

		if done || len(page.Items) < 100 {
			break
		}

		col = page
	}

	logger.Info("Updated %d assets incrementally", len(updated))
	return updated, nil
}

// updateCDAEntries incrementally loads published entries via CDA ordered by -sys.updatedAt
// and returns them for attachment to matching CMA entities.
func (mc *MigrationClient) updateCDAEntries(ctx context.Context, cutoff time.Time, logger *Logger) (map[string]Entity, error) {
	col := mc.cda.Entries.List(ctx, mc.spaceID)
	col.Query.Locale("*").Include(0).Limit(100).Order("sys.updatedAt", true)

	updated := make(map[string]Entity)
	done := false
	for !done {
		page, err := col.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch CDA entries: %w", err)
		}
		if len(page.Items) == 0 {
			break
		}

		for _, entry := range page.Items {
			updatedAt, parseErr := time.Parse(time.RFC3339, entry.Sys.UpdatedAt)
			if parseErr != nil {
				return nil, fmt.Errorf("failed to parse updatedAt for CDA entry %s: %w", entry.Sys.ID, parseErr)
			}
			if updatedAt.Before(cutoff) {
				done = true
				break
			}
			updated[entry.Sys.ID] = &EntryEntity{Entry: &entry, Client: mc}
		}

		if done || len(page.Items) < 100 {
			break
		}

		col = page
	}

	logger.Info("Updated %d CDA entry views incrementally", len(updated))
	return updated, nil
}

// updateCDAAssets incrementally loads published assets via CDA ordered by -sys.updatedAt
// and returns them for attachment to matching CMA assets.
func (mc *MigrationClient) updateCDAAssets(ctx context.Context, cutoff time.Time, logger *Logger) (map[string]Entity, error) {
	col := mc.cda.Assets.List(ctx, mc.spaceID)
	col.Query.Locale("*").Limit(100).Order("sys.updatedAt", true)

	updated := make(map[string]Entity)
	done := false
	for !done {
		page, err := col.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to fetch CDA assets: %w", err)
		}
		if len(page.Items) == 0 {
			break
		}

		for _, asset := range page.Items {
			updatedAt, parseErr := time.Parse(time.RFC3339, asset.Sys.UpdatedAt)
			if parseErr != nil {
				return nil, fmt.Errorf("failed to parse updatedAt for CDA asset %s: %w", asset.Sys.ID, parseErr)
			}
			if updatedAt.Before(cutoff) {
				done = true
				break
			}
			updated[asset.Sys.ID] = &AssetEntity{Asset: &asset, Client: mc}
		}

		if done || len(page.Items) < 100 {
			break
		}

		col = page
	}

	logger.Info("Updated %d CDA asset views incrementally", len(updated))
	return updated, nil
}
