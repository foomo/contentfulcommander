package commanderclient

import (
	"fmt"
	"sort"
)

// TranslateFunc is called for each text chunk that needs translation.
// It receives the source text and should return the translated text.
type TranslateFunc func(text string) (translated string, err error)

// TranslateBatchFunc is called with all text chunks at once for batch translation.
// This is more efficient when using APIs like DeepL that support batch requests.
// The returned slice must have the same length and order as the input slice.
type TranslateBatchFunc func(texts []string) (translated []string, err error)

// TranslateField translates a field value from source to target locale.
// It automatically handles different field types:
//   - String fields (Symbol, Text): translated directly
//   - RichText fields: all text nodes are extracted, translated individually, and reassembled
//
// The translate function is called once for each text chunk.
// For RichText fields with many text nodes, consider using TranslateFieldBatch for efficiency.
func TranslateField(
	entity Entity,
	fieldName string,
	sourceLocale Locale,
	targetLocale Locale,
	translate TranslateFunc,
) error {
	value := entity.GetFieldValue(fieldName, sourceLocale)
	if value == nil {
		return nil
	}

	// Try as RichText first
	if rt, err := parseRichText(value); err == nil && rt.isDocument() {
		// Extract all text nodes
		texts := rt.extractText()

		if len(texts) == 0 {
			entity.SetFieldValue(fieldName, targetLocale, rt)
			return nil
		}

		// Translate each text node
		translated := make(map[string]string)
		for path, text := range texts {
			result, err := translate(text)
			if err != nil {
				return fmt.Errorf("translation failed for path %s: %w", path, err)
			}
			translated[path] = result
		}

		// Replace in tree
		rt.replaceText(translated)

		entity.SetFieldValue(fieldName, targetLocale, rt)
		return nil
	}

	// Fall back to simple string
	if str, ok := value.(string); ok {
		if str == "" {
			entity.SetFieldValue(fieldName, targetLocale, "")
			return nil
		}
		result, err := translate(str)
		if err != nil {
			return fmt.Errorf("translation failed: %w", err)
		}
		entity.SetFieldValue(fieldName, targetLocale, result)
		return nil
	}

	return fmt.Errorf("unsupported field type for translation: field '%s' is neither string nor RichText", fieldName)
}

// TranslateFieldBatch translates a field value using batch translation.
// This is more efficient for RichText fields when using APIs that support batch requests,
// as all text nodes are translated in a single API call.
//
// For simple string fields, this behaves the same as TranslateField but wraps
// the single text in a batch call.
func TranslateFieldBatch(
	entity Entity,
	fieldName string,
	sourceLocale Locale,
	targetLocale Locale,
	translateBatch TranslateBatchFunc,
) error {
	value := entity.GetFieldValue(fieldName, sourceLocale)
	if value == nil {
		return nil
	}

	// Try as RichText first
	if rt, err := parseRichText(value); err == nil && rt.isDocument() {
		// Extract all text nodes
		textsByPath := rt.extractText()

		if len(textsByPath) == 0 {
			entity.SetFieldValue(fieldName, targetLocale, rt)
			return nil
		}

		// Build ordered lists for batch translation
		paths := make([]string, 0, len(textsByPath))
		for path := range textsByPath {
			paths = append(paths, path)
		}
		sort.Strings(paths) // Ensure consistent ordering

		texts := make([]string, len(paths))
		for i, path := range paths {
			texts[i] = textsByPath[path]
		}

		// Batch translate all text nodes
		translatedTexts, err := translateBatch(texts)
		if err != nil {
			return fmt.Errorf("batch translation failed: %w", err)
		}

		if len(translatedTexts) != len(texts) {
			return fmt.Errorf("batch translation returned %d results, expected %d", len(translatedTexts), len(texts))
		}

		// Map translations back to paths
		translated := make(map[string]string)
		for i, path := range paths {
			translated[path] = translatedTexts[i]
		}

		// Replace in tree
		rt.replaceText(translated)

		entity.SetFieldValue(fieldName, targetLocale, rt)
		return nil
	}

	// Fall back to simple string
	if str, ok := value.(string); ok {
		if str == "" {
			entity.SetFieldValue(fieldName, targetLocale, "")
			return nil
		}
		// Wrap single string in batch call
		results, err := translateBatch([]string{str})
		if err != nil {
			return fmt.Errorf("translation failed: %w", err)
		}
		if len(results) != 1 {
			return fmt.Errorf("batch translation returned %d results, expected 1", len(results))
		}
		entity.SetFieldValue(fieldName, targetLocale, results[0])
		return nil
	}

	return fmt.Errorf("unsupported field type for translation: field '%s' is neither string nor RichText", fieldName)
}

// TranslateFieldIfEmpty translates only if the target locale field is empty or nil.
// This is useful for incremental translation where you don't want to re-translate
// already translated content.
func TranslateFieldIfEmpty(
	entity Entity,
	fieldName string,
	sourceLocale Locale,
	targetLocale Locale,
	translate TranslateFunc,
) error {
	// Check if target already has a value
	targetValue := entity.GetFieldValue(fieldName, targetLocale)
	if targetValue != nil {
		// Check if it's an empty string
		if str, ok := targetValue.(string); ok && str == "" {
			// Empty string, proceed with translation
		} else {
			// Has value, skip translation
			return nil
		}
	}

	return TranslateField(entity, fieldName, sourceLocale, targetLocale, translate)
}

// TranslateFieldBatchIfEmpty is like TranslateFieldIfEmpty but uses batch translation.
func TranslateFieldBatchIfEmpty(
	entity Entity,
	fieldName string,
	sourceLocale Locale,
	targetLocale Locale,
	translateBatch TranslateBatchFunc,
) error {
	// Check if target already has a value
	targetValue := entity.GetFieldValue(fieldName, targetLocale)
	if targetValue != nil {
		// Check if it's an empty string
		if str, ok := targetValue.(string); ok && str == "" {
			// Empty string, proceed with translation
		} else {
			// Has value, skip translation
			return nil
		}
	}

	return TranslateFieldBatch(entity, fieldName, sourceLocale, targetLocale, translateBatch)
}
