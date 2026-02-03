package commanderclient

import "fmt"

// HyperlinkResolver receives a URI and returns the replacement URI.
// Return the original URI unchanged if no modification is needed.
// Return an error to abort processing.
type HyperlinkResolver func(uri string) (newUri string, err error)

// ProcessHyperlinks finds all hyperlinks in a RichText field and applies the resolver
// to each hyperlink's URI. This is useful for fixing URLs during content migration,
// such as converting German URLs to English equivalents.
//
// The function modifies the entity's field in-place for the specified locale.
// Only RichText fields are supported; string fields will return an error.
//
// Example:
//
//	resolver := func(uri string) (string, error) {
//	    // Convert German URLs to English
//	    if strings.HasPrefix(uri, "/de/") {
//	        return strings.Replace(uri, "/de/", "/en/", 1), nil
//	    }
//	    return uri, nil
//	}
//	err := ProcessHyperlinks(entry, "content", cc.Locale("en"), resolver)
func ProcessHyperlinks(
	entity Entity,
	fieldName string,
	locale Locale,
	resolver HyperlinkResolver,
) error {
	value := entity.GetFieldValue(fieldName, locale)
	if value == nil {
		return nil
	}

	// Parse as RichText
	rt, err := parseRichText(value)
	if err != nil {
		return fmt.Errorf("failed to parse field '%s' as RichText: %w", fieldName, err)
	}

	if !rt.isDocument() {
		return fmt.Errorf("field '%s' is not a RichText document", fieldName)
	}

	// Track if any modifications were made
	modified := false

	// Walk all hyperlinks and apply resolver
	err = rt.walkHyperlinks(func(node *RichTextNode) error {
		uri := node.getHyperlinkURI()
		if uri == "" {
			return nil
		}

		newUri, err := resolver(uri)
		if err != nil {
			return fmt.Errorf("resolver failed for URI '%s': %w", uri, err)
		}

		if newUri != uri {
			node.setHyperlinkURI(newUri)
			modified = true
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Only update the field if modifications were made
	if modified {
		entity.SetFieldValue(fieldName, locale, rt)
	}

	return nil
}

// ProcessHyperlinksInFields processes hyperlinks in multiple fields.
// This is a convenience function for processing several RichText fields at once.
// Errors are collected and returned as a combined error; processing continues
// even if some fields fail.
func ProcessHyperlinksInFields(
	entity Entity,
	fieldNames []string,
	locale Locale,
	resolver HyperlinkResolver,
) error {
	var errors []error

	for _, fieldName := range fieldNames {
		if err := ProcessHyperlinks(entity, fieldName, locale, resolver); err != nil {
			errors = append(errors, fmt.Errorf("field '%s': %w", fieldName, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("hyperlink processing failed for %d field(s): %v", len(errors), errors)
	}

	return nil
}
