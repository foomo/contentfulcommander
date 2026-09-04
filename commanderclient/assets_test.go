package commanderclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testAssetID     = "moonsample"
	testAssetURL    = "https://example.com/img/moon.jpg"
	testAssetType   = "image/jpeg"
	testAssetTitle  = "Moon"
	testAssetLocale = "en-US"
	testAssetCDNURL = "//images.ctfassets.net/space/moonsample/moon.jpg"
)

// assetJSON renders an asset whose file carries either the pending upload URL or, once
// processed, the CDN URL.
func assetJSON(version int, processed bool) string {
	file := `"upload":"` + testAssetURL + `"`
	if processed {
		file = `"url":"` + testAssetCDNURL + `"`
	}
	return fmt.Sprintf(
		`{"sys":{"id":%q,"type":"Asset","version":%d},"fields":{"title":{%q:%q},"file":{%q:{"fileName":"moon.jpg","contentType":%q,%s}}}}`,
		testAssetID, version, testAssetLocale, testAssetTitle, testAssetLocale, testAssetType, file,
	)
}

// assetServer serves the create/process/poll flow. The file is reported as processed
// from getsUntilProcessed onwards; a negative value means never.
type assetServer struct {
	mu       sync.Mutex
	requests []string
	upsert   []byte
	gets     int
}

func (s *assetServer) record(method, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, method+" "+path)
}

func (s *assetServer) recorded() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

// roundTripFunc serves requests in-process, so these tests need no listening socket.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// serveInProcess turns an http.Handler into a RoundTripper.
func serveInProcess(handler http.Handler) http.RoundTripper {
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, r)
		response := recorder.Result()
		response.Request = r
		return response, nil
	})
}

func newAssetHandler(t *testing.T, getsUntilProcessed int) (http.Handler, *assetServer) {
	t.Helper()

	state := &assetServer{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suffix := r.URL.Path[strings.Index(r.URL.Path, "/assets"):]
		state.record(r.Method, suffix)

		switch {
		case r.Method == http.MethodPut && strings.HasSuffix(suffix, "/process"):
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && strings.HasSuffix(suffix, "/published"):
			_, _ = io.WriteString(w, assetJSON(4, true))
		case r.Method == http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			state.mu.Lock()
			state.upsert = body
			state.mu.Unlock()
			_, _ = io.WriteString(w, assetJSON(1, false))
		case r.Method == http.MethodGet:
			state.mu.Lock()
			state.gets++
			done := getsUntilProcessed >= 0 && state.gets >= getsUntilProcessed
			state.mu.Unlock()
			if done {
				_, _ = io.WriteString(w, assetJSON(3, true))
				return
			}
			_, _ = io.WriteString(w, assetJSON(1, false))
		default:
			t.Errorf("unexpected request %s %s", r.Method, suffix)
		}
	})

	return handler, state
}

func newAssetTestClient(t *testing.T, handler http.Handler) *MigrationClient {
	t.Helper()

	client := newMigrationClient("test-key", "", "test-space", "master")
	client.cma.SetBaseURL("https://cma.test")
	client.cma.SetHTTPTransport(serveInProcess(handler))
	client.spaceModel = &SpaceModel{
		SpaceID:       "test-space",
		Environment:   "master",
		DefaultLocale: testAssetLocale,
		Entries:       make(map[string]Entity),
		Assets:        make(map[string]Entity),
	}
	return client
}

func TestCreateAssetFromURL(t *testing.T) {
	handler, state := newAssetHandler(t, 2)

	client := newAssetTestClient(t, handler)

	entity, err := client.CreateAssetFromURL(
		context.Background(), testAssetID, testAssetURL, testAssetType, testAssetTitle, testAssetLocale,
	)
	require.NoError(t, err)

	// Upsert, then process, then poll until the CDN url appears.
	assert.Equal(t, []string{
		"PUT /assets/" + testAssetID,
		"PUT /assets/" + testAssetID + "/files/" + testAssetLocale + "/process",
		"GET /assets/" + testAssetID,
		"GET /assets/" + testAssetID,
	}, state.recorded())

	// The upserted draft points Contentful at the remote URL and derives the file name.
	var sent struct {
		Fields struct {
			Title map[string]string `json:"title"`
			File  map[string]struct {
				Name        string `json:"fileName"`
				ContentType string `json:"contentType"`
				UploadURL   string `json:"upload"`
			} `json:"file"`
		} `json:"fields"`
	}
	require.NoError(t, json.Unmarshal(state.upsert, &sent))
	assert.Equal(t, testAssetTitle, sent.Fields.Title[testAssetLocale])
	assert.Equal(t, testAssetURL, sent.Fields.File[testAssetLocale].UploadURL)
	assert.Equal(t, testAssetType, sent.Fields.File[testAssetLocale].ContentType)
	assert.Equal(t, "moon.jpg", sent.Fields.File[testAssetLocale].Name)

	// The returned entity is the polled one, so it carries the post-processing version.
	assert.Equal(t, testAssetID, entity.GetID())
	assert.Equal(t, 3, entity.GetVersion())
	assert.Equal(t, testAssetCDNURL, entity.Asset.Fields.File[testAssetLocale].URL)

	// ...and it is in the cache.
	cached, found := client.GetEntity(testAssetID)
	require.True(t, found)
	assert.Same(t, entity, cached)
	assert.Equal(t, 1, client.GetAssets().Count())
	assert.Len(t, client.GetSpaceModel().Assets, 1)
}

