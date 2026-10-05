package commanderclient

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReverseQueriesDoNotRaceWithSaves hammers SaveDraft on entities that reverse
// queries are reading: as the queried entity, as a returned parent, and as a returned
// referrer that results are sorted by. Callers then read IDs of the results.
func TestReverseQueriesDoNotRaceWithSaves(t *testing.T) {
	ctx := context.Background()
	fake := newFakeContentful(t)
	for _, id := range []string{"p1", "p2", "p3"} {
		fake.putEntry(newFakeEntry(id, "ct0", map[string]any{"f1": map[string]any{"en-US": fakeLink("child")}}), false)
	}
	fake.putEntry(newFakeEntry("child", "ct0", map[string]any{"f1": map[string]any{"en-US": fakeLink("p1")}}), false)
	mc := fake.newClient(false)
	require.NoError(t, mc.LoadSpaceModel(ctx, NewLogger(false)))

	var wg sync.WaitGroup
	for _, id := range []string{"p1", "p2", "p3", "child"} {
		wg.Go(func() {
			entity, _ := mc.GetEntity(id)
			for range 100 {
				_ = mc.SaveDraft(ctx, entity)
			}
		})
	}
	for range 4 {
		wg.Go(func() {
			child, _ := mc.GetEntity("child")
			p1, _ := mc.GetEntity("p1")
			for range 300 {
				for _, parent := range child.GetParents(nil).Get() {
					_ = parent.GetID()
				}
				_ = p1.(*EntryEntity).GetParentsViaFields([]string{"f1"}, []string{"ct0"})
				_, _ = p1.GetReferrerPath([]string{"f1"})
				for _, referrer := range mc.GetReferrers("child", ReferrerFilter{ContentTypes: []string{"ct0"}}) {
					_ = referrer.Entity.GetID()
				}
			}
		})
	}
	wg.Wait()
}
