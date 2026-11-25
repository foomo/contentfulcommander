package commanderclient

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateEntry(t *testing.T) {
	ctx := context.Background()

	// setup loads a space with "y" and builds the reference index, so a created
	// entry must be added to it incrementally.
	setup := func(t *testing.T) (*fakeContentful, *MigrationClient) {
		t.Helper()
		fake := newFakeContentful(t)
		fake.putEntry(newFakeEntry("y", "ct0", nil), false)
		mc := fake.newClient(false)
		require.NoError(t, mc.LoadSpaceModel(ctx, NewLogger(false)))
		require.Empty(t, mc.GetReferrers("y", ReferrerFilter{}), "builds the index")
		return fake, mc
	}
	fields := map[string]map[Locale]any{
		"title": {"en-US": "New", "de-DE": "Neu"},
		"f1":    {"en-US": fakeLink("y")},
	}

	t.Run("with a generated ID it is a cached draft", func(t *testing.T) {
		fake, mc := setup(t)

		entity, err := mc.CreateEntry(ctx, "", "ct0", fields)
		require.NoError(t, err)

		assert.Equal(t, "generated1", entity.GetID())
		assert.Equal(t, "ct0", entity.GetContentType())
		assert.Equal(t, 1, entity.GetVersion())
		assert.Equal(t, StatusDraft, entity.GetPublishingStatus())
		assert.Equal(t, "Neu", entity.GetFieldValueAsString("title", "de-DE"))
		assert.Contains(t, fake.entries, "generated1")

		cached, ok := mc.GetEntity("generated1")
		require.True(t, ok)
		assert.Same(t, entity, cached)
		assert.Contains(t, mc.GetSpaceModel().Entries, "generated1")
		assert.Equal(t, []edge{{"generated1", "f1", "en-US"}}, referrerEdges(mc.GetReferrers("y", ReferrerFilter{})))
		assertIndexMatchesBruteForce(t, mc, "after create")
	})

	t.Run("with a given ID", func(t *testing.T) {
		fake, mc := setup(t)

		entity, err := mc.CreateEntry(ctx, "teaser-1", "ct0", nil)
		require.NoError(t, err)

		assert.Equal(t, "teaser-1", entity.GetID())
		assert.Contains(t, fake.entries, "teaser-1")
		_, ok := mc.GetEntity("teaser-1")
		assert.True(t, ok)
	})

	t.Run("over an existing ID it fails and leaves the entry", func(t *testing.T) {
		fake, mc := setup(t)
		before, _ := mc.GetEntity("y")

		_, err := mc.CreateEntry(ctx, "y", "ct0", fields)
		require.EqualError(t, err, `entry "y" already exists`)

		after, _ := mc.GetEntity("y")
		assert.Same(t, before, after)
		assert.Nil(t, fake.entries["y"].Fields)
	})

	t.Run("a CMA error is returned and nothing is cached", func(t *testing.T) {
		fake, mc := setup(t)
		fake.setFault("entries", fakeFault{status: http.StatusInternalServerError})

		_, err := mc.CreateEntry(ctx, "", "ct0", fields)
		require.ErrorContains(t, err, `failed to create ct0 entry "": `)
		assert.Len(t, mc.GetSpaceModel().Entries, 1)
	})

	t.Run("an unknown content type is the CMA's error", func(t *testing.T) {
		_, mc := setup(t)

		_, err := mc.CreateEntry(ctx, "", "nope", fields)
		require.ErrorContains(t, err, `failed to create nope entry "": `)
	})

	t.Run("the content type is required", func(t *testing.T) {
		_, mc := setup(t)

		_, err := mc.CreateEntry(ctx, "", "", fields)
		require.EqualError(t, err, "content type is required")
	})
}
