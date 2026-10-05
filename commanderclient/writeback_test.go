package commanderclient

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/foomo/contentful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCloneSysCoversPointerFields fails when the SDK adds a pointer field to Sys that
// cloneSys does not deep-copy: a shared pointer would let a CMA response decoded into a
// detached copy write into the cached entity outside the cache lock.
func TestCloneSysCoversPointerFields(t *testing.T) {
	covered := []string{"UpdatedBy", "ArchivedBy", "ContentType", "Space", "PublishedBy"}
	var pointers []string
	sysType := reflect.TypeFor[contentful.Sys]()
	for i := range sysType.NumField() {
		if kind := sysType.Field(i).Type.Kind(); kind == reflect.Pointer || kind == reflect.Map || kind == reflect.Slice {
			pointers = append(pointers, sysType.Field(i).Name)
		}
	}
	assert.ElementsMatch(t, covered, pointers)
}

func TestDetachedCopySharesNothing(t *testing.T) {
	sys := func() *contentful.Sys {
		return &contentful.Sys{
			ID: "e", Version: 3,
			UpdatedBy:   &contentful.Sys{ID: "user"},
			ArchivedBy:  &contentful.Sys{ID: "user"},
			PublishedBy: &contentful.Sys{ID: "user"},
			ContentType: &contentful.ContentType{Sys: &contentful.Sys{ID: "ct"}},
			Space:       &contentful.Space{Sys: &contentful.Sys{ID: "space"}},
		}
	}
	entity := &EntryEntity{Entry: &contentful.Entry{
		Sys:      sys(),
		Metadata: &contentful.Metadata{Tags: []contentful.Tag{{Sys: &contentful.Sys{ID: "tag"}}}},
		Fields: map[string]any{
			"children": map[string]any{"en-US": []any{fakeLink("a"), []any{fakeLink("b")}}},
			"title":    map[string]any{"en-US": "t"},
		},
	}}
	pristine, err := json.Marshal(entity.Entry)
	require.NoError(t, err)
	sysBefore, fieldsBefore := entity.Entry.Sys, entity.Entry.Fields

	mc := newMigrationClient("key", "", "space", "master")
	work, err := mc.detachEntity(entity)
	require.NoError(t, err)

	// Decode a response into the copy the way the SDK does, overwriting every nested object.
	written := work.(*EntryEntity).Entry
	response := `{"metadata":{"tags":[{"sys":{"id":"tag2"}}]},"sys":{"id":"e","version":4,"updatedBy":{"id":"x"},"archivedBy":{"id":"x"},"publishedBy":{"id":"x"},
		"contentType":{"sys":{"id":"x"}},"space":{"sys":{"id":"x"}}},"fields":{"children":{"en-US":[{"sys":{"id":"c"}}]}}}`
	require.NoError(t, json.Unmarshal([]byte(response), &written))
	after, err := json.Marshal(entity.Entry)
	require.NoError(t, err)
	assert.JSONEq(t, string(pristine), string(after), "decoding into the copy must not touch the original")

	mc.applyDetached(entity, work, true)
	assert.Same(t, sysBefore, entity.Entry.Sys, "Sys keeps its identity")
	assert.Equal(t, 4, entity.Entry.Sys.Version)
	assert.Equal(t, "tag2", entity.Entry.Metadata.Tags[0].Sys.ID)
	assert.Equal(t, reflect.ValueOf(fieldsBefore).Pointer(), reflect.ValueOf(entity.Entry.Fields).Pointer(), "the field map keeps its identity")
	assert.Equal(t, map[string]any{"en-US": []any{map[string]any{"sys": map[string]any{"id": "c"}}}}, entity.Entry.Fields["children"])
	assert.Equal(t, map[string]any{"en-US": "t"}, entity.Entry.Fields["title"], "fields absent from the response are kept")
}

func TestSaveDraftAppliesResponseAndReindexes(t *testing.T) {
	ctx := context.Background()
	fake := newFakeContentful(t)
	fake.putEntry(newFakeEntry("parent", "ct0", map[string]any{"f1": map[string]any{"en-US": fakeLink("child")}}), false)
	mc := fake.newClient(false)
	require.NoError(t, mc.LoadSpaceModel(ctx, NewLogger(false)))
	require.Len(t, mc.GetReferrers("child", ReferrerFilter{}), 1)

	parent, _ := mc.GetEntity("parent")
	sysBefore := parent.GetSys()
	parent.(*EntryEntity).Entry.Fields["f1"] = map[string]any{"en-US": fakeLink("other")} // unobserved edit
	require.NoError(t, mc.SaveDraft(ctx, parent))

	assert.Same(t, sysBefore, parent.GetSys())
	assert.Equal(t, 2, parent.GetVersion(), "the response version is applied to the entity")
	assert.Empty(t, mc.GetReferrers("child", ReferrerFilter{}))
	assert.Len(t, mc.GetReferrers("other", ReferrerFilter{}), 1)
}
