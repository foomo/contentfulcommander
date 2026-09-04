package commanderclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"time"

	"github.com/foomo/contentful"
)

const (
	// defaultAssetProcessTimeout caps the wait for asset processing when the caller's
	// context carries no deadline of its own.
	defaultAssetProcessTimeout = 2 * time.Minute
	// assetProcessPollInterval is how often the asset is re-fetched while waiting for
	// Contentful to finish processing the uploaded file.
	assetProcessPollInterval = time.Second
	// defaultAssetFileName is used when neither the URL nor the asset ID yields a name.
	defaultAssetFileName = "asset"
)

// CreateAssetFromURL creates a draft asset whose file Contentful fetches from assetURL.
// It upserts the draft, triggers processing and waits until processing has finished,
// so the returned asset already carries its CDN URL.
//
// Pass an empty id to let Contentful generate one, and an empty locale to use the space
// default locale. The file name is derived from assetURL, falling back to the asset ID
// when the URL has no usable last path segment.
//
// The wait is bounded by ctx; when ctx has no deadline, processing may take at most
// defaultAssetProcessTimeout. Creating an asset with an ID that already exists is an
// error — use SaveDraft to modify an existing asset.
func (mc *MigrationClient) CreateAssetFromURL(ctx context.Context, id, assetURL, contentType, title string, locale Locale) (*AssetEntity, error) {
	if mc == nil {
		return nil, fmt.Errorf("migration client is nil")
	}
	if mc.cma == nil {
		return nil, fmt.Errorf("migration client has no CMA client")
	}
	if assetURL == "" {
		return nil, fmt.Errorf("asset URL is required")
	}
	if contentType == "" {
		return nil, fmt.Errorf("content type is required")
	}
	// Assets.Process iterates the title locales, so an asset without a title for the
	// locale is never processed and the wait below would time out with no explanation.
	if title == "" {
		return nil, fmt.Errorf("title is required: Contentful only processes locales that have one")
	}

	if locale == "" {
		locale = mc.GetDefaultLocale()
		if locale == "" {
			return nil, fmt.Errorf("no locale given and no default locale available: load the space model first")
		}
	}
	localeCode := string(locale)

	asset := &contentful.Asset{
		Sys: &contentful.Sys{ID: id},
		Fields: &contentful.FileFields{
			Title: map[string]string{localeCode: title},
			File: map[string]*contentful.File{
				localeCode: {
					Name:        assetFileName(assetURL, id),
					ContentType: contentType,
					UploadURL:   assetURL,
				},
			},
		},
	}

	// Upsert decodes the response into asset, so it picks up the generated ID (when id
	// was empty) and the version that Process needs below.
	if err := mc.cma.Assets.Upsert(ctx, mc.spaceID, asset); err != nil {
		var mismatch contentful.VersionMismatchError
		if errors.As(err, &mismatch) {
			return nil, fmt.Errorf("asset %q already exists", id)
		}
		return nil, fmt.Errorf("failed to create asset %q: %w", id, err)
	}

	if err := mc.cma.Assets.Process(ctx, mc.spaceID, asset); err != nil {
		return nil, fmt.Errorf("failed to process asset %q: %w", asset.Sys.ID, err)
	}

	processed, err := mc.awaitAssetProcessing(ctx, asset.Sys.ID, localeCode)
	if err != nil {
		return nil, err
	}

	entity := &AssetEntity{Asset: processed, Client: mc}

	mc.cacheMu.Lock()
	mc.cache[entity.GetID()] = entity
	if mc.spaceModel != nil {
		mc.spaceModel.Assets[entity.GetID()] = entity
	}
	mc.cacheMu.Unlock()

	return entity, nil
}

// CreateAssetFromURLAndPublish creates a draft asset from a remote URL and publishes it.
// See CreateAssetFromURL for the parameters and the processing wait.
//
// When creation succeeds but publishing does not, the asset exists as a draft and is in
// the cache — retrieve it with GetEntity and publish it separately.
func (mc *MigrationClient) CreateAssetFromURLAndPublish(ctx context.Context, id, assetURL, contentType, title string, locale Locale) (*AssetEntity, error) {
	entity, err := mc.CreateAssetFromURL(ctx, id, assetURL, contentType, title, locale)
	if err != nil {
		return nil, err
	}

	if err := mc.Publish(ctx, entity); err != nil {
		return nil, fmt.Errorf("asset %q was created but not published: %w", entity.GetID(), err)
	}

	return entity, nil
}

// awaitAssetProcessing polls the asset until Contentful has replaced the upload URL with
// the CDN URL for the given locale. Processing is asynchronous, and the version the
// process call left behind is stale, so the polled asset is what callers should keep.
func (mc *MigrationClient) awaitAssetProcessing(ctx context.Context, assetID, localeCode string) (*contentful.Asset, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultAssetProcessTimeout)
		defer cancel()
	}

	start := time.Now()
	ticker := time.NewTicker(assetProcessPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("asset %q was not processed within %s: %w", assetID, time.Since(start).Round(time.Millisecond), ctx.Err())
		case <-ticker.C:
		}

		processed, err := mc.cma.Assets.Get(ctx, mc.spaceID, assetID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch asset %q while waiting for processing: %w", assetID, err)
		}
		if processed.Fields != nil {
			if file, ok := processed.Fields.File[localeCode]; ok && file != nil && file.URL != "" {
				return processed, nil
			}
		}
	}
}

// assetFileName derives the file name from the URL's last path segment, falling back to
// the asset ID when the URL carries nothing usable (a bare host, or a path-less download
// endpoint such as https://example.com/download?id=99). Contentful rejects an empty file
// name, so a generic name is used when the ID is generated rather than caller-supplied.
func assetFileName(assetURL, fallback string) string {
	if fallback == "" {
		fallback = defaultAssetFileName
	}

	parsed, err := url.Parse(assetURL)
	if err != nil {
		return fallback
	}

	name := path.Base(parsed.Path)
	if name == "" || name == "." || name == "/" {
		return fallback
	}
	return name
}
