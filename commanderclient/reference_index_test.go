package commanderclient

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/foomo/contentful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testFieldNames   = []string{"f0", "f1", "f2", "f3"}
	testContentTypes = []string{"ct0", "ct1", "ct2"}
	testLocales      = []Locale{"en-US", "de-DE"}
	testMissingIDs   = []string{"missing-0", "missing-1"}
)

// spaceGen generates random spaces and edits. Values cover single links, link arrays
// (nested, with duplicates), self and dangling links, JSON objects whose "sys" holds an
// "id" (a link by definition) and values that must not count: rich text with embedded
// links, objects nesting a link deeper, non-string ids.
type spaceGen struct {
	rng       *rand.Rand
	entryIDs  []string
	assetIDs  []string
	nextNewID int
}

func newSpaceGen(seed uint64) *spaceGen {
	return &spaceGen{rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
}

func (g *spaceGen) pick(values []string) string {
	return values[g.rng.IntN(len(values))]
}

func (g *spaceGen) target(self string) string {
	switch g.rng.IntN(10) {
	case 0:
		return self
	case 1:
		return g.pick(testMissingIDs)
	case 2:
		return g.pick(g.assetIDs)
	default:
		return g.pick(g.entryIDs)
	}
}

func (g *spaceGen) value(self string) any {
	switch g.rng.IntN(10) {
	case 0:
		return "text"
	case 1, 2:
		return fakeLink(g.target(self))
	case 3, 4:
		links := []any{}
		for range g.rng.IntN(5) {
			links = append(links, fakeLink(g.target(self)))
		}
		if g.rng.IntN(3) == 0 {
			links = append(links, []any{fakeLink(g.target(self)), "not a link"})
		}
		return links
	case 5:
		return map[string]any{"sys": map[string]any{"id": g.target(self), "type": "Entry"}, "extra": 1.0}
	case 6:
		return map[string]any{"nodeType": "document", "content": []any{map[string]any{
			"nodeType": "embedded-entry-block",
			"data":     map[string]any{"target": fakeLink(g.target(self))},
		}}}
	case 7:
		return map[string]any{"data": fakeLink(g.target(self)), "sys": map[string]any{"id": 42.0}}
	case 8:
		return 42.0
	default:
		return nil
	}
}

func (g *spaceGen) fields(self string) map[string]any {
	fields := map[string]any{}
	for _, name := range testFieldNames {
		if g.rng.IntN(5) == 0 {
			continue
		}
		localeMap := map[string]any{}
		for _, locale := range testLocales {
			if g.rng.IntN(3) > 0 {
				localeMap[string(locale)] = g.value(self)
			}
		}
		fields[name] = localeMap
	}
	return fields
}

func (g *spaceGen) entry(id string) *contentful.Entry {
	entry := newFakeEntry(id, g.pick(testContentTypes), g.fields(id))
	if g.rng.IntN(2) == 0 {
		entry.Sys.PublishedVersion = 1
		entry.Sys.Version = 2
	}
	if g.rng.IntN(8) == 0 {
		entry.Sys.ArchivedVersion = entry.Sys.Version
	}
	return entry
}

func (g *spaceGen) newID() string {
	g.nextNewID++
	id := fmt.Sprintf("new-%d", g.nextNewID)
	g.entryIDs = append(g.entryIDs, id)
	return id
}

// populate fills the fake with a random space. An asset occasionally shares an entry's
// ID: the cache then holds the asset under that key, as LoadSpaceModel does.
func (g *spaceGen) populate(fake *fakeContentful) {
	fake.contentTypes = testContentTypes
	for i := range 8 + g.rng.IntN(16) {
		g.entryIDs = append(g.entryIDs, fmt.Sprintf("e%d", i))
	}
	for i := range 1 + g.rng.IntN(3) {
		g.assetIDs = append(g.assetIDs, fmt.Sprintf("a%d", i))
	}
	if g.rng.IntN(4) == 0 {
		g.assetIDs = append(g.assetIDs, g.pick(g.entryIDs))
	}
	for _, id := range g.entryIDs {
		fake.putEntry(g.entry(id), false)
	}
	for _, id := range g.assetIDs {
		fake.putAsset(newFakeAsset(id), false)
	}
}

// cachedEntryEntities returns the cached entries, sorted by ID for reproducibility.
func cachedEntryEntities(mc *MigrationClient) []*EntryEntity {
	var entries []*EntryEntity
	for _, entity := range mc.GetAllEntities().Get() {
		if entry, ok := entity.(*EntryEntity); ok {
			entries = append(entries, entry)
		}
	}
	slices.SortFunc(entries, func(a, b *EntryEntity) int { return strings.Compare(a.GetID(), b.GetID()) })
	return entries
}

func cachedIDs(mc *MigrationClient) []string {
	mc.cacheMu.RLock()
	defer mc.cacheMu.RUnlock()
	return slices.Sorted(maps.Keys(mc.cache))
}

// assertIndexMatchesBruteForce compares every index-backed query with a brute-force scan.
func assertIndexMatchesBruteForce(t *testing.T, mc *MigrationClient, step string) {
	t.Helper()

	targets := map[string]struct{}{}
	for _, id := range cachedIDs(mc) {
		targets[id] = struct{}{}
	}
	for _, entity := range mc.GetAllEntities().Get() {
		forEachReference(entity.GetFields(), func(targetID, _ string, _ Locale) { targets[targetID] = struct{}{} })
	}
	for _, id := range testMissingIDs {
		targets[id] = struct{}{}
	}

	contentTypeFilters := [][]string{nil, {}, {"ct0"}, {"ct1", "ct2"}}
	fieldFilters := [][]string{nil, {}, {"f1"}, {"f0", "f3"}}
	referrerFilters := []ReferrerFilter{
		{},
		{FieldNames: []string{"f1"}},
		{ContentTypes: []string{"ct0", "ct2"}},
		{ContentTypes: []string{"ct1"}, FieldNames: []string{"f0", "f2"}},
	}

	for _, id := range slices.Sorted(maps.Keys(targets)) {
		entity, cached := mc.GetEntity(id)
		if !cached {
			entity = &EntryEntity{Entry: &contentful.Entry{Sys: &contentful.Sys{ID: id}}, Client: mc}
		}
		viaFields := func(fieldNames, contentTypes []string) *EntityCollection {
			if asset, ok := entity.(*AssetEntity); ok {
				return asset.GetParentsViaFields(fieldNames, contentTypes)
			}
			return entity.(*EntryEntity).GetParentsViaFields(fieldNames, contentTypes)
		}

		for _, contentTypes := range contentTypeFilters {
			want := bruteParents(mc, id, nil, contentTypes)
			require.Equal(t, want, entityIDs(entity.GetParents(contentTypes)), "%s: GetParents(%q, %v)", step, id, contentTypes)
			for _, fieldNames := range fieldFilters {
				require.Equal(t, bruteParents(mc, id, fieldNames, contentTypes), entityIDs(viaFields(fieldNames, contentTypes)),
					"%s: GetParentsViaFields(%q, %v, %v)", step, id, fieldNames, contentTypes)
			}
		}
		for _, filter := range referrerFilters {
			got := mc.GetReferrers(id, filter)
			require.Equal(t, bruteReferrers(mc, id, filter), referrerEdges(got), "%s: GetReferrers(%q, %+v)", step, id, filter)
			for _, referrer := range got {
				cachedReferrer, ok := mc.GetEntity(referrer.Entity.GetID())
				require.True(t, ok)
				require.Same(t, cachedReferrer, referrer.Entity, "%s: referrers are the cached entities", step)
			}
		}
		if cached {
			for _, fieldNames := range [][]string{{"f1"}, {"f0", "f2"}} {
				wantPath, wantErr := brutePath(mc, id, fieldNames)
				path, err := entity.GetReferrerPath(fieldNames)
				require.Equal(t, wantErr, err, "%s: GetReferrerPath(%q, %v) error", step, id, fieldNames)
				if wantErr == nil {
					require.Equal(t, wantPath, entityIDs(NewEntityCollection(path)), "%s: GetReferrerPath(%q, %v)", step, id, fieldNames)
				}
			}
		}
	}

	// The incrementally maintained index must equal one rebuilt from scratch.
	mc.cacheMu.RLock()
	defer mc.cacheMu.RUnlock()
	if mc.refIndex != nil {
		require.Equal(t, normalizeIndex(newReferenceIndex(mc.cache)), normalizeIndex(mc.refIndex), "%s: incremental index drifted", step)
	}
}

func normalizeIndex(idx *referenceIndex) map[string]map[string][]fieldLocale {
	normalized := map[string]map[string][]fieldLocale{}
	for target, byReferrer := range idx.incoming {
		normalized[target] = map[string][]fieldLocale{}
		for referrer, slots := range byReferrer {
			sorted := slices.Clone(slots)
			slices.SortFunc(sorted, func(a, b fieldLocale) int {
				return strings.Compare(a.field+"\x00"+string(a.locale), b.field+"\x00"+string(b.locale))
			})
			normalized[target][referrer] = sorted
		}
	}
	for referrer, targets := range idx.outgoing {
		normalized["\x00outgoing:"+referrer] = map[string][]fieldLocale{}
		for _, target := range targets {
			normalized["\x00outgoing:"+referrer][target] = nil
		}
	}
	return normalized
}

func TestReferenceIndexEquivalence(t *testing.T) {
	ctx := context.Background()
	for seed := range uint64(30) {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			gen := newSpaceGen(seed)
			fake := newFakeContentful(t)
			gen.populate(fake)
			mc := fake.newClient(gen.rng.IntN(2) == 0)
			require.NoError(t, mc.LoadSpaceModel(ctx, NewLogger(false)))
			assertIndexMatchesBruteForce(t, mc, "initial load")

			for step := range 40 {
				var op string
				entries := cachedEntryEntities(mc)
				switch gen.rng.IntN(10) {
				case 0:
					op = "reload"
					for range 3 {
						id := gen.pick(gen.entryIDs)
						fake.putEntry(gen.entry(id), true)
					}
					require.NoError(t, mc.LoadSpaceModel(ctx, NewLogger(false)))
				case 1:
					id := gen.pick(gen.entryIDs)
					if gen.rng.IntN(4) == 0 {
						id = gen.newID()
					}
					op = "refresh-updated " + id
					fake.putEntry(gen.entry(id), true)
					outcome, err := mc.RefreshEntityOutcome(ctx, id)
					require.NoError(t, err)
					require.Equal(t, RefreshOutcomeUpdated, outcome)
				case 2:
					id := gen.pick(append(slices.Clone(gen.entryIDs), testMissingIDs...))
					op = "refresh-removed " + id
					fake.deleteEntity(id)
					outcome, err := mc.RefreshEntityOutcome(ctx, id)
					require.NoError(t, err)
					require.Equal(t, RefreshOutcomeRemoved, outcome)
					_, stillCached := mc.GetEntity(id)
					require.False(t, stillCached)
				case 3:
					ids := cachedIDs(mc)
					if len(ids) == 0 {
						continue
					}
					id := gen.pick(ids)
					op = "remove " + id
					mc.RemoveEntity(id)
				case 4, 5:
					if len(entries) == 0 {
						continue
					}
					entry := entries[gen.rng.IntN(len(entries))]
					field, locale := gen.pick(testFieldNames), testLocales[gen.rng.IntN(len(testLocales))]
					op = fmt.Sprintf("set-field %s.%s.%s", entry.GetID(), field, locale)
					entry.SetFieldValue(field, locale, gen.value(entry.GetID()))
				case 6:
					if len(entries) == 0 {
						continue
					}
					entry := entries[gen.rng.IntN(len(entries))]
					field := gen.pick(testFieldNames)
					op = fmt.Sprintf("direct-edit+reindex %s.%s", entry.GetID(), field)
					switch gen.rng.IntN(3) {
					case 0:
						delete(entry.Entry.Fields, field)
					case 1:
						entry.Entry.Fields[field] = "not a locale map"
					default:
						entry.Entry.Fields[field] = map[string]any{"en-US": gen.value(entry.GetID())}
					}
					var handle Entity = entry
					if gen.rng.IntN(2) == 0 {
						handle = &EntryEntity{Entry: entry.Entry} // an uncached wrapper re-reads the cached entity
					}
					mc.ReindexEntity(handle)
				case 7:
					op = "update-space-model"
					for range gen.rng.IntN(4) {
						id := gen.pick(gen.entryIDs)
						if gen.rng.IntN(3) == 0 {
							id = gen.newID()
						}
						fake.putEntry(gen.entry(id), true)
					}
					require.NoError(t, mc.UpdateSpaceModel(ctx, NewLogger(false)))
				case 8:
					if len(entries) == 0 {
						continue
					}
					entry := entries[gen.rng.IntN(len(entries))]
					if _, inFake := fake.entries[entry.GetID()]; !inFake {
						continue
					}
					// Unobserved edit followed by SaveDraft, which decodes the CMA response into
					// the entity and must re-extract its edges.
					op = "save-draft " + entry.GetID()
					entry.Entry.Fields[gen.pick(testFieldNames)] = map[string]any{"de-DE": gen.value(entry.GetID())}
					require.NoError(t, mc.SaveDraft(ctx, entry))
				default:
					op = "rebuild"
					mc.RebuildReferenceIndex()
				}
				assertIndexMatchesBruteForce(t, mc, fmt.Sprintf("step %d (%s)", step, op))
			}
		})
	}
}

