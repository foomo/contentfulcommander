package commanderclient

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/foomo/contentful"
)

// loadContentTypes loads all content types from the space
func (mc *MigrationClient) loadContentTypes(ctx context.Context, spaceModel *SpaceModel) error {
	collection := mc.cma.ContentTypes.List(ctx, mc.spaceID)

	for {
		_, err := collection.Next()
		if err != nil {
			return err
		}
		if len(collection.Items) == 0 {
			break
		}

		for _, contentType := range collection.Items {
			// The contentType is interface{}, so we need to marshal and unmarshal it into a *contentful.ContentType
			var ct contentful.ContentType
			if m, ok := contentType.(map[string]interface{}); ok {
				// marshal to JSON then unmarshal
				if b, err := json.Marshal(m); err == nil {
					if err := json.Unmarshal(b, &ct); err == nil {
						spaceModel.ContentTypes[ct.Sys.ID] = &ct
					}
				}
			}
		}
	}
	return nil
}

// loadEntries loads all entries from the space
func (mc *MigrationClient) loadEntries(ctx context.Context, spaceModel *SpaceModel, limit uint16, logger *Logger) error {
	if limit == 0 {
		limit = 512
	}
	collection := mc.cma.Entries.List(ctx, mc.spaceID)
	collection.Query.Locale("*").Include(0).Limit(limit)
	allItems := []interface{}{}

	for {
		_, err := collection.Next()
		if err != nil {
			switch errTyped := err.(type) {
			case contentful.ErrorResponse:
				msg := errTyped.Message
				if (strings.Contains(msg, "Response size too big") || strings.Contains(msg, "Too many links")) && limit >= 20 {
					return mc.loadEntries(ctx, spaceModel, limit/2, logger)
				}
				return errors.New(msg)
			default:
				return err
			}
		}
		allItems = append(allItems, collection.Items...)
		logger.Info("Loaded %d entries", len(allItems))
		// If we got fewer items than the limit, we're done
		if len(collection.Items) < int(limit) {
			break
		}
	}
	for _, entry := range allItems {
		// The entry is not a *contentful.Entry, it's likely a map[string]interface{} and needs to be unmarshalled
		if entryMap, ok := entry.(map[string]interface{}); ok {
			entryBytes, err := json.Marshal(entryMap)
			if err != nil {
				logger.Warn("Failed to marshal entry map: %v", err)
			} else {
				var e contentful.Entry
				if err := json.Unmarshal(entryBytes, &e); err != nil {
					logger.Warn("Failed to unmarshal entry: %v", err)
				} else {
					entity := &EntryEntity{Entry: &e, Client: mc}
					spaceModel.Entries[e.Sys.ID] = entity
					mc.stats.ProcessedEntries++
				}
			}
		}
	}

	return nil
}

// loadAssets loads all assets from the space
func (mc *MigrationClient) loadAssets(ctx context.Context, spaceModel *SpaceModel, logger *Logger) error {
	collection := mc.cma.Assets.List(ctx, mc.spaceID)
	collection.Query.Locale("*").Limit(1000) // Use reasonable batch size
	itemCount := 0
	for {
		_, err := collection.Next()
		if err != nil {
			return err
		}
		if len(collection.Items) == 0 {
			break
		}
		itemCount += len(collection.Items)
		logger.Info("Loaded %d assets", itemCount)

		for _, asset := range collection.Items {
			assetMap, ok := asset.(map[string]interface{})
			if ok {
				assetBytes, err := json.Marshal(assetMap)
				if err != nil {
					logger.Warn("Failed to marshal asset map: %v", err)
				} else {
					var a contentful.Asset
					if err := json.Unmarshal(assetBytes, &a); err != nil {
						logger.Warn("Failed to unmarshal asset: %v", err)
					} else {
						entity := &AssetEntity{Asset: &a, Client: mc}
						spaceModel.Assets[a.Sys.ID] = entity
						mc.stats.ProcessedAssets++
					}
				}
			}
		}

		// If we got fewer items than the limit, we're done
		if len(collection.Items) < 1000 {
			break
		}
	}
	return nil
}
