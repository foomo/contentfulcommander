package commanderclient

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/foomo/contentful"
)

// Helper to create a test entry with fields
func createTestEntry(id string, fields map[string]any) *EntryEntity {
	return &EntryEntity{
		Entry: &contentful.Entry{
			Sys: &contentful.Sys{
				ID: id,
				ContentType: &contentful.ContentType{
					Sys: &contentful.Sys{ID: "test-type"},
				},
			},
			Fields: fields,
		},
	}
}

// Mock translation function that uppercases text
func mockTranslate(text string) (string, int, error) {
	return strings.ToUpper(text), len(text), nil
}

// Mock batch translation function
func mockBatchTranslate(texts []string) ([]string, int, error) {
	results := make([]string, len(texts))
	totalBilled := 0
	for i, text := range texts {
		results[i] = strings.ToUpper(text)
		totalBilled += len(text)
	}
	return results, totalBilled, nil
}

func TestTranslateField_SimpleString(t *testing.T) {
	entry := createTestEntry("test-1", map[string]any{
		"title": map[string]any{
			"de": "Hallo Welt",
		},
	})

	billed, err := TranslateField(entry, "title", Locale("de"), Locale("en"), mockTranslate)
	if err != nil {
		t.Fatalf("TranslateField failed: %v", err)
	}
	if billed != 10 {
		t.Errorf("Expected 10 billed characters, got %d", billed)
	}

	result := entry.GetFieldValueAsString("title", Locale("en"))
	if result != "HALLO WELT" {
		t.Errorf("Expected 'HALLO WELT', got '%s'", result)
	}
}

func TestTranslateField_EmptyString(t *testing.T) {
	entry := createTestEntry("test-2", map[string]any{
		"title": map[string]any{
			"de": "",
		},
	})

	billed, err := TranslateField(entry, "title", Locale("de"), Locale("en"), mockTranslate)
	if err != nil {
		t.Fatalf("TranslateField failed: %v", err)
	}
	if billed != 0 {
		t.Errorf("Expected 0 billed characters for empty string, got %d", billed)
	}

	result := entry.GetFieldValueAsString("title", Locale("en"))
	if result != "" {
		t.Errorf("Expected empty string, got '%s'", result)
	}
}

func TestTranslateField_NilField(t *testing.T) {
	entry := createTestEntry("test-3", map[string]any{})

	billed, err := TranslateField(entry, "title", Locale("de"), Locale("en"), mockTranslate)
	if err != nil {
		t.Fatalf("TranslateField should not fail for nil field: %v", err)
	}
	if billed != 0 {
		t.Errorf("Expected 0 billed characters for nil field, got %d", billed)
	}

	result := entry.GetFieldValue("title", Locale("en"))
	if result != nil {
		t.Errorf("Expected nil, got '%v'", result)
	}
}

func TestTranslateField_RichText(t *testing.T) {
	// Create a RichText document with multiple text nodes
	richTextValue := map[string]any{
		"nodeType": "document",
		"content": []any{
			map[string]any{
				"nodeType": "paragraph",
				"content": []any{
					map[string]any{
						"nodeType": "text",
						"value":    "Hello",
					},
					map[string]any{
						"nodeType": "text",
						"value":    " World",
					},
				},
			},
			map[string]any{
				"nodeType": "paragraph",
				"content": []any{
					map[string]any{
						"nodeType": "text",
						"value":    "Goodbye",
					},
				},
			},
		},
	}

	entry := createTestEntry("test-4", map[string]any{
		"description": map[string]any{
			"de": richTextValue,
		},
	})

	billed, err := TranslateField(entry, "description", Locale("de"), Locale("en"), mockTranslate)
	if err != nil {
		t.Fatalf("TranslateField failed: %v", err)
	}
	// "Hello" (5) + " World" (6) + "Goodbye" (7) = 18
	if billed != 18 {
		t.Errorf("Expected 18 billed characters, got %d", billed)
	}

	// Verify the translated content
	result := entry.GetFieldValue("description", Locale("en"))
	if result == nil {
		t.Fatal("Expected translated RichText, got nil")
	}

	// Parse the result to verify text nodes were translated
	rt, err := parseRichText(result)
	if err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	texts := rt.extractText()
	expectedTexts := map[string]string{
		"000-000-000": "HELLO",
		"000-000-001": " WORLD",
		"000-001-000": "GOODBYE",
	}

	for path, expected := range expectedTexts {
		if texts[path] != expected {
			t.Errorf("Path %s: expected '%s', got '%s'", path, expected, texts[path])
		}
	}
}