func TestReferenceIndexIsLazy(t *testing.T) {
	ctx := context.Background()
	fake := newFakeContentful(t)
	fake.putEntry(newFakeEntry("parent", "ct0", map[string]any{"f1": map[string]any{"en-US": fakeLink("child")}}), false)
	fake.putEntry(newFakeEntry("child", "ct0", map[string]any{}), false)
	fake.putEntry(newFakeEntry("other", "ct0", map[string]any{}), false)
	mc := fake.newClient(true)

	require.NoError(t, mc.LoadSpaceModel(ctx, NewLogger(false)))
	assert.Nil(t, mc.refIndex, "loading must not build the index")

	// Mutation paths and forward reads leave the index unbuilt.
	child, _ := mc.GetEntity("child")
	child.SetFieldValue("f0", "en-US", fakeLink("parent"))
	require.NoError(t, mc.RefreshEntity(ctx, "other"))
	mc.RemoveEntity("other")
	require.NoError(t, mc.UpdateSpaceModel(ctx, NewLogger(false)))
	mc.ReindexEntity(child)
	mc.RebuildReferenceIndex()
	_ = mc.GetEntries()
	_ = mc.FilterEntities(FilterByContentType("ct0"))
	_, _ = child.GetFieldValueAsReferencedEntity("f0", "en-US")
	assert.Nil(t, mc.refIndex, "only reverse queries build the index")

	assert.Equal(t, []string{"parent"}, entityIDs(child.GetParents(nil)))
	assert.NotNil(t, mc.refIndex, "the first reverse query builds the index")

	// A full reload drops the index; the next query sees the reloaded links.
	fake.putEntry(newFakeEntry("parent", "ct0", map[string]any{"f1": map[string]any{"en-US": fakeLink("other")}}), true)
	fake.putEntry(newFakeEntry("other", "ct0", map[string]any{}), false)
	require.NoError(t, mc.LoadSpaceModel(ctx, NewLogger(false)))
	assert.Nil(t, mc.refIndex)
	reloadedChild, _ := mc.GetEntity("child")
	other, _ := mc.GetEntity("other")
	assert.Empty(t, entityIDs(reloadedChild.GetParents(nil)))
	assert.Equal(t, []string{"parent"}, entityIDs(other.GetParents(nil)))
	assertIndexMatchesBruteForce(t, mc, "after reload")
}

func TestReferenceIndexConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	gen := newSpaceGen(7)
	fake := newFakeContentful(t)
	gen.populate(fake)
	mc := fake.newClient(true)
	require.NoError(t, mc.LoadSpaceModel(ctx, NewLogger(false)))
	require.Nil(t, mc.refIndex)

	// The generator is not goroutine-safe: pre-generate every random input.
	type write struct {
		kind  int
		id    string
		entry *contentful.Entry
		field string
		value any
	}
	// Writer kinds: 0 refresh, 1-2 SetFieldValue, 3 remove, 4 UpdateSpaceModel,
	// 5 LoadSpaceModel / RebuildReferenceIndex (forcing readers to rebuild the index),
	// 6-7 SaveDraft (the CMA response is applied to the cached entry).
	var writes [][]write
	for worker := range 8 {
		var ops []write
		for range 30 {
			id := gen.pick(gen.entryIDs[:len(gen.entryIDs)/2+1]) // overlap the writers
			ops = append(ops, write{kind: worker, id: id, entry: gen.entry(id), field: gen.pick(testFieldNames), value: gen.value(id)})
		}
		writes = append(writes, ops)
	}
	targets := append(slices.Clone(gen.entryIDs), testMissingIDs...)

	start := make(chan struct{})
	var wg sync.WaitGroup
	var saved atomic.Int32
	for reader := range 4 {
		wg.Go(func() {
			<-start
			for i := range 200 {
				id := targets[(i*7+reader)%len(targets)]
				_ = mc.GetReferrers(id, ReferrerFilter{FieldNames: []string{"f1"}})
				_ = mc.GetReferrers(id, ReferrerFilter{ContentTypes: []string{"ct0", "ct1"}})
				if entity, ok := mc.GetEntity(id); ok {
					_ = entity.GetParents(nil)
					if entry, ok := entity.(*EntryEntity); ok {
						_ = entry.GetParentsViaFields([]string{"f0"}, nil)
					}
					_, _ = entity.GetReferrerPath([]string{"f1"})
				}
			}
		})
	}
	for _, ops := range writes {
		wg.Go(func() {
			<-start
			for _, op := range ops {
				switch op.kind {
				case 0:
					fake.putEntry(op.entry, true)
					if err := mc.RefreshEntity(ctx, op.id); err != nil {
						t.Errorf("refresh %s: %v", op.id, err)
					}
				case 1, 2:
					if entity, ok := mc.GetEntity(op.id); ok {
						entity.SetFieldValue(op.field, "en-US", op.value)
					}
				case 3:
					mc.RemoveEntity(op.id)
				case 4:
					fake.putEntry(op.entry, true)
					if err := mc.UpdateSpaceModel(ctx, NewLogger(false)); err != nil {
						t.Errorf("update: %v", err)
					}
				case 6, 7:
					// Errors are expected: a concurrent refresh or update may have replaced
					// the entity with a newer version, or removal may have raced the save.
					if entity, ok := mc.GetEntity(op.id); ok && mc.SaveDraft(ctx, entity) == nil {
						saved.Add(1)
					}
				default:
					if len(op.id)%2 == 0 {
						mc.RebuildReferenceIndex()
					} else if err := mc.LoadSpaceModel(ctx, NewLogger(false)); err != nil {
						t.Errorf("load: %v", err)
					}
				}
			}
		})
	}
	close(start)
	wg.Wait()
	require.Positive(t, saved.Load(), "some concurrent saves must succeed")

	assertIndexMatchesBruteForce(t, mc, "after concurrent access")
}

