package commanderclient

import (
	"slices"
	"strings"
)

// Reverse-reference index
//
// The client answers "who references ID X" from an index of edges
// "referrer -> target", one per (referrer ID, field name, locale). A link is a field
// value that is a map whose "sys" map holds a string "id", or an array of such values
// (arrays are searched recursively). Every field and every locale counts; rich-text
// documents and other nested objects are not traversed.
//
// The index is built from the cache on the first reverse query (GetParents,
// GetParentsViaFields, GetReferrers, GetReferrerPath) and maintained incrementally
// afterwards, so clients that never ask for referrers pay nothing for it. It is guarded
// by the same lock as the cache. These library paths keep it consistent:
//   - LoadSpaceModel drops it; the next reverse query rebuilds it from the new cache.
//   - UpdateSpaceModel re-extracts every entity whose data the merge replaced.
//   - RefreshEntity / RefreshEntityOutcome re-extract the refreshed entity, or drop the
//     edges of an entity they remove.
//   - RemoveEntity drops the removed entity's outgoing edges. Edges pointing at it
//     remain: its referrers still link to the now dangling ID.
//   - CreateAssetFromURL / CreateAssetFromURLAndPublish index the new asset.
//   - SaveDraft, Publish and the MigrationExecutor operations apply the CMA response
//     to the entity and re-extract its edges in one critical section (writeback.go).
//   - EntryEntity.SetFieldValue re-extracts the entity when it is the cached one.
//
// Edits the library cannot observe are NOT tracked: writes through exported maps
// (Entry.Fields, the live maps returned by GetFields or GetFieldValue) and later
// mutation of a value previously passed to SetFieldValue. After such an edit call
// ReindexEntity for one entity, or RebuildReferenceIndex after bulk edits.

// fieldLocale identifies where in the referrer an edge originates.
type fieldLocale struct {
	field  string
	locale Locale
}

// referenceIndex holds IDs and field/locale names only, never entity copies.
type referenceIndex struct {
	// incoming maps a target ID to the referrers linking to it: referrer ID -> the
	// distinct field/locale pairs holding the link.
	incoming map[string]map[string][]fieldLocale
	// outgoing maps a referrer ID to the distinct targets it links to, so its edges can
	// be removed without scanning incoming.
	outgoing map[string][]string
}

// Referrer is one edge pointing at a target: Entity links to the target through
// FieldName in Locale.
type Referrer struct {
	Entity    Entity
	FieldName string
	Locale    Locale
}

// ReferrerFilter restricts GetReferrers. An empty (nil or zero-length) slice means no
// restriction.
type ReferrerFilter struct {
	// ContentTypes keeps referrers with one of these content type IDs.
	ContentTypes []string
	// FieldNames keeps edges originating from one of these fields.
	FieldNames []string
}

// forEachReference calls fn for every link in fields. It is the single definition of
// what counts as a reference, used by the index and by every reverse query.
func forEachReference(fields map[string]any, fn func(targetID, fieldName string, locale Locale)) {
	for fieldName, fieldValue := range fields {
		localeMap, ok := fieldValue.(map[string]any)
		if !ok {
			continue
		}
		for locale, localeValue := range localeMap {
			forEachValueReference(localeValue, func(targetID string) {
				fn(targetID, fieldName, Locale(locale))
			})
		}
	}
}

// forEachValueReference calls fn for a single link value, or for each link in an array
// (recursively through nested arrays).
func forEachValueReference(value any, fn func(targetID string)) {
	switch v := value.(type) {
	case map[string]any:
		if sysData, ok := v["sys"].(map[string]any); ok {
			if id, ok := sysData["id"].(string); ok {
				fn(id)
			}
		}
	case []any:
		for _, item := range v {
			forEachValueReference(item, fn)
		}
	}
}

// referenceFields returns the fields to extract links from. Asset fields are typed maps
// (map[string]string, map[string]*contentful.File) that never hold a link, so assets
// have no outgoing edges; skipping them also avoids reading asset structs that CMA
// writes decode into.
func referenceFields(entity Entity) map[string]any {
	switch e := entity.(type) {
	case *EntryEntity:
		if e == nil || e.Entry == nil {
			return nil
		}
		return e.Entry.Fields
	case *AssetEntity:
		return nil
	default:
		if isNilEntity(entity) {
			return nil
		}
		return entity.GetFields()
	}
}

func newReferenceIndex(cache map[string]Entity) *referenceIndex {
	idx := &referenceIndex{
		incoming: make(map[string]map[string][]fieldLocale),
		outgoing: make(map[string][]string),
	}
	for id, entity := range cache {
		idx.add(id, entity)
	}
	return idx
}