func TestTranslateField_RichTextEmpty(t *testing.T) {
	// Create a RichText document with no text nodes
	richTextValue := map[string]any{
		"nodeType": "document",
		"content":  []any{},
	}

	entry := createTestEntry("test-5", map[string]any{
		"description": map[string]any{
			"de": richTextValue,
		},
	})

	billed, err := TranslateField(entry, "description", Locale("de"), Locale("en"), mockTranslate)
	if err != nil {
		t.Fatalf("TranslateField failed: %v", err)
	}
	if billed != 0 {
		t.Errorf("Expected 0 billed characters for empty RichText, got %d", billed)
	}

	// Verify the structure was copied
	result := entry.GetFieldValue("description", Locale("en"))
	if result == nil {
		t.Fatal("Expected RichText structure, got nil")
	}
}

func TestTranslateField_TranslationError(t *testing.T) {
	entry := createTestEntry("test-6", map[string]any{
		"title": map[string]any{
			"de": "Test",
		},
	})

	errorTranslate := func(text string) (string, int, error) {
		return "", 0, errors.New("translation failed")
	}

	_, err := TranslateField(entry, "title", Locale("de"), Locale("en"), errorTranslate)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "translation failed") {
		t.Errorf("Expected error to contain 'translation failed', got: %v", err)
	}
}

func TestTranslateFieldBatch_SimpleString(t *testing.T) {
	entry := createTestEntry("test-7", map[string]any{
		"title": map[string]any{
			"de": "Hallo Welt",
		},
	})

	billed, err := TranslateFieldBatch(entry, "title", Locale("de"), Locale("en"), mockBatchTranslate)
	if err != nil {
		t.Fatalf("TranslateFieldBatch failed: %v", err)
	}
	if billed != 10 {
		t.Errorf("Expected 10 billed characters, got %d", billed)
	}

	result := entry.GetFieldValueAsString("title", Locale("en"))
	if result != "HALLO WELT" {
		t.Errorf("Expected 'HALLO WELT', got '%s'", result)
	}
}

func TestTranslateFieldBatch_RichText(t *testing.T) {
	// Track the order of texts received
	var receivedTexts []string
	trackingTranslate := func(texts []string) ([]string, int, error) {
		receivedTexts = texts
		return mockBatchTranslate(texts)
	}

	richTextValue := map[string]any{
		"nodeType": "document",
		"content": []any{
			map[string]any{
				"nodeType": "paragraph",
				"content": []any{
					map[string]any{
						"nodeType": "text",
						"value":    "First",
					},
					map[string]any{
						"nodeType": "text",
						"value":    "Second",
					},
				},
			},
		},
	}

	entry := createTestEntry("test-8", map[string]any{
		"description": map[string]any{
			"de": richTextValue,
		},
	})

	_, err := TranslateFieldBatch(entry, "description", Locale("de"), Locale("en"), trackingTranslate)
	if err != nil {
		t.Fatalf("TranslateFieldBatch failed: %v", err)
	}

	// Verify batch was called with all texts at once
	if len(receivedTexts) != 2 {
		t.Errorf("Expected 2 texts in batch, got %d", len(receivedTexts))
	}
}

func TestTranslateFieldIfEmpty_SkipsExisting(t *testing.T) {
	entry := createTestEntry("test-9", map[string]any{
		"title": map[string]any{
			"de": "German",
			"en": "English",
		},
	})

	translateCalled := false
	trackingTranslate := func(text string) (string, int, error) {
		translateCalled = true
		return strings.ToUpper(text), len(text), nil
	}

	billed, err := TranslateFieldIfEmpty(entry, "title", Locale("de"), Locale("en"), trackingTranslate)
	if err != nil {
		t.Fatalf("TranslateFieldIfEmpty failed: %v", err)
	}
	if billed != 0 {
		t.Errorf("Expected 0 billed characters when skipped, got %d", billed)
	}

	if translateCalled {
		t.Error("Translate should not have been called when target exists")
	}

	// Verify original value unchanged
	result := entry.GetFieldValueAsString("title", Locale("en"))
	if result != "English" {
		t.Errorf("Expected 'English', got '%s'", result)
	}
}