func TestRefreshEntityOutcomes(t *testing.T) {
	ctx := context.Background()

	// setup caches a referrer linking to "x", and "x" itself linking to "y".
	setup := func(t *testing.T) (*fakeContentful, *MigrationClient) {
		t.Helper()
		fake := newFakeContentful(t)
		fake.putEntry(newFakeEntry("referrer", "ct0", map[string]any{"f1": map[string]any{"en-US": fakeLink("x")}}), false)
		fake.putEntry(newFakeEntry("x", "ct0", map[string]any{"f1": map[string]any{"en-US": fakeLink("y")}}), false)
		mc := fake.newClient(false)
		require.NoError(t, mc.LoadSpaceModel(ctx, NewLogger(false)))
		require.Len(t, mc.GetReferrers("y", ReferrerFilter{}), 1, "builds the index")
		return fake, mc
	}

	t.Run("entry and asset not found removes the entity", func(t *testing.T) {
		fake, mc := setup(t)
		fake.deleteEntity("x")

		err := mc.RefreshEntity(ctx, "x")
		require.ErrorIs(t, err, ErrEntityNotFound)
		var notFound *EntityNotFoundError
		require.ErrorAs(t, err, &notFound)
		assert.Equal(t, "x", notFound.ID)
		assert.Equal(t, "entity x not found", err.Error())

		_, cached := mc.GetEntity("x")
		assert.False(t, cached)
		assert.NotContains(t, mc.GetSpaceModel().Entries, "x")
		assert.Empty(t, mc.GetReferrers("y", ReferrerFilter{}), "outgoing edges of the removed entity are dropped")
		assert.Equal(t, []edge{{"referrer", "f1", "en-US"}}, referrerEdges(mc.GetReferrers("x", ReferrerFilter{})),
			"incoming edges remain: the referrer still links to the dangling ID")

		outcome, err := mc.RefreshEntityOutcome(ctx, "x")
		require.NoError(t, err)
		assert.Equal(t, RefreshOutcomeRemoved, outcome, "removal is reported again for an uncached ID")
	})

	t.Run("entry not found falls through to the asset", func(t *testing.T) {
		fake, mc := setup(t)
		fake.putAsset(newFakeAsset("asset"), false)

		outcome, err := mc.RefreshEntityOutcome(ctx, "asset")
		require.NoError(t, err)
		assert.Equal(t, RefreshOutcomeUpdated, outcome)
		entity, cached := mc.GetEntity("asset")
		require.True(t, cached)
		assert.True(t, entity.IsAsset())
		assert.Contains(t, mc.GetSpaceModel().Assets, "asset")
	})

	failures := []struct {
		name   string
		faults map[string]fakeFault
	}{
		{"entry 500", map[string]fakeFault{"entries/x": {status: 500}, "assets/x": {status: 404}}},
		{"entry 429", map[string]fakeFault{"entries/x": {status: 429}, "assets/x": {status: 404}}},
		{"entry network error", map[string]fakeFault{"entries/x": {transportErr: errFakeNetwork}, "assets/x": {status: 404}}},
		{"entry 401", map[string]fakeFault{"entries/x": {status: 401}, "assets/x": {status: 404}}},
		{"entry 404, asset 500", map[string]fakeFault{"entries/x": {status: 404}, "assets/x": {status: 500}}},
		{"entry 404, asset network error", map[string]fakeFault{"entries/x": {status: 404}, "assets/x": {transportErr: errFakeNetwork}}},
		{"both 500", map[string]fakeFault{"entries/x": {status: 500}, "assets/x": {status: 500}}},
	}
	// The entry lookup failing for a reason other than not found must not fall through
	// to the asset lookup, even when that one would succeed.
	for _, tc := range []struct {
		name  string
		fault fakeFault
	}{
		{"500", fakeFault{status: 500}},
		{"429", fakeFault{status: 429}},
		{"401", fakeFault{status: 401}},
		{"network error", fakeFault{transportErr: errFakeNetwork}},
	} {
		t.Run("entry "+tc.name+", asset found leaves the cache untouched", func(t *testing.T) {
			fake, mc := setup(t)
			fake.putAsset(newFakeAsset("asset"), false)
			require.Equal(t, RefreshOutcomeUpdated, must(mc.RefreshEntityOutcome(ctx, "asset")))
			before, _ := mc.GetEntity("asset")
			updated := newFakeAsset("asset")
			updated.Sys.Version = 2
			fake.putAsset(updated, true)
			fake.setFault("entries/asset", tc.fault)

			outcome, err := mc.RefreshEntityOutcome(ctx, "asset")
			require.Error(t, err)
			assert.Empty(t, outcome)
			after, _ := mc.GetEntity("asset")
			assert.Same(t, before, after)
		})
	}

	for _, tc := range failures {
		t.Run(tc.name+" leaves the cache untouched", func(t *testing.T) {
			fake, mc := setup(t)
			before, _ := mc.GetEntity("x")
			for key, fault := range tc.faults {
				fake.setFault(key, fault)
			}

			outcome, err := mc.RefreshEntityOutcome(ctx, "x")
			require.Error(t, err)
			assert.Empty(t, outcome)
			require.NotErrorIs(t, err, ErrEntityNotFound)
			require.Error(t, mc.RefreshEntity(ctx, "x"))

			after, cached := mc.GetEntity("x")
			require.True(t, cached)
			assert.Same(t, before, after)
			assert.Len(t, mc.GetReferrers("y", ReferrerFilter{}), 1)
		})
	}

	t.Run("network error is wrapped", func(t *testing.T) {
		fake, mc := setup(t)
		fake.setFault("entries/x", fakeFault{transportErr: errFakeNetwork})
		fake.setFault("assets/x", fakeFault{status: 404})
		assert.ErrorIs(t, mc.RefreshEntity(ctx, "x"), errFakeNetwork)
	})
}

