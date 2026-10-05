package commanderclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/foomo/contentful"
)

const (
	fakeCMAHost = "cma.test"
	fakeCDAHost = "cda.test"
)

// fakeFault makes the fake fail one item endpoint ("entries/<id>" or "assets/<id>").
type fakeFault struct {
	status       int   // HTTP status with a Contentful error body
	transportErr error // returned by the RoundTripper instead of a response
}

// fakeContentful is a stateful in-process CMA + CDA. The CDA serves the current CMA
// state of every entity that has a published version and is not archived.
type fakeContentful struct {
	t            *testing.T
	mu           sync.Mutex
	locales      []string
	contentTypes []string
	entries      map[string]*contentful.Entry
	assets       map[string]*contentful.Asset
	faults       map[string]fakeFault
	cdaRequests  []string
}

func newFakeContentful(t *testing.T) *fakeContentful {
	t.Helper()
	return &fakeContentful{
		t:            t,
		locales:      []string{"en-US", "de-DE"},
		contentTypes: []string{"ct0"},
		entries:      make(map[string]*contentful.Entry),
		assets:       make(map[string]*contentful.Asset),
		faults:       make(map[string]fakeFault),
	}
}

// newClient returns a client wired to the fake, with a CDA client when withCDA is set.
func (f *fakeContentful) newClient(withCDA bool) *MigrationClient {
	cdaKey := ""
	if withCDA {
		cdaKey = "cda-key"
	}
	client := newMigrationClient("cma-key", cdaKey, "space", "master")
	client.cma.SetBaseURL("https://" + fakeCMAHost)
	client.cma.SetHTTPTransport(f)
	if withCDA {
		client.cda.SetBaseURL("https://" + fakeCDAHost)
		client.cda.SetHTTPTransport(f)
	}
	return client
}

func fakeTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// putEntry stores an entry; touch stamps it as updated now.
func (f *fakeContentful) putEntry(entry *contentful.Entry, touch bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if touch {
		entry.Sys.UpdatedAt = fakeTimestamp(time.Now())
	}
	f.entries[entry.Sys.ID] = entry
}

func (f *fakeContentful) putAsset(asset *contentful.Asset, touch bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if touch {
		asset.Sys.UpdatedAt = fakeTimestamp(time.Now())
	}
	f.assets[asset.Sys.ID] = asset
}

func (f *fakeContentful) deleteEntity(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.entries, id)
	delete(f.assets, id)
}

func (f *fakeContentful) setFault(key string, fault fakeFault) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.faults[key] = fault
}

func (f *fakeContentful) cdaRequestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.cdaRequests)
}

func fakeServesCDA(sys *contentful.Sys) bool {
	return sys.PublishedVersion > 0 && sys.ArchivedVersion == 0
}