func TestTranslateFieldIfEmpty_TranslatesWhenEmpty(t *testing.T) {
	entry := createTestEntry("test-10", map[string]any{
		"title": map[string]any{
			"de": "German",
			"en": "",
		},
	})

	billed, err := TranslateFieldIfEmpty(entry, "title", Locale("de"), Locale("en"), mockTranslate)
	if err != nil {
		t.Fatalf("TranslateFieldIfEmpty failed: %v", err)
	}
	if billed != 6 {
		t.Errorf("Expected 6 billed characters, got %d", billed)
	}

	result := entry.GetFieldValueAsString("title", Locale("en"))
	if result != "GERMAN" {
		t.Errorf("Expected 'GERMAN', got '%s'", result)
	}
}

func TestProcessHyperlinks(t *testing.T) {
	richTextValue := map[string]any{
		"nodeType": "document",
		"content": []any{
			map[string]any{
				"nodeType": "paragraph",
				"content": []any{
					map[string]any{
						"nodeType": "hyperlink",
						"data": map[string]any{
							"uri": "/de/products/item",
						},
						"content": []any{
							map[string]any{
								"nodeType": "text",
								"value":    "Link text",
							},
						},
					},
				},
			},
		},
	}

	entry := createTestEntry("test-11", map[string]any{
		"content": map[string]any{
			"de": richTextValue,
		},
	})

	resolver := func(uri string) (string, error) {
		return strings.Replace(uri, "/de/", "/en/", 1), nil
	}

	err := ProcessHyperlinks(entry, "content", Locale("de"), resolver)
	if err != nil {
		t.Fatalf("ProcessHyperlinks failed: %v", err)
	}

	// Verify the hyperlink was updated
	result := entry.GetFieldValue("content", Locale("de"))
	rt, _ := parseRichText(result)

	var foundUri string
	rt.walkHyperlinks(func(node *RichTextNode) error {
		foundUri = node.getHyperlinkURI()
		return nil
	})

	if foundUri != "/en/products/item" {
		t.Errorf("Expected '/en/products/item', got '%s'", foundUri)
	}
}

func TestProcessHyperlinks_NoChange(t *testing.T) {
	richTextValue := map[string]any{
		"nodeType": "document",
		"content": []any{
			map[string]any{
				"nodeType": "paragraph",
				"content": []any{
					map[string]any{
						"nodeType": "hyperlink",
						"data": map[string]any{
							"uri": "/en/products/item",
						},
						"content": []any{
							map[string]any{
								"nodeType": "text",
								"value":    "Link text",
							},
						},
					},
				},
			},
		},
	}

	entry := createTestEntry("test-12", map[string]any{
		"content": map[string]any{
			"de": richTextValue,
		},
	})

	// Resolver that doesn't change anything
	resolver := func(uri string) (string, error) {
		return uri, nil
	}

	err := ProcessHyperlinks(entry, "content", Locale("de"), resolver)
	if err != nil {
		t.Fatalf("ProcessHyperlinks failed: %v", err)
	}
}

func TestProcessHyperlinks_MultipleLinks(t *testing.T) {
	richTextValue := map[string]any{
		"nodeType": "document",
		"content": []any{
			map[string]any{
				"nodeType": "paragraph",
				"content": []any{
					map[string]any{
						"nodeType": "hyperlink",
						"data": map[string]any{
							"uri": "/de/link1",
						},
						"content": []any{
							map[string]any{
								"nodeType": "text",
								"value":    "Link 1",
							},
						},
					},
					map[string]any{
						"nodeType": "hyperlink",
						"data": map[string]any{
							"uri": "/de/link2",
						},
						"content": []any{
							map[string]any{
								"nodeType": "text",
								"value":    "Link 2",
							},
						},
					},
				},
			},
		},
	}

	entry := createTestEntry("test-13", map[string]any{
		"content": map[string]any{
			"de": richTextValue,
		},
	})

	resolveCount := 0
	resolver := func(uri string) (string, error) {
		resolveCount++
		return strings.Replace(uri, "/de/", "/en/", 1), nil
	}

	err := ProcessHyperlinks(entry, "content", Locale("de"), resolver)
	if err != nil {
		t.Fatalf("ProcessHyperlinks failed: %v", err)
	}

	if resolveCount != 2 {
		t.Errorf("Expected resolver to be called 2 times, got %d", resolveCount)
	}
}

