package commanderclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	localesBody      = `{"sys":{"type":"Array"},"total":1,"skip":0,"limit":100,"items":[{"name":"English","code":"en-US","default":true}]}`
	contentTypesBody = `{"sys":{"type":"Array"},"total":1,"skip":0,"limit":100,"items":[{"sys":{"id":"ct","type":"ContentType"},"name":"CT","fields":[]}]}`
	entriesBody      = `{"sys":{"type":"Array"},"total":1,"skip":0,"limit":1000,"items":[{"sys":{"id":"e1","type":"Entry","version":1,"publishedVersion":1,"updatedAt":"2020-01-01T00:00:00Z","contentType":{"sys":{"id":"ct"}}},"fields":{}}]}`
	assetsBody       = `{"sys":{"type":"Array"},"total":1,"skip":0,"limit":1000,"items":[{"sys":{"id":"a1","type":"Asset","version":1,"publishedVersion":1,"updatedAt":"2020-01-01T00:00:00Z"},"fields":{}}]}`
)

// newLoadScopeServer serves a minimal space (one content type, one entry, one asset)
// and records which collection endpoints were requested.
func newLoadScopeServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()

	var mu sync.Mutex
	var requested []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body string
		switch {
		case strings.HasSuffix(r.URL.Path, "/locales"):
			body = localesBody
		case strings.HasSuffix(r.URL.Path, "/content_types"):
			body = contentTypesBody
		case strings.HasSuffix(r.URL.Path, "/entries"):
			body = entriesBody
		case strings.HasSuffix(r.URL.Path, "/assets"):
			body = assetsBody
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			return
		}

		mu.Lock()
		requested = append(requested, r.URL.Path[strings.LastIndex(r.URL.Path, "/"):])
		mu.Unlock()

		_, _ = io.WriteString(w, body)
	}))

	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), requested...)
	}
}

func TestLoadSpaceModelScope(t *testing.T) {
	tests := []struct {
		name        string
		skipEntries bool
		skipAssets  bool
		wantEntries int
		wantAssets  int
	}{
		{name: "entries and assets", wantEntries: 1, wantAssets: 1},
		{name: "entries only", skipAssets: true, wantEntries: 1, wantAssets: 0},
		{name: "assets only", skipEntries: true, wantEntries: 0, wantAssets: 1},
		{name: "neither", skipEntries: true, skipAssets: true, wantEntries: 0, wantAssets: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, requested := newLoadScopeServer(t)
			defer server.Close()

			client := newMigrationClient("test-key", "", "test-space", "master")
			client.cma.SetBaseURL(server.URL)
			client.skipEntries = tt.skipEntries
			client.skipAssets = tt.skipAssets

			require.NoError(t, client.LoadSpaceModel(context.Background(), NewLogger(false)))

			paths := requested()
			assert.Contains(t, paths, "/locales")
			assert.Contains(t, paths, "/content_types")
			assert.Equal(t, !tt.skipEntries, slices.Contains(paths, "/entries"), "entries endpoint requested")
			assert.Equal(t, !tt.skipAssets, slices.Contains(paths, "/assets"), "assets endpoint requested")

			model := client.GetSpaceModel()
			assert.Len(t, model.Entries, tt.wantEntries)
			assert.Len(t, model.Assets, tt.wantAssets)
			assert.Equal(t, tt.wantEntries, client.GetEntries().Count())
			assert.Equal(t, tt.wantAssets, client.GetAssets().Count())
			assert.Equal(t, tt.wantEntries+tt.wantAssets, client.GetAllEntities().Count())
		})
	}
}

func TestUpdateSpaceModelScope(t *testing.T) {
	server, requested := newLoadScopeServer(t)
	defer server.Close()

	client := newMigrationClient("test-key", "", "test-space", "master")
	client.cma.SetBaseURL(server.URL)
	client.skipEntries = true

	logger := NewLogger(false)
	require.NoError(t, client.LoadSpaceModel(context.Background(), logger))
	require.NoError(t, client.UpdateSpaceModel(context.Background(), logger))

	assert.False(t, slices.Contains(requested(), "/entries"), "update must not fetch entries when entries are skipped")
	assert.Len(t, client.GetSpaceModel().Assets, 1)
}