func TestReferrerSemantics(t *testing.T) {
	mc := newMigrationClient("key", "", "space", "master")
	add := func(id, contentType string, fields map[string]any) *EntryEntity {
		entity := &EntryEntity{Entry: newFakeEntry(id, contentType, fields), Client: mc}
		mc.cache[id] = entity
		return entity
	}
	self := add("self", "node", map[string]any{
		"children": map[string]any{"en-US": []any{fakeLink("self"), fakeLink("self")}, "de-DE": fakeLink("self")},
	})
	add("b", "node", map[string]any{"children": map[string]any{"en-US": []any{fakeLink("self")}}})
	add("a", "page", map[string]any{
		"hero":     map[string]any{"en-US": fakeLink("self")},
		"children": map[string]any{"en-US": []any{fakeLink("ghost")}},
	})

	t.Run("GetParents excludes self and is sorted", func(t *testing.T) {
		assert.Equal(t, []string{"a", "b"}, entityIDs(self.GetParents(nil)))
		assert.Equal(t, []string{"b"}, entityIDs(self.GetParentsViaFields([]string{"children"}, nil)))
		assert.Equal(t, []string{"a"}, entityIDs(self.GetParentsViaFields(nil, []string{"page"})))
		assert.Empty(t, entityIDs(self.GetParents([]string{})), "a non-nil empty content type list matches nothing")
	})

	t.Run("GetReferrers reports every edge including self-references", func(t *testing.T) {
		assert.Equal(t, []edge{
			{"a", "hero", "en-US"},
			{"b", "children", "en-US"},
			{"self", "children", "de-DE"},
			{"self", "children", "en-US"},
		}, referrerEdges(mc.GetReferrers("self", ReferrerFilter{})))
		assert.Equal(t, []edge{{"a", "hero", "en-US"}},
			referrerEdges(mc.GetReferrers("self", ReferrerFilter{ContentTypes: []string{"page"}, FieldNames: []string{"hero", "x"}})))
	})

	t.Run("GetReferrers answers for uncached IDs", func(t *testing.T) {
		assert.Equal(t, []edge{{"a", "children", "en-US"}}, referrerEdges(mc.GetReferrers("ghost", ReferrerFilter{})))
		assert.Empty(t, mc.GetReferrers("nobody", ReferrerFilter{}))
	})

	t.Run("SetFieldValue on the cached entity reindexes; on a copy it does not", func(t *testing.T) {
		b, _ := mc.GetEntity("b")
		b.SetFieldValue("children", "en-US", []any{fakeLink("ghost")})
		assert.Equal(t, []string{"a"}, entityIDs(self.GetParents(nil)))
		assert.Equal(t, []string{"a", "b"}, entityIDs(NewEntityCollection(entitiesOf(mc.GetReferrers("ghost", ReferrerFilter{})))))

		copied := &EntryEntity{Entry: newFakeEntry("b", "node", map[string]any{}), Client: mc}
		copied.SetFieldValue("children", "en-US", fakeLink("self"))
		assert.Equal(t, []string{"a"}, entityIDs(self.GetParents(nil)), "an uncached copy does not touch the index")
	})

	t.Run("SetFieldValue on a wrapper sharing the cached data reindexes", func(t *testing.T) {
		a, _ := mc.GetEntity("a")
		wrapper := &EntryEntity{Entry: a.(*EntryEntity).Entry, Client: mc} // as UpdateSpaceModel re-wraps entries
		wrapper.SetFieldValue("hero", "en-US", nil)
		assert.Empty(t, entityIDs(self.GetParents(nil)))
	})
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func entitiesOf(referrers []Referrer) []Entity {
	var entities []Entity
	for _, r := range referrers {
		if !slices.Contains(entities, r.Entity) {
			entities = append(entities, r.Entity)
		}
	}
	return entities
}

func TestAssetsHaveNoOutgoingReferences(t *testing.T) {
	asset := &AssetEntity{Asset: newFakeAsset("a")}
	asset.Asset.Fields.Description = map[string]string{"en-US": "d"}
	forEachReference(asset.GetFields(), func(targetID, fieldName string, _ Locale) {
		t.Errorf("asset field %s yielded a reference to %s", fieldName, targetID)
	})
	assert.Nil(t, referenceFields(asset), "skipping asset fields is equivalent to scanning them")
}

func TestCreateAssetFromURLMaintainsIndex(t *testing.T) {
	handler, _ := newAssetHandler(t, 1)
	client := newAssetTestClient(t, handler)
	referrer := &EntryEntity{Entry: newFakeEntry("page", "ct0", map[string]any{"image": map[string]any{"en-US": fakeLink(testAssetID)}}), Client: client}
	client.cache["page"] = referrer
	require.Equal(t, []edge{{"page", "image", "en-US"}}, referrerEdges(client.GetReferrers(testAssetID, ReferrerFilter{})), "dangling before creation")

	asset, err := client.CreateAssetFromURL(context.Background(), testAssetID, testAssetURL, testAssetType, testAssetTitle, testAssetLocale)
	require.NoError(t, err)
	assert.Equal(t, []string{"page"}, entityIDs(asset.GetParents(nil)))
	assertIndexMatchesBruteForce(t, client, "after asset creation")
}

func TestArchivedStatus(t *testing.T) {
	entry := func(version, published, archived int) *EntryEntity {
		return &EntryEntity{Entry: &contentful.Entry{Sys: &contentful.Sys{ID: "e", Version: version, PublishedVersion: published, ArchivedVersion: archived}}}
	}
	asset := func(version, published, archived int) *AssetEntity {
		return &AssetEntity{Asset: &contentful.Asset{Sys: &contentful.Sys{ID: "a", Version: version, PublishedVersion: published, ArchivedVersion: archived}}}
	}

	cases := []struct {
		name                         string
		version, published, archived int
		want                         string
	}{
		{"draft", 1, 0, 0, StatusDraft},
		{"published", 3, 2, 0, StatusPublished},
		{"changed", 4, 2, 0, StatusChanged},
		{"archived draft", 3, 0, 2, StatusArchived},
		{"archived with stale published version", 3, 2, 2, StatusArchived},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, a := entry(tc.version, tc.published, tc.archived), asset(tc.version, tc.published, tc.archived)
			assert.Equal(t, tc.want, e.GetPublishingStatus())
			assert.Equal(t, tc.want, a.GetPublishingStatus())
			assert.Equal(t, tc.archived > 0, e.IsArchived())
			assert.Equal(t, tc.archived > 0, a.IsArchived())
			assert.Equal(t, tc.archived > 0, IsArchived(e))
			assert.Equal(t, tc.archived > 0, IsArchived(a))
		})
	}

	t.Run("package helper is nil-safe and generic", func(t *testing.T) {
		assert.False(t, IsArchived(nil))
		assert.False(t, IsArchived((*EntryEntity)(nil)))
		assert.False(t, IsArchived(&AssetEntity{}))
		assert.False(t, IsArchived(&EntryEntity{Entry: &contentful.Entry{}}))
		type customEntity struct{ *EntryEntity }
		assert.True(t, IsArchived(customEntity{entry(3, 0, 2)}))
	})
}

