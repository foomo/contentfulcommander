package chid

import (
	"encoding/json"
	"example.com/contentfulcommander/cmd/common"
	"example.com/contentfulcommander/contentfulclient"
	"github.com/foomo/contentful"
	"log"
)

func Run(cma *contentful.Contentful, params []string) error {
	spaceID, environment := contentfulclient.GetSpaceAndEnvironment(params[0])
	cma.Environment = environment
	oldID := params[1]
	newID := params[2]
	oldEntry := common.MustGetEntryByID(cma, spaceID, oldID)
	if common.EntryExistsByID(cma, spaceID, newID) {
		log.Fatal("An entry with the new ID supplied already exists")
	}
	newEntry := &contentful.Entry{}
	newEntry.Fields = oldEntry.Fields
	newEntry.Sys = &contentful.Sys{
		ID: newID,
		ContentType: &contentful.ContentType{
			Sys: &contentful.Sys{
				ID:       oldEntry.Sys.ContentType.Sys.ID,
				Type:     "Link",
				LinkType: "ContentType",
			},
		},
	}
	parents, err := common.GetEntriesLinkingToThis(cma, spaceID, oldID)
	if err != nil {
		return err
	}
	if len(parents) == 0 {
		log.Printf("None found\n")
	} else {
		log.Printf("Found %d\n", len(parents))
	}
	parentNeedsUpdate := map[string]*contentful.Entry{}
	for _, parent := range parents {
		for fieldName, field := range parent.Fields {
			bytes, err := json.Marshal(field)
			if err != nil {
				return err
			}
			// Try single reference
			singleRefLocalized := map[string]common.ReferenceSys{}
			err = json.Unmarshal(bytes, &singleRefLocalized)
			if err == nil {
				for locale, referenceSys := range singleRefLocalized {
					if referenceSys.Sys.ID == oldID {
						log.Printf("Found a reference in entry %s and field %s", parent.Sys.ID, fieldName)
						newReferenceSys := common.ReferenceSys{
							Sys: common.ReferenceSysAttributes{
								ID:       newID,
								Type:     "Link",
								LinkType: "Entry",
							},
						}
						singleRefLocalized[locale] = newReferenceSys
						parent.Fields[fieldName] = singleRefLocalized
						parentNeedsUpdate[parent.Sys.ID] = parent
					}
				}
			}
			// Try multiple references
			multiRefLocalized := map[string][]common.ReferenceSys{}
			err = json.Unmarshal(bytes, &multiRefLocalized)
			if err == nil {
				for locale, referenceSysSlice := range multiRefLocalized {
					var newReferenceSysMap []common.ReferenceSys
					for _, referenceSys := range referenceSysSlice {
						if referenceSys.Sys.ID == oldID {
							log.Printf("Found a reference in entry %s and field %s", parent.Sys.ID, fieldName)
							newReferenceSys := common.ReferenceSys{
								Sys: common.ReferenceSysAttributes{
									ID:       newID,
									Type:     "Link",
									LinkType: "Entry",
								},
							}
							newReferenceSysMap = append(newReferenceSysMap, newReferenceSys)
							parent.Fields[fieldName] = multiRefLocalized
							parentNeedsUpdate[parent.Sys.ID] = parent
						} else {
							newReferenceSysMap = append(newReferenceSysMap, referenceSys)
						}
					}
					multiRefLocalized[locale] = newReferenceSysMap
				}
			}
		}
	}
	err = common.SmartUpdateEntry(newEntry, oldEntry, cma, spaceID)
	if err != nil {
		log.Fatalf("New entry error in smart update: %v", err)
	}
	for _, parent := range parentNeedsUpdate {
		err := common.SmartUpdateEntry(parent, nil, cma, spaceID)
		if err != nil {
			log.Printf("Parent entry %s could not be updated: %v", parent.Sys.ID, err)
		}
	}
	log.Printf("New entry: https://app.contentful.com/spaces/%s/environments/%s/entries/%s", spaceID, cma.Environment, newEntry.Sys.ID)
	log.Printf("Old entry: https://app.contentful.com/spaces/%s/environments/%s/entries/%s", spaceID, cma.Environment, oldEntry.Sys.ID)
	return nil
}
