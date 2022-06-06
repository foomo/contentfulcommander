package common

import (
	"errors"
	"github.com/foomo/contentful"
	"log"
)

func EntryExistsByID(cma *contentful.Contentful, spaceID, entryID string) bool {
	collection := cma.Entries.List(spaceID)
	collection.Query.SysID(entryID)
	var err error
	collection, err = collection.Next()
	if err != nil {
		log.Fatalf("could not check if new entry ID is already taken: %v", err)
	}
	if collection != nil && len(collection.Items) > 0 {
		return true
	}
	return false
}

func GetEntryByID(cma *contentful.Contentful, spaceID, entryID string) (*contentful.Entry, error) {
	collection := cma.Entries.List(spaceID)
	collection.Query.SysID(entryID)
	var err error
	collection, err = collection.Next()
	if err != nil {
		return nil, err
	}
	convertedEntries := collection.ToEntry()
	if len(convertedEntries) == 0 {
		return nil, errors.New("GetEntryByID could not convert collection to entries")
	}
	return convertedEntries[0], nil
}
func MustGetEntryByID(cma *contentful.Contentful, spaceID, entryID string) *contentful.Entry {
	collection := cma.Entries.List(spaceID)
	collection.Query.SysID(entryID)
	var err error
	collection, err = collection.Next()
	if err != nil {
		log.Fatalf("entry with ID %v not found", entryID)
	}
	convertedEntries := collection.ToEntry()
	if len(convertedEntries) != 1 {
		log.Fatalf("got %d entries", len(convertedEntries))
	}
	return convertedEntries[0]
}

func GetEntriesLinkingToThis(cma *contentful.Contentful, spaceID, entryID string) ([]*contentful.Entry, error) {
	collection := cma.Entries.List(spaceID)
	collection.Query.Equal("links_to_entry", entryID)
	var err error
	collection, err = collection.GetAll()
	if err != nil {
		return nil, err
	}
	return collection.ToEntry(), nil
}

func SmartUpdateEntry(entry *contentful.Entry, refEntry *contentful.Entry, cma *contentful.Contentful, spaceID string) error {
	if entry == nil {
		return errors.New("entry is nil")
	}
	wasPublished := false
	if refEntry != nil {
		if refEntry.Sys.Version-refEntry.Sys.PublishedVersion == 1 {
			wasPublished = true
		}
	} else if entry.Sys.Version-entry.Sys.PublishedVersion == 1 {
		wasPublished = true
	}
	err := cma.Entries.Upsert(spaceID, entry)
	if err != nil {
		return err
	}
	log.Printf("Entry %s was updated", entry.Sys.ID)
	if wasPublished {
		updatedEntry, err := GetEntryByID(cma, spaceID, entry.Sys.ID)
		if err != nil {
			return err
		}
		err = cma.Entries.Publish(spaceID, updatedEntry)
		if err != nil {
			return err
		}
		log.Printf("Entry %s was re-published", entry.Sys.ID)
		return nil
	}
	log.Printf("Entry %s didn't need re-publishing", entry.Sys.ID)
	return nil
}