// add indexes the outgoing edges of referrerID, which must have none indexed yet.
func (idx *referenceIndex) add(referrerID string, entity Entity) {
	var targets []string
	forEachReference(referenceFields(entity), func(targetID, fieldName string, locale Locale) {
		byReferrer := idx.incoming[targetID]
		if byReferrer == nil {
			byReferrer = make(map[string][]fieldLocale, 1)
			idx.incoming[targetID] = byReferrer
		}
		slots, linked := byReferrer[referrerID]
		if !linked {
			targets = append(targets, targetID)
		}
		slot := fieldLocale{field: fieldName, locale: locale}
		if !slices.Contains(slots, slot) {
			byReferrer[referrerID] = append(slots, slot)
		}
	})
	if len(targets) > 0 {
		idx.outgoing[referrerID] = targets
	}
}

// remove drops the outgoing edges of referrerID. Edges pointing at it are kept.
func (idx *referenceIndex) remove(referrerID string) {
	for _, targetID := range idx.outgoing[referrerID] {
		byReferrer := idx.incoming[targetID]
		delete(byReferrer, referrerID)
		if len(byReferrer) == 0 {
			delete(idx.incoming, targetID)
		}
	}
	delete(idx.outgoing, referrerID)
}

// reindex replaces the outgoing edges of referrerID with those of entity.
func (idx *referenceIndex) reindex(referrerID string, entity Entity) {
	idx.remove(referrerID)
	idx.add(referrerID, entity)
}

// withReferenceIndex runs fn with the index built and cacheMu held, so fn sees cache
// and index consistently. fn must not call methods that take cacheMu. The first call
// builds the index under the write lock; later calls only take the read lock.
func (mc *MigrationClient) withReferenceIndex(fn func(idx *referenceIndex)) {
	mc.cacheMu.RLock()
	if mc.refIndex != nil {
		defer mc.cacheMu.RUnlock()
		fn(mc.refIndex)
		return
	}
	mc.cacheMu.RUnlock()

	mc.cacheMu.Lock()
	defer mc.cacheMu.Unlock()
	if mc.refIndex == nil {
		mc.refIndex = newReferenceIndex(mc.cache)
	}
	fn(mc.refIndex)
}

// reindexLocked re-extracts the edges of the entity cached under id, or drops them when
// id is no longer cached. The caller holds cacheMu for writing.
func (mc *MigrationClient) reindexLocked(id string) {
	if mc.refIndex == nil {
		return
	}
	if entity, ok := mc.cache[id]; ok {
		mc.refIndex.reindex(id, entity)
		return
	}
	mc.refIndex.remove(id)
}

// reindexIfCachedLocked re-extracts entity's edges when it is the cached entity (or
// shares its data). The caller holds cacheMu for writing.
func (mc *MigrationClient) reindexIfCachedLocked(entity Entity) {
	if mc.refIndex == nil {
		return
	}
	id := entity.GetID()
	if cached, ok := mc.cache[id]; ok && sameEntityData(cached, entity) {
		mc.refIndex.reindex(id, cached)
	}
}

// sameEntityData reports whether a and b wrap the same Contentful data. Wrappers are
// compared by their payload because UpdateSpaceModel re-wraps cached entries and assets
// when attaching CDA views, leaving several wrappers around one payload.
func sameEntityData(a, b Entity) bool {
	switch x := a.(type) {
	case *EntryEntity:
		y, ok := b.(*EntryEntity)
		return ok && x.Entry == y.Entry
	case *AssetEntity:
		y, ok := b.(*AssetEntity)
		return ok && x.Asset == y.Asset
	default:
		return a == b
	}
}

// ReindexEntity re-extracts the reference edges of the entity cached under
// entity.GetID(). Call it after editing a cached entity's fields directly (through
// Entry.Fields or the maps returned by GetFields / GetFieldValue). The index always
// reflects the cached entity: passing an uncached copy re-reads the cached one, and an
// ID that is no longer cached loses its outgoing edges. It does nothing until the index
// has been built by a reverse query.
func (mc *MigrationClient) ReindexEntity(entity Entity) {
	if mc == nil || isNilEntity(entity) {
		return
	}
	mc.cacheMu.Lock()
	defer mc.cacheMu.Unlock()
	mc.reindexLocked(entity.GetID())
}

// RebuildReferenceIndex rebuilds the reference index from the whole cache. Call it
// after bulk direct field edits that bypassed SetFieldValue. It does nothing until the
// index has been built by a reverse query.
func (mc *MigrationClient) RebuildReferenceIndex() {
	if mc == nil {
		return
	}
	mc.cacheMu.Lock()
	defer mc.cacheMu.Unlock()
	if mc.refIndex != nil {
		mc.refIndex = newReferenceIndex(mc.cache)
	}
}