// RoundTrip implements http.RoundTripper.
func (f *fakeContentful) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	cda := r.URL.Host == fakeCDAHost
	if cda {
		f.cdaRequests = append(f.cdaRequests, r.URL.Path)
	}
	_, resource, _ := strings.Cut(r.URL.Path, "/environments/master/")
	if fault, ok := f.faults[resource]; ok && !cda {
		if fault.transportErr != nil {
			return nil, fault.transportErr
		}
		return fakeError(r, fault.status), nil
	}
	if r.Method == http.MethodPut && !cda && strings.HasPrefix(resource, "entries/") {
		return f.upsertEntry(r, strings.TrimPrefix(resource, "entries/")), nil
	}
	if r.Method != http.MethodGet {
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL)
		return fakeError(r, http.StatusMethodNotAllowed), nil
	}

	query := r.URL.Query()
	switch {
	case resource == "locales":
		items := make([]map[string]any, len(f.locales))
		for i, code := range f.locales {
			items[i] = map[string]any{"code": code, "name": code, "default": i == 0}
		}
		return fakeJSON(r, fakeArray(items, 0, 100)), nil
	case resource == "content_types":
		items := make([]map[string]any, len(f.contentTypes))
		for i, ct := range f.contentTypes {
			items[i] = map[string]any{"sys": map[string]any{"id": ct, "type": "ContentType"}, "name": ct, "fields": []any{}}
		}
		return fakeJSON(r, fakeArray(items, 0, 100)), nil
	case resource == "entries":
		var items []*contentful.Entry
		for _, entry := range f.entries {
			if ct := query.Get("content_type"); ct != "" && entry.Sys.ContentType.Sys.ID != ct {
				continue
			}
			if cda && !fakeServesCDA(entry.Sys) {
				continue
			}
			items = append(items, entry)
		}
		sortFakeItems(items, query.Get("order"), func(e *contentful.Entry) *contentful.Sys { return e.Sys })
		return fakeJSON(r, fakePage(items, query)), nil
	case resource == "assets":
		var items []*contentful.Asset
		for _, asset := range f.assets {
			if cda && !fakeServesCDA(asset.Sys) {
				continue
			}
			items = append(items, asset)
		}
		sortFakeItems(items, query.Get("order"), func(a *contentful.Asset) *contentful.Sys { return a.Sys })
		return fakeJSON(r, fakePage(items, query)), nil
	case strings.HasPrefix(resource, "entries/"):
		entry, ok := f.entries[strings.TrimPrefix(resource, "entries/")]
		if !ok || (cda && !fakeServesCDA(entry.Sys)) {
			return fakeError(r, http.StatusNotFound), nil
		}
		return fakeJSON(r, entry), nil
	case strings.HasPrefix(resource, "assets/"):
		asset, ok := f.assets[strings.TrimPrefix(resource, "assets/")]
		if !ok || (cda && !fakeServesCDA(asset.Sys)) {
			return fakeError(r, http.StatusNotFound), nil
		}
		if cda {
			// Without a locale the CDA answers in the no-locale asset format.
			return fakeJSON(r, map[string]any{"sys": asset.Sys, "fields": map[string]any{"title": asset.Fields.Title["en-US"]}}), nil
		}
		return fakeJSON(r, asset), nil
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL)
		return fakeError(r, http.StatusNotFound), nil
	}
}

