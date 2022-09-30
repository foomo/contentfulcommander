package modeldiff

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/foomo/contentful"
	"github.com/foomo/contentfulcommander/contentfulclient"
	"github.com/foomo/contentfulcommander/model"
	"sort"
	"strings"
)

func Run(cma *contentful.Contentful, params []string) error {

	firstSpace, firstEnvironment := contentfulclient.GetSpaceAndEnvironment(params[0])
	if firstSpace == "" {
		return errors.New("firstspace ID is empty")
	}
	if firstEnvironment == "" {
		return errors.New("firstEnvironment ID is empty")
	}
	secondSpace, secondEnvironment := contentfulclient.GetSpaceAndEnvironment(params[1])
	if secondSpace == "" {
		return errors.New("secondspace ID is empty")
	}
	if secondEnvironment == "" {
		return errors.New("secondEnvironment ID is empty")
	}
	fmt.Printf("A: %s/%s B: %s/%s\n", firstSpace, firstEnvironment, secondSpace, secondEnvironment)

	firstSpaceContentTypes, err := getContentTypes(cma, firstSpace, firstEnvironment)
	if err != nil {
		return err
	}
	secondSpaceContentTypes, err := getContentTypes(cma, secondSpace, secondEnvironment)
	if err != nil {
		return err
	}
	diffContentTypes(fmt.Sprintf("%s/%s", firstSpace, firstEnvironment),
		fmt.Sprintf("%s/%s", secondSpace, secondEnvironment),
		firstSpaceContentTypes,
		secondSpaceContentTypes)
	return nil
}

func getContentTypes(cma *contentful.Contentful, spaceID, environment string) (contentTypes []model.ContentType, err error) {
	cma.Environment = environment
	col := cma.ContentTypes.List(spaceID)
	_, errGetAll := col.GetAll()
	if errGetAll != nil {
		err = fmt.Errorf("could not get content types for %s/%s: %v", spaceID, environment, errGetAll)
	}
	for _, item := range col.Items {
		var contentType model.ContentType
		byteArray, _ := json.Marshal(item)
		err = json.NewDecoder(bytes.NewReader(byteArray)).Decode(&contentType)
		if err != nil {
			break
		}
		var filteredFields []model.ContentTypeField
		for _, field := range contentType.Fields {
			if !field.Omitted {
				filteredFields = append(filteredFields, field)
			}
		}
		contentType.Fields = filteredFields
		contentTypes = append(contentTypes, contentType)
	}
	sort.Slice(
		contentTypes, func(i, j int) bool {
			return contentTypes[i].Name < contentTypes[j].Name
		},
	)
	return
}