// GetReferrers returns every edge pointing at targetID, whether or not targetID is
// cached (dangling links are reported too). Unlike GetParents it includes
// self-references, since an entity linking to itself is a legitimate finding, and it
// reports one Referrer per (referrer, field, locale), so a referrer linking through
// several fields or locales appears several times. Repeated links within one field
// value count once. Results are sorted by referrer ID, field name and locale.
func (mc *MigrationClient) GetReferrers(targetID string, filter ReferrerFilter) []Referrer {
	if mc == nil {
		return nil
	}
	// Sort keys are read under the lock: writes apply CMA responses to cached entities
	// under it, so entity data must not be read once it is released.
	type keyed struct {
		id string
		Referrer
	}
	var referrers []keyed
	mc.withReferenceIndex(func(idx *referenceIndex) {
		for referrerID, slots := range idx.incoming[targetID] {
			entity, ok := mc.cache[referrerID]
			if !ok {
				continue
			}
			if len(filter.ContentTypes) > 0 && !slices.Contains(filter.ContentTypes, entity.GetContentType()) {
				continue
			}
			id := entity.GetID()
			for _, slot := range slots {
				if len(filter.FieldNames) > 0 && !slices.Contains(filter.FieldNames, slot.field) {
					continue
				}
				referrers = append(referrers, keyed{id: id, Referrer: Referrer{Entity: entity, FieldName: slot.field, Locale: slot.locale}})
			}
		}
	})
	slices.SortFunc(referrers, func(a, b keyed) int {
		if c := strings.Compare(a.id, b.id); c != 0 {
			return c
		}
		if c := strings.Compare(a.FieldName, b.FieldName); c != 0 {
			return c
		}
		return strings.Compare(string(a.Locale), string(b.Locale))
	})
	if len(referrers) == 0 {
		return nil
	}
	result := make([]Referrer, len(referrers))
	for i, referrer := range referrers {
		result[i] = referrer.Referrer
	}
	return result
}

// getParents returns the distinct cached entities other than target that link to it
// through one of fieldNames (any field when empty), sorted by ID. A non-nil contentTypes
// restricts the parents to those content types. Entity data, including target's ID, is
// read only under the cache lock.
func getParents(client *MigrationClient, target Entity, fieldNames, contentTypes []string) *EntityCollection {
	if client == nil {
		return NewEntityCollection(nil)
	}
	var parents []keyedEntity
	client.withReferenceIndex(func(idx *referenceIndex) {
		targetID := target.GetID()
		for referrerID, slots := range idx.incoming[targetID] {
			entity, ok := client.cache[referrerID]
			if !ok || entity.GetID() == targetID {
				continue
			}
			if contentTypes != nil && !slices.Contains(contentTypes, entity.GetContentType()) {
				continue
			}
			if len(fieldNames) > 0 && !slotsInFields(slots, fieldNames) {
				continue
			}
			parents = append(parents, keyedEntity{id: entity.GetID(), entity: entity})
		}
	})
	return NewEntityCollection(sortedByID(parents))
}

func slotsInFields(slots []fieldLocale, fieldNames []string) bool {
	return slices.ContainsFunc(slots, func(slot fieldLocale) bool {
		return slices.Contains(fieldNames, slot.field)
	})
}

// keyedEntity pairs an entity with its ID read under the cache lock.
type keyedEntity struct {
	id     string
	entity Entity
}

func sortedByID(keyed []keyedEntity) []Entity {
	slices.SortFunc(keyed, func(a, b keyedEntity) int { return strings.Compare(a.id, b.id) })
	entities := make([]Entity, len(keyed))
	for i := range keyed {
		entities[i] = keyed[i].entity
	}
	return entities
}

// GetParentsViaFields returns the entities that reference this entry through one of
// fieldNames (any field when nil or empty), in any locale, excluding the entry itself,
// sorted by ID. A non-nil contentTypes restricts the parents to those content types,
// as in GetParents.
func (ee *EntryEntity) GetParentsViaFields(fieldNames []string, contentTypes []string) *EntityCollection {
	return getParents(ee.Client, ee, fieldNames, contentTypes)
}

// GetParentsViaFields returns the entities that reference this asset through one of
// fieldNames (any field when nil or empty), in any locale, excluding the asset itself,
// sorted by ID. A non-nil contentTypes restricts the parents to those content types,
// as in GetParents.
func (ae *AssetEntity) GetParentsViaFields(fieldNames []string, contentTypes []string) *EntityCollection {
	return getParents(ae.Client, ae, fieldNames, contentTypes)
}