func TestCreateAssetFromURLUsesDefaultLocale(t *testing.T) {
	handler, state := newAssetHandler(t, 1)

	client := newAssetTestClient(t, handler)

	_, err := client.CreateAssetFromURL(
		context.Background(), testAssetID, testAssetURL, testAssetType, testAssetTitle, "",
	)
	require.NoError(t, err)

	assert.Contains(t, state.recorded(), "PUT /assets/"+testAssetID+"/files/"+testAssetLocale+"/process")
}

func TestCreateAssetFromURLAndPublish(t *testing.T) {
	handler, state := newAssetHandler(t, 1)

	client := newAssetTestClient(t, handler)

	entity, err := client.CreateAssetFromURLAndPublish(
		context.Background(), testAssetID, testAssetURL, testAssetType, testAssetTitle, testAssetLocale,
	)
	require.NoError(t, err)

	assert.Equal(t, "PUT /assets/"+testAssetID+"/published", state.recorded()[len(state.recorded())-1])
	assert.Equal(t, testAssetID, entity.GetID())
}

func TestCreateAssetFromURLValidation(t *testing.T) {
	handler, state := newAssetHandler(t, 1)

	tests := []struct {
		name        string
		url         string
		contentType string
		title       string
		wantErr     string
	}{
		{name: "missing url", contentType: testAssetType, title: testAssetTitle, wantErr: "asset URL is required"},
		{name: "missing content type", url: testAssetURL, title: testAssetTitle, wantErr: "content type is required"},
		{name: "missing title", url: testAssetURL, contentType: testAssetType, wantErr: "title is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newAssetTestClient(t, handler)

			_, err := client.CreateAssetFromURL(
				context.Background(), testAssetID, tt.url, tt.contentType, tt.title, testAssetLocale,
			)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}

	assert.Empty(t, state.recorded(), "validation must fail before any request is sent")
}

func TestCreateAssetFromURLWithoutDefaultLocale(t *testing.T) {
	handler, state := newAssetHandler(t, 1)

	// No space model, so there is no default locale to fall back to.
	client := newMigrationClient("test-key", "", "test-space", "master")
	client.cma.SetBaseURL("https://cma.test")
	client.cma.SetHTTPTransport(serveInProcess(handler))

	_, err := client.CreateAssetFromURL(
		context.Background(), testAssetID, testAssetURL, testAssetType, testAssetTitle, "",
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no default locale available")
	assert.Empty(t, state.recorded())
}

func TestCreateAssetFromURLAlreadyExists(t *testing.T) {
	// A PUT with version 0 against an existing asset conflicts.
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"sys":{"type":"Error","id":"VersionMismatch"}}`)
	})

	client := newAssetTestClient(t, handler)

	_, err := client.CreateAssetFromURL(
		context.Background(), testAssetID, testAssetURL, testAssetType, testAssetTitle, testAssetLocale,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `asset "`+testAssetID+`" already exists`)
}

func TestCreateAssetFromURLProcessingTimeout(t *testing.T) {
	handler, _ := newAssetHandler(t, -1) // never reports the file as processed

	client := newAssetTestClient(t, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	_, err := client.CreateAssetFromURL(
		ctx, testAssetID, testAssetURL, testAssetType, testAssetTitle, testAssetLocale,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "was not processed within")
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	_, found := client.GetEntity(testAssetID)
	assert.False(t, found, "a failed creation must not be cached")
}

func TestAssetFileName(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		fallback string
		want     string
	}{
		{name: "plain path", url: "https://example.com/img/moon.jpg", fallback: "id", want: "moon.jpg"},
		{name: "query stripped", url: "https://example.com/img/moon.jpg?v=2", fallback: "id", want: "moon.jpg"},
		{name: "fragment stripped", url: "https://example.com/img/moon.jpg#top", fallback: "id", want: "moon.jpg"},
		{name: "escaped path", url: "https://example.com/img/full%20moon.jpg", fallback: "id", want: "full moon.jpg"},
		{name: "no path", url: "https://example.com", fallback: "id", want: "id"},
		{name: "root path", url: "https://example.com/", fallback: "id", want: "id"},
		{name: "query only", url: "https://example.com/download?id=99", fallback: "id", want: "download"},
		{name: "no path no fallback", url: "https://example.com", fallback: "", want: defaultAssetFileName},
		{name: "unparseable", url: "://nope", fallback: "id", want: "id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, assetFileName(tt.url, tt.fallback))
		})
	}
}
