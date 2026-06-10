package commanderclient

import (
	"context"
	"strings"
	"testing"

	"github.com/foomo/contentful"
)

func TestGetContentTypeAndFieldNilSafe(t *testing.T) {
	var nilClient *MigrationClient
	if contentType, ok := nilClient.GetContentType("article"); ok || contentType != nil {
		t.Fatal("expected nil client content type lookup to fail")
	}

	client := &MigrationClient{}
	if contentType, ok := client.GetContentType("article"); ok || contentType != nil {
		t.Fatal("expected unloaded space model content type lookup to fail")
	}

	client.spaceModel = &SpaceModel{
		ContentTypes: map[string]*contentful.ContentType{
			"article": {
				Fields: []*contentful.Field{
					nil,
					{ID: "title", Name: "Title"},
				},
			},
			"nilType": nil,
		},
	}

	if contentType, ok := client.GetContentType("article"); !ok || contentType == nil {
		t.Fatal("expected content type lookup to succeed")
	}
	if contentType, ok := client.GetContentType("nilType"); ok || contentType != nil {
		t.Fatal("expected nil content type lookup to fail")
	}
	if field, ok := client.GetContentTypeField("article", "title"); !ok || field == nil {
		t.Fatal("expected field lookup to succeed")
	}
	if field, ok := client.GetContentTypeField("article", "missing"); ok || field != nil {
		t.Fatal("expected missing field lookup to fail")
	}
}

func TestFieldValidationHelpers(t *testing.T) {
	field := &contentful.Field{
		Validations: []contentful.FieldValidation{
			&contentful.FieldValidationSize{Size: &contentful.MinMax{Min: 2, Max: 50}},
			contentful.FieldValidationSize{Size: &contentful.MinMax{Min: 5, Max: 20}},
			&contentful.FieldValidationPredefinedValues{In: []interface{}{"General", "iOS"}},
			contentful.FieldValidationRegex{Regex: &contentful.Regex{Pattern: "^such", Flags: "im"}},
		},
	}

	if !FieldIsEditable(field) {
		t.Fatal("expected field to be editable")
	}
	if FieldIsEditable(&contentful.Field{Disabled: true}) {
		t.Fatal("expected disabled field not to be editable")
	}
	if FieldIsEditable(&contentful.Field{Omitted: true}) {
		t.Fatal("expected omitted field not to be editable")
	}

	if maxLength, ok := FieldMaxLength(field); !ok || maxLength != 20 {
		t.Fatalf("expected strictest max length 20, got %d (%t)", maxLength, ok)
	}
	if minLength, ok := FieldMinLength(field); !ok || minLength != 5 {
		t.Fatalf("expected strictest min length 5, got %d (%t)", minLength, ok)
	}
	if values, ok := FieldAllowedValues(field); !ok || len(values) != 2 || values[0] != "General" {
		t.Fatalf("expected allowed values, got %#v (%t)", values, ok)
	}
	if pattern, flags, ok := FieldRegex(field); !ok || pattern != "^such" || flags != "im" {
		t.Fatalf("expected regex ^such/im, got %q/%q (%t)", pattern, flags, ok)
	}

	summary := GetFieldValidationSummary(field)
	if summary.MinLength == nil || *summary.MinLength != 5 {
		t.Fatalf("expected summary min length 5, got %#v", summary.MinLength)
	}
	if summary.MaxLength == nil || *summary.MaxLength != 20 {
		t.Fatalf("expected summary max length 20, got %#v", summary.MaxLength)
	}
	if len(summary.AllowedValues) != 2 {
		t.Fatalf("expected summary allowed values, got %#v", summary.AllowedValues)
	}
	if summary.RegexPattern != "^such" || summary.RegexFlags != "im" {
		t.Fatalf("expected summary regex ^such/im, got %q/%q", summary.RegexPattern, summary.RegexFlags)
	}
}

func TestGetEntryDisplayNameUsesRawLocaleValue(t *testing.T) {
	client := &MigrationClient{
		spaceModel: &SpaceModel{
			ContentTypes: map[string]*contentful.ContentType{
				"article": {DisplayField: "title"},
			},
		},
	}
	entity := &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID: "entry-id",
				ContentType: &contentful.ContentType{
					Sys: &contentful.Sys{ID: "article"},
				},
			},
			Fields: map[string]any{
				"title": map[string]any{
					"en-US": "English title",
				},
			},
		},
	}

	if displayName := client.GetEntryDisplayName(entity, Locale("en-US")); displayName != "English title" {
		t.Fatalf("expected locale display name, got %q", displayName)
	}
	if displayName := client.GetEntryDisplayName(entity, Locale("de-DE")); displayName != "entry-id" {
		t.Fatalf("expected no fallback and entity ID, got %q", displayName)
	}
}

func TestFilterByFieldEmptyWithLocale(t *testing.T) {
	entity := &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{ID: "entry-id", ContentType: &contentful.ContentType{Sys: &contentful.Sys{ID: "article"}}},
			Fields: map[string]any{
				"title": map[string]any{
					"en-US": "Title",
					"de-DE": "",
				},
			},
		},
	}

	if !FilterByFieldEmptyWithLocale("title", Locale("de-DE"))(entity) {
		t.Fatal("expected empty locale filter to match")
	}
	if FilterByFieldEmptyWithLocale("title", Locale("en-US"))(entity) {
		t.Fatal("expected empty locale filter not to match")
	}
	if !FilterByFieldNotEmptyWithLocale("title", Locale("en-US"))(entity) {
		t.Fatal("expected not-empty locale filter to match")
	}
}

func TestSaveDraftPublishPreconditions(t *testing.T) {
	ctx := context.Background()

	var nilClient *MigrationClient
	if err := nilClient.SaveDraft(ctx, nil); err == nil || !strings.Contains(err.Error(), "migration client is nil") {
		t.Fatalf("expected nil client error, got %v", err)
	}

	clientWithoutCMA := &MigrationClient{}
	if err := clientWithoutCMA.Publish(ctx, &EntryEntity{}); err == nil || !strings.Contains(err.Error(), "no CMA client") {
		t.Fatalf("expected missing CMA error, got %v", err)
	}

	client := newMigrationClient("token", "", "space", "master")
	var nilEntry *EntryEntity
	if err := client.SaveDraft(ctx, nilEntry); err == nil || !strings.Contains(err.Error(), "entity is nil") {
		t.Fatalf("expected nil entity error, got %v", err)
	}
	if err := client.Publish(ctx, unsupportedEntity{}); err == nil || !strings.Contains(err.Error(), "unsupported entity type") {
		t.Fatalf("expected unsupported entity error, got %v", err)
	}
}

type unsupportedEntity struct {
	Entity
}