// Archived entities are returned by unfiltered CMA collections, so LoadSpaceModel caches
// them; UpdateSpaceModel and RefreshEntity must keep them too, without a CDA view.
func TestArchivedMembershipIsConsistent(t *testing.T) {
	ctx := context.Background()
	fake := newFakeContentful(t)
	archived := newFakeEntry("archived", "ct0", map[string]any{})
	archived.Sys.Version, archived.Sys.ArchivedVersion = 3, 2
	published := newFakeEntry("published", "ct0", map[string]any{})
	published.Sys.Version, published.Sys.PublishedVersion = 2, 1
	asset := newFakeAsset("asset")
	asset.Sys.Version, asset.Sys.PublishedVersion = 2, 1
	fake.putEntry(archived, false)
	fake.putEntry(published, false)
	fake.putAsset(asset, false)
	mc := fake.newClient(true)

	require.NoError(t, mc.LoadSpaceModel(ctx, NewLogger(false)))
	assertStatus := func(id, status string) {
		t.Helper()
		entity, ok := mc.GetEntity(id)
		require.True(t, ok, "%s stays cached", id)
		assert.Equal(t, status, entity.GetPublishingStatus(), id)
		assert.Equal(t, status != StatusArchived, entity.HasCDAView(), "%s CDA view", id)
	}
	assertStatus("archived", StatusArchived)
	assertStatus("published", StatusPublished)
	assertStatus("asset", StatusPublished)
	assert.Equal(t, 2, mc.GetEntitiesByContentType("ct0").Count())

	// Archive the published ones (keeping a stale publishedVersion, the adversarial case):
	// the merge must not carry their old CDA views over.
	archivedNow := newFakeEntry("published", "ct0", map[string]any{})
	archivedNow.Sys.Version, archivedNow.Sys.PublishedVersion, archivedNow.Sys.ArchivedVersion = 4, 1, 3
	fake.putEntry(archivedNow, true)
	archivedAsset := newFakeAsset("asset")
	archivedAsset.Sys.Version, archivedAsset.Sys.PublishedVersion, archivedAsset.Sys.ArchivedVersion = 4, 1, 3
	fake.putAsset(archivedAsset, true)
	require.NoError(t, mc.UpdateSpaceModel(ctx, NewLogger(false)))
	assertStatus("published", StatusArchived)
	assertStatus("asset", StatusArchived)

	// RefreshEntity keeps archived entities and does not ask the CDA for them.
	cdaRequests := fake.cdaRequestCount()
	for _, id := range []string{"archived", "published", "asset"} {
		outcome, err := mc.RefreshEntityOutcome(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, RefreshOutcomeUpdated, outcome)
		assertStatus(id, StatusArchived)
	}
	assert.Equal(t, cdaRequests, fake.cdaRequestCount())

	// Unarchived and republished, the entry gets a CDA view again.
	republished := newFakeEntry("published", "ct0", map[string]any{})
	republished.Sys.Version, republished.Sys.PublishedVersion = 6, 5
	fake.putEntry(republished, true)
	require.NoError(t, mc.RefreshEntity(ctx, "published"))
	assertStatus("published", StatusPublished)
}

func TestRefreshEntityErrorIsTyped(t *testing.T) {
	err := error(&EntityNotFoundError{ID: "x"})
	require.ErrorIs(t, fmt.Errorf("wrapped: %w", err), ErrEntityNotFound)
	assert.NotErrorIs(t, errors.New("entity x not found"), ErrEntityNotFound, "string matching is not enough")
}
