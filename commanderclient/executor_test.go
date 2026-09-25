package commanderclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/foomo/contentful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateEntityPublishesOnlyPublishedEntities(t *testing.T) {
	cases := []struct {
		name             string
		version          int
		publishedVersion int
		wantPublish      bool
	}{
		{name: "never-edited draft", version: 1, publishedVersion: 0, wantPublish: false},
		{name: "published", version: 5, publishedVersion: 4, wantPublish: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var upserts, publishes int
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPut && isPublish(r):
					publishes++
				case r.Method == http.MethodPut:
					upserts++
					_, _ = io.WriteString(w, fmt.Sprintf(
						`{"sys":{"id":"e1","type":"Entry","version":%d,"publishedVersion":%d,"contentType":{"sys":{"id":"ct"}}}}`,
						tc.version+1, tc.publishedVersion,
					))
				case r.Method == http.MethodGet:
					_, _ = io.WriteString(w, entryJSON("e1", tc.version+2))
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
			})

			client := newMigrationClient("test-key", "", "test-space", "master")
			client.cma.SetBaseURL("https://cma.test")
			client.cma.SetHTTPTransport(serveInProcess(handler))

			entity := &EntryEntity{Entry: &contentful.Entry{
				Sys: &contentful.Sys{
					ID:               "e1",
					Version:          tc.version,
					PublishedVersion: tc.publishedVersion,
					ContentType:      &contentful.ContentType{Sys: &contentful.Sys{ID: "ct"}},
				},
				Fields: map[string]any{"title": map[string]any{"en-US": "edited"}},
			}}

			executor := NewMigrationExecutor(client, nil)
			success, err := executor.updateEntity(context.Background(), &MigrationOperation{
				EntityID:  "e1",
				Operation: OperationUpdate,
				Entity:    entity,
			})

			require.NoError(t, err)
			assert.True(t, success)
			assert.Equal(t, 1, upserts, "update must upsert exactly once")
			if tc.wantPublish {
				assert.Equal(t, 1, publishes, "a published entity must be re-published after the upsert")
			} else {
				assert.Zero(t, publishes, "a never-published draft must not be published")
			}
		})
	}
}