func diffContentTypes(firstSpaceName, secondSpaceName string, firstSpaceContentTypes, secondSpaceContentTypes []model.ContentType) {

	firstContentTypeMap,
		secondContentTypeMap,
		firstOnlyTypes,
		secondOnlyTypes,
		_,
		sortedTypes :=
		sliceElementsCompare(firstSpaceContentTypes, secondSpaceContentTypes,
			func(contentType model.ContentType) string {
				return contentType.Sys.ID
			})

	const contentTypeHeader = "Content Type: '%s' %s\n"
	for _, contentTypeID := range sortedTypes {
		if _, ok := firstOnlyTypes[contentTypeID]; ok {
			_ = printContentTypeHeader(contentTypeHeader, contentTypeID, false)
			fmt.Printf("AAA ___ content type only available in %s\n", firstSpaceName)
			continue
		}
		if _, ok := secondOnlyTypes[contentTypeID]; ok {
			_ = printContentTypeHeader(contentTypeHeader, contentTypeID, false)
			fmt.Printf("___ BBB content type only available in %s\n", secondSpaceName)
			continue
		}
		firstContentType := firstContentTypeMap[contentTypeID]
		secondContentType := secondContentTypeMap[contentTypeID]
		contentTypeHeaderAlreadyPrinted := false
		if firstContentType.Name != secondContentType.Name {
			contentTypeHeaderAlreadyPrinted = printContentTypeHeader(contentTypeHeader, contentTypeID, contentTypeHeaderAlreadyPrinted)
			fmt.Printf("AAA BBB Name is different\n")
			fmt.Printf(" ^   ^----B: %s\n", firstContentType.Name)
			fmt.Printf(" ^--------A: %s\n", secondContentType.Name)
		}
		firstFields := firstContentType.Fields
		sort.Slice(firstFields, func(i, j int) bool {
			return firstFields[i].ID < firstFields[j].ID
		})
		secondFields := secondContentType.Fields
		sort.Slice(secondFields, func(i, j int) bool {
			return secondFields[i].ID < secondFields[j].ID
		})
		firstContentTypeFieldMap,
			secondContentTypeFieldMap,
			firstOnlyFields,
			secondOnlyFields,
			_,
			sortedFields :=
			sliceElementsCompare(firstFields, secondFields,
				func(field model.ContentTypeField) string {
					return field.ID
				})
		for _, fieldID := range sortedFields {
			if _, ok := firstOnlyFields[fieldID]; ok {
				contentTypeHeaderAlreadyPrinted = printContentTypeHeader(contentTypeHeader, contentTypeID, contentTypeHeaderAlreadyPrinted)
				fmt.Printf("    AAA ___ field '%s' only available in %s\n", fieldID, firstSpaceName)
				continue
			}
			if _, ok := secondOnlyFields[fieldID]; ok {
				contentTypeHeaderAlreadyPrinted = printContentTypeHeader(contentTypeHeader, contentTypeID, contentTypeHeaderAlreadyPrinted)
				fmt.Printf("    ___ BBB field '%s' only available in %s\n", fieldID, firstSpaceName)
				continue
			}
			firstField := firstContentTypeFieldMap[fieldID]
			secondField := secondContentTypeFieldMap[fieldID]
			fieldHeaderAlreadyPrinted := false
			printHeaders := func() {
				contentTypeHeaderAlreadyPrinted = printContentTypeHeader(contentTypeHeader, contentTypeID, contentTypeHeaderAlreadyPrinted)
				fieldHeaderAlreadyPrinted = printFieldHeader(fieldID, fieldHeaderAlreadyPrinted)
			}
			if firstField.Name != secondField.Name {
				printHeaders()
				printFieldValuesAB("Name", secondField.Name, firstField.Name)
			}
			if firstField.Type != secondField.Type {
				printHeaders()
				printFieldValuesAB("Type", secondField.Type, firstField.Type)
			}
			if firstField.LinkType != secondField.LinkType {
				printHeaders()
				printFieldValuesAB("LinkType", secondField.Type, firstField.Type)
			}
			if firstField.Localized != secondField.Localized {
				printHeaders()
				printFieldValuesAB("Localized", secondField.Localized, firstField.Localized)
			}
			if firstField.Disabled != secondField.Disabled {
				printHeaders()
				printFieldValuesAB("Disabled", secondField.Localized, firstField.Disabled)
			}
			if firstField.Omitted != secondField.Omitted {
				printHeaders()
				printFieldValuesAB("Omitted", secondField.Omitted, firstField.Omitted)
			}
			if firstField.Required != secondField.Required {
				printHeaders()
				printFieldValuesAB("Required", secondField.Required, firstField.Required)
			}
			firstFieldValidations := getJsonString(firstField.Validations)
			secondFieldValidations := getJsonString(secondField.Validations)
			if firstFieldValidations != secondFieldValidations {
				printHeaders()
				printFieldValuesAB("Validations", firstFieldValidations, secondFieldValidations)
			}
			firstFieldItems := getJsonString(firstField.Items)
			secondFieldItems := getJsonString(secondField.Items)
			if firstFieldItems != secondFieldItems {
				printHeaders()
				printFieldValuesAB("Items", firstFieldItems, secondFieldItems)
			}
		}
	}
}

func getJsonString(value any) (stringValue string) {
	byt, _ := json.Marshal(value)
	stringValue = string(byt)
	return
}

func sliceElementsCompare[A any](firstSlice, secondSlice []A, getID func(element A) string) (
	map[string]A,
	map[string]A,
	map[string]bool,
	map[string]bool,
	map[string]bool,
	[]string,
) {
	firstObjectMap := map[string]A{}
	secondObjectMap := map[string]A{}
	firstOnly := map[string]bool{}
	secondOnly := map[string]bool{}
	common := map[string]bool{}
	all := map[string]bool{}
	for _, element := range firstSlice {
		firstObjectMap[getID(element)] = element
		all[getID(element)] = true
	}
	for _, element := range secondSlice {
		secondObjectMap[getID(element)] = element
		all[getID(element)] = true
	}
	for id := range firstObjectMap {
		if _, hasIt := secondObjectMap[id]; hasIt {
			common[id] = true
		} else {
			firstOnly[id] = true
		}
	}
	for id := range secondObjectMap {
		if _, hasIt := firstObjectMap[id]; hasIt {
			common[id] = true
		} else {
			secondOnly[id] = true
		}
	}
	sortedIDs := make([]string, 0, len(all))
	for k := range all {
		sortedIDs = append(sortedIDs, k)
	}
	sort.Strings(sortedIDs)
	return firstObjectMap, secondObjectMap, firstOnly, secondOnly, common, sortedIDs
}

func printContentTypeHeader(contentTypeHeader, contentTypeID string, contentTypeHeaderAlreadyPrinted bool) bool {
	if !contentTypeHeaderAlreadyPrinted {
		fmt.Printf(contentTypeHeader, contentTypeID, strings.Repeat("-", 80-len(contentTypeID)))
	}
	return true
}

func printFieldHeader(fieldID string, fieldHeaderAlreadyPrinted bool) bool {
	if !fieldHeaderAlreadyPrinted {
		fmt.Printf("    AAA BBB field '%s' is different\n", fieldID)
	}
	return true
}

func printFieldValuesAB(fieldAttribute string, firstValue, secondValue any) {
	fmt.Printf("     ^   ^----B: %s = %v\n", fieldAttribute, firstValue)
	fmt.Printf("     ^--------A: %s = %v\n", fieldAttribute, secondValue)
}
