package commanderclient

// Benchmarks in this file use only APIs that predate the reference index, so the same
// file can be run against an older checkout for before/after comparisons.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/foomo/contentful"
)

const (
	benchCacheSize    = 100_000
	benchContentTypes = 10
)

// benchEntryFields gives entry i five children, a parent, a hub link in two locales,
// a title and a rich-text body with an embedded link (which is not a reference).
func benchEntryFields(rng *rand.Rand, i, size int) map[string]any {
	link := func(id int) map[string]any { return fakeLinkBench(fmt.Sprintf("e%d", id)) }
	children := make([]any, 5)
	for c := range children {
		children[c] = link(rng.IntN(size))
	}
	return map[string]any{
		"title":    map[string]any{"en-US": fmt.Sprintf("Entry %d", i), "de-DE": fmt.Sprintf("Eintrag %d", i)},
		"children": map[string]any{"en-US": children},
		"parent":   map[string]any{"en-US": link(i / 20)},
		"site":     map[string]any{"en-US": link(0), "de-DE": link(0)},
		"body": map[string]any{"en-US": map[string]any{"nodeType": "document", "content": []any{
			map[string]any{"nodeType": "embedded-entry-block", "data": map[string]any{"target": link(rng.IntN(size))}},
		}}},
	}
}

func fakeLinkBench(id string) map[string]any {
	return map[string]any{"sys": map[string]any{"type": "Link", "linkType": "Entry", "id": id}}
}

func benchEntry(rng *rand.Rand, i, size int) *contentful.Entry {
	return &contentful.Entry{
		Sys: &contentful.Sys{
			ID:          fmt.Sprintf("e%d", i),
			Type:        "Entry",
			Version:     2,
			UpdatedAt:   "2020-01-01T00:00:00Z",
			ContentType: &contentful.ContentType{Sys: &contentful.Sys{ID: fmt.Sprintf("ct%d", i%benchContentTypes)}},
		},
		Fields: benchEntryFields(rng, i, size),
	}
}

func newBenchClient(b *testing.B, size int) *MigrationClient {
	b.Helper()
	rng := rand.New(rand.NewPCG(1, 2))
	client := newMigrationClient("key", "", "space", "master")
	for i := range size {
		entity := &EntryEntity{Entry: benchEntry(rng, i, size), Client: client}
		client.cache[entity.GetID()] = entity
	}
	return client
}

// BenchmarkGetParents measures one GetParents call on a 100k-entity cache, for a
// typical target and for the hub every entry links to.
func BenchmarkGetParents(b *testing.B) {
	client := newBenchClient(b, benchCacheSize)
	for _, target := range []string{"e4242", "e0"} {
		b.Run(target, func(b *testing.B) {
			entity, _ := client.GetEntity(target)
			entity.GetParents(nil) // build the index (if any) outside the timing
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				entity.GetParents(nil)
			}
		})
	}
}

// BenchmarkLoadSpaceModel loads 20k entries in 10 content types and 1k assets from an
// in-process fake with pre-rendered pages; no reverse query is made.
func BenchmarkLoadSpaceModel(b *testing.B) {
	const entries, assets = 20_000, 1_000
	rng := rand.New(rand.NewPCG(3, 4))

	byContentType := map[string][]*contentful.Entry{}
	for i := range entries {
		entry := benchEntry(rng, i, entries)
		ct := entry.Sys.ContentType.Sys.ID
		byContentType[ct] = append(byContentType[ct], entry)
	}
	assetItems := make([]*contentful.Asset, assets)
	for i := range assetItems {
		id := fmt.Sprintf("a%d", i)
		assetItems[i] = &contentful.Asset{
			Sys:    &contentful.Sys{ID: id, Type: "Asset", Version: 2, PublishedVersion: 1},
			Fields: &contentful.FileFields{Title: map[string]string{"en-US": id}, File: map[string]*contentful.File{"en-US": {Name: id, URL: "//x/" + id}}},
		}
	}

	pages := map[string]string{}
	render := func(key string, items any, total, skip, limit int) {
		data, err := json.Marshal(map[string]any{"sys": map[string]any{"type": "Array"}, "total": total, "skip": skip, "limit": limit, "items": items})
		if err != nil {
			b.Fatal(err)
		}
		pages[key] = string(data)
	}
	render("locales", []map[string]any{{"code": "en-US", "name": "en-US", "default": true}, {"code": "de-DE", "name": "de-DE"}}, 2, 0, 100)
	var contentTypes []map[string]any
	for ct := range byContentType {
		contentTypes = append(contentTypes, map[string]any{"sys": map[string]any{"id": ct, "type": "ContentType"}, "name": ct, "fields": []any{}})
	}
	render("content_types", contentTypes, len(contentTypes), 0, 100)
	for ct, items := range byContentType {
		for skip := 0; skip <= len(items); skip += int(initialEntryPageSize) {
			render(fmt.Sprintf("entries?%s&%d", ct, skip), items[skip:min(skip+int(initialEntryPageSize), len(items))], len(items), skip, int(initialEntryPageSize))
		}
	}
	// Contentful's GetAll pages assets by the collection limit (100), whatever the query says.
	const assetPage = 100
	for skip := 0; skip <= len(assetItems); skip += assetPage {
		render(fmt.Sprintf("assets?%d", skip), assetItems[skip:min(skip+assetPage, len(assetItems))], len(assetItems), skip, assetPage)
	}

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		_, resource, _ := strings.Cut(r.URL.Path, "/environments/master/")
		skip, _ := strconv.Atoi(r.URL.Query().Get("skip"))
		key := resource
		switch resource {
		case "entries":
			key = fmt.Sprintf("entries?%s&%d", r.URL.Query().Get("content_type"), skip)
		case "assets":
			key = fmt.Sprintf("assets?%d", skip)
		}
		body, ok := pages[key]
		if !ok {
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})

	client := newMigrationClient("key", "", "space", "master")
	client.cma.SetBaseURL("https://cma.test")
	client.cma.SetHTTPTransport(transport)
	logger := NewLogger(false)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := client.LoadSpaceModel(context.Background(), logger); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if got := len(client.cache); got != entries+assets {
		b.Fatalf("loaded %d entities, want %d", got, entries+assets)
	}
}
