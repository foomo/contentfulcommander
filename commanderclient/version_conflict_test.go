package commanderclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/foomo/contentful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const versionMismatchBody = `{"sys":{"type":"Error","id":"VersionMismatch"}}`

func entryJSON(id string, version int) string {
	return `{"sys":{"id":"` + id + `","type":"Entry","version":` +
		strconv.Itoa(version) + `,"publishedVersion":1,"contentType":{"sys":{"id":"ct"}}},` +
		`"fields":{"title":{"en-US":"server-side"}}}`
}

func newConflictTestEntry(id string) *EntryEntity {
	return &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID:               id,
				Version:          2,
				PublishedVersion: 1,
				ContentType:      &contentful.ContentType{Sys: &contentful.Sys{ID: "ct"}},
			},
			Fields: map[string]any{"title": map[string]any{"en-US": "edited"}},
		},
	}
}

// isPublish reports whether the request targets the .../published endpoint.
func isPublish(r *http.Request) bool { return strings.HasSuffix(r.URL.Path, "/published") }

func TestPublishRetriesOnVersionConflict(t *testing.T) {
	var publishPUTs, gets int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			gets++
			_, _ = io.WriteString(w, entryJSON("e1", 5))
		case r.Method == http.MethodPut && isPublish(r):
			publishPUTs++
			if publishPUTs == 1 {
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, versionMismatchBody)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newMigrationClient("test-key", "", "test-space", "master")
	client.cma.SetBaseURL(server.URL)

	entity := newConflictTestEntry("e1")
	err := client.Publish(context.Background(), entity)

	require.NoError(t, err)
	assert.Equal(t, 2, publishPUTs, "should publish once, then retry once after refreshing version")
	assert.GreaterOrEqual(t, gets, 1, "should re-fetch the entity to refresh its version")
	assert.Equal(t, 5, entity.Entry.Sys.Version, "in-memory version should be refreshed from the server")
}

func TestSaveDraftRetriesOnVersionConflictPreservingFields(t *testing.T) {
	var upsertPUTs int
	var retryBody, retryVersionHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			// Server reports a newer version with different field content; only the
			// version should be adopted, never the server-side fields.
			_, _ = io.WriteString(w, entryJSON("e2", 9))
		case r.Method == http.MethodPut && !isPublish(r):
			upsertPUTs++
			if upsertPUTs == 1 {
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, versionMismatchBody)
				return
			}
			body, _ := io.ReadAll(r.Body)
			retryBody = string(body)
			retryVersionHeader = r.Header.Get("X-Contentful-Version")
			_, _ = io.WriteString(w, entryJSON("e2", 10))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newMigrationClient("test-key", "", "test-space", "master")
	client.cma.SetBaseURL(server.URL)

	entity := newConflictTestEntry("e2")
	err := client.SaveDraft(context.Background(), entity)

	require.NoError(t, err)
	assert.Equal(t, 2, upsertPUTs, "should upsert once, then retry once")
	assert.Equal(t, "9", retryVersionHeader, "retry must use the refreshed version")
	assert.Contains(t, retryBody, "edited", "retry must preserve the locally-edited field")
}

func TestPublishSurfacesPersistentVersionConflict(t *testing.T) {
	var publishPUTs int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			_, _ = io.WriteString(w, entryJSON("e3", 5))
		case r.Method == http.MethodPut && isPublish(r):
			publishPUTs++
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, versionMismatchBody)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newMigrationClient("test-key", "", "test-space", "master")
	client.cma.SetBaseURL(server.URL)

	err := client.Publish(context.Background(), newConflictTestEntry("e3"))

	require.Error(t, err)
	assert.Equal(t, 2, publishPUTs, "must retry exactly once, not loop")
	var mismatch contentful.VersionMismatchError
	assert.ErrorAs(t, err, &mismatch, "the surfaced error should be a version mismatch")
}

func TestPublishDoesNotRetryNonConflictError(t *testing.T) {
	var publishPUTs, gets int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			gets++
			_, _ = io.WriteString(w, entryJSON("e4", 5))
		case r.Method == http.MethodPut && isPublish(r):
			publishPUTs++
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = io.WriteString(w, `{"sys":{"type":"Error","id":"InvalidEntry"},"message":"nope"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newMigrationClient("test-key", "", "test-space", "master")
	client.cma.SetBaseURL(server.URL)

	err := client.Publish(context.Background(), newConflictTestEntry("e4"))

	require.Error(t, err)
	assert.Equal(t, 1, publishPUTs, "non-conflict errors must not be retried")
	assert.Equal(t, 0, gets, "non-conflict errors must not trigger a version refresh")
}