func TestProcessHyperlinks_NonRichTextField(t *testing.T) {
	entry := createTestEntry("test-14", map[string]any{
		"title": map[string]any{
			"de": "Just a string",
		},
	})

	resolver := func(uri string) (string, error) {
		return uri, nil
	}

	err := ProcessHyperlinks(entry, "title", Locale("de"), resolver)
	if err == nil {
		t.Fatal("Expected error for non-RichText field")
	}
}

// Test the DeepLTranslator helper using httptest to mock the API
func TestDeepLTranslator(t *testing.T) {
	// Create a mock server that returns predictable translations
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "DeepL-Auth-Key test-key" {
			t.Errorf("Unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		// Parse the request body
		var req DeepLTranslateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
		}

		// Verify languages
		if req.SourceLang != DeepLSourceDE {
			t.Errorf("Expected source DE, got %s", req.SourceLang)
		}
		if req.TargetLang != DeepLTargetENGB {
			t.Errorf("Expected target EN-GB, got %s", req.TargetLang)
		}

		// Verify ShowBilledChars is set
		if req.ShowBilledChars == nil || !*req.ShowBilledChars {
			t.Error("Expected ShowBilledChars to be true")
		}

		// Build response - prefix each text with [EN] and return billed characters
		translations := make([]DeepLTranslation, len(req.Text))
		for i, text := range req.Text {
			translations[i] = DeepLTranslation{
				Text:                   "[EN] " + text,
				DetectedSourceLanguage: "DE",
				BilledCharacters:       len(text),
			}
		}

		resp := DeepLTranslateResponse{Translations: translations}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create client with mock server URL
	client := NewDeepLClient("test-key", WithDeepLBaseURL(server.URL))
	translator := NewDeepLTranslator(
		client,
		SourceLocale{Locale: "de", DeepLLang: DeepLSourceDE},
		TargetLocale{Locale: "en", DeepLLang: DeepLTargetENGB},
	)

	// Test single translation
	result, billed, err := translator.Translate("Test")
	if err != nil {
		t.Fatalf("Translate failed: %v", err)
	}
	if result != "[EN] Test" {
		t.Errorf("Expected '[EN] Test', got '%s'", result)
	}
	if billed != 4 {
		t.Errorf("Expected 4 billed characters, got %d", billed)
	}

	// Test batch translation
	results, totalBilled, err := translator.TranslateBatch([]string{"One", "Two"})
	if err != nil {
		t.Fatalf("TranslateBatch failed: %v", err)
	}
	if len(results) != 2 || results[0] != "[EN] One" || results[1] != "[EN] Two" {
		t.Errorf("Unexpected batch results: %v", results)
	}
	if totalBilled != 6 { // "One" (3) + "Two" (3)
		t.Errorf("Expected 6 total billed characters, got %d", totalBilled)
	}
}

func TestRichTextInternal_ExtractAndReplace(t *testing.T) {
	// Test the internal richtext functions
	rt := &RichTextNode{
		NodeType: nodeTypeDocument,
		Content: []*RichTextNode{
			{
				NodeType: nodeTypeParagraph,
				Content: []*RichTextNode{
					{NodeType: nodeTypeText, Value: "First"},
					{NodeType: nodeTypeText, Value: "Second"},
				},
			},
			{
				NodeType: nodeTypeParagraph,
				Content: []*RichTextNode{
					{NodeType: nodeTypeText, Value: "Third"},
				},
			},
		},
	}

	texts := rt.extractText()
	if len(texts) != 3 {
		t.Errorf("Expected 3 texts, got %d", len(texts))
	}

	// Replace with modified texts
	replacements := map[string]string{
		"000-000-000": "FIRST",
		"000-000-001": "SECOND",
		"000-001-000": "THIRD",
	}
	rt.replaceText(replacements)

	// Verify replacement
	newTexts := rt.extractText()
	if newTexts["000-000-000"] != "FIRST" {
		t.Errorf("Expected 'FIRST', got '%s'", newTexts["000-000-000"])
	}
}