// upsertEntry stores the PUT fields of an existing entry as its new draft version.
func (f *fakeContentful) upsertEntry(r *http.Request, id string) *http.Response {
	current, ok := f.entries[id]
	if !ok {
		return fakeError(r, http.StatusNotFound)
	}
	var body struct {
		Fields map[string]any `json:"fields"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		f.t.Errorf("decode upsert body: %v", err)
		return fakeError(r, http.StatusInternalServerError)
	}
	sys := *current.Sys
	sys.Version++
	sys.UpdatedAt = fakeTimestamp(time.Now())
	updated := &contentful.Entry{Sys: &sys, Fields: body.Fields}
	f.entries[id] = updated
	return fakeJSON(r, updated)
}

func sortFakeItems[T any](items []T, order string, sys func(T) *contentful.Sys) {
	slices.SortFunc(items, func(a, b T) int {
		if order == "-sys.updatedAt" {
			if c := strings.Compare(sys(b).UpdatedAt, sys(a).UpdatedAt); c != 0 {
				return c
			}
		}
		return strings.Compare(sys(a).ID, sys(b).ID)
	})
}

func fakePage[T any](items []T, query map[string][]string) map[string]any {
	skip, _ := strconv.Atoi(first(query["skip"]))
	limit, _ := strconv.Atoi(first(query["limit"]))
	if limit == 0 {
		limit = 100
	}
	end := min(skip+limit, len(items))
	page := []T{}
	if skip < len(items) {
		page = items[skip:end]
	}
	result := fakeArray(page, skip, limit)
	result["total"] = len(items)
	return result
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func fakeArray[T any](items []T, skip, limit int) map[string]any {
	return map[string]any{"sys": map[string]any{"type": "Array"}, "total": len(items), "skip": skip, "limit": limit, "items": items}
}

func fakeJSON(r *http.Request, body any) *http.Response {
	data, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return fakeResponse(r, http.StatusOK, string(data))
}

func fakeError(r *http.Request, status int) *http.Response {
	id := map[int]string{
		http.StatusNotFound:            "NotFound",
		http.StatusTooManyRequests:     "RateLimitExceeded",
		http.StatusInternalServerError: "InternalServerError",
		http.StatusUnauthorized:        "AccessTokenInvalid",
	}[status]
	if id == "" {
		id = "Unknown"
	}
	return fakeResponse(r, status, fmt.Sprintf(`{"sys":{"type":"Error","id":%q},"message":"fake %d"}`, id, status))
}

func fakeResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}
}

var errFakeNetwork = errors.New("fake network failure")

// Entity builders

func fakeLink(id string) map[string]any {
	return map[string]any{"sys": map[string]any{"type": "Link", "linkType": "Entry", "id": id}}
}

func newFakeEntry(id, contentType string, fields map[string]any) *contentful.Entry {
	return &contentful.Entry{
		Sys: &contentful.Sys{
			ID:          id,
			Type:        "Entry",
			Version:     1,
			UpdatedAt:   "2020-01-01T00:00:00Z",
			ContentType: &contentful.ContentType{Sys: &contentful.Sys{ID: contentType}},
		},
		Fields: fields,
	}
}

func newFakeAsset(id string) *contentful.Asset {
	return &contentful.Asset{
		Sys: &contentful.Sys{ID: id, Type: "Asset", Version: 1, UpdatedAt: "2020-01-01T00:00:00Z"},
		Fields: &contentful.FileFields{
			Title: map[string]string{"en-US": "asset " + id},
			File:  map[string]*contentful.File{"en-US": {Name: id + ".jpg", ContentType: "image/jpeg", URL: "//images/" + id}},
		},
	}
}

// Brute-force references: scan the whole cache the way GetParents did before the index,
// using the shared extraction function on GetFields().

type edge struct {
	referrer string
	field    string
	locale   Locale
}

func bruteEdges(mc *MigrationClient, targetID string) []edge {
	mc.cacheMu.RLock()
	defer mc.cacheMu.RUnlock()
	seen := map[edge]struct{}{}
	var edges []edge
	for id, entity := range mc.cache {
		forEachReference(entity.GetFields(), func(target, field string, locale Locale) {
			e := edge{referrer: id, field: field, locale: locale}
			if _, dup := seen[e]; target == targetID && !dup {
				seen[e] = struct{}{}
				edges = append(edges, e)
			}
		})
	}
	return edges
}

func bruteReferrers(mc *MigrationClient, targetID string, filter ReferrerFilter) []edge {
	var result []edge
	for _, e := range bruteEdges(mc, targetID) {
		entity, _ := mc.GetEntity(e.referrer)
		if len(filter.ContentTypes) > 0 && !slices.Contains(filter.ContentTypes, entity.GetContentType()) {
			continue
		}
		if len(filter.FieldNames) > 0 && !slices.Contains(filter.FieldNames, e.field) {
			continue
		}
		result = append(result, edge{referrer: entity.GetID(), field: e.field, locale: e.locale})
	}
	slices.SortFunc(result, func(a, b edge) int {
		return strings.Compare(a.referrer+"\x00"+a.field+"\x00"+string(a.locale), b.referrer+"\x00"+b.field+"\x00"+string(b.locale))
	})
	return result
}

func bruteParents(mc *MigrationClient, targetID string, fieldNames, contentTypes []string) []string {
	parents := map[string]struct{}{}
	for _, e := range bruteEdges(mc, targetID) {
		entity, _ := mc.GetEntity(e.referrer)
		if entity.GetID() == targetID {
			continue
		}
		if contentTypes != nil && !slices.Contains(contentTypes, entity.GetContentType()) {
			continue
		}
		if len(fieldNames) > 0 && !slices.Contains(fieldNames, e.field) {
			continue
		}
		parents[entity.GetID()] = struct{}{}
	}
	ids := make([]string, 0, len(parents))
	for id := range parents {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// brutePath walks the referrer chain with brute-force scans, as GetReferrerPath did
// before the index.
func brutePath(mc *MigrationClient, startID string, fieldNames []string) ([]string, error) {
	path := []string{startID}
	visited := map[string]struct{}{startID: {}}
	currentID := startID
	for {
		parents := bruteParents(mc, currentID, fieldNames, nil)
		if len(parents) == 0 {
			break
		}
		if len(parents) > 1 {
			return nil, ErrAmbiguousPath
		}
		if _, seen := visited[parents[0]]; seen {
			return nil, ErrCircularReference
		}
		visited[parents[0]] = struct{}{}
		path = append(path, parents[0])
		currentID = parents[0]
	}
	slices.Reverse(path)
	return path, nil
}

func entityIDs(collection *EntityCollection) []string {
	ids := []string{}
	for _, entity := range collection.Get() {
		ids = append(ids, entity.GetID())
	}
	return ids
}

func referrerEdges(referrers []Referrer) []edge {
	var edges []edge
	for _, r := range referrers {
		edges = append(edges, edge{referrer: r.Entity.GetID(), field: r.FieldName, locale: r.Locale})
	}
	return edges
}
