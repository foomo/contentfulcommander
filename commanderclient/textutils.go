package commanderclient

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

func MatchCase(input, reference string) string {
	if len(reference) == 0 || len(input) == 0 {
		return input
	}

	if strings.ToUpper(reference) == reference {
		return strings.ToUpper(input)
	}

	if strings.ToLower(reference) == reference {
		return strings.ToLower(input)
	}

	if unicode.IsUpper([]rune(reference)[0]) {
		runes := []rune(input)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			for i := 1; i < len(runes); i++ {
				runes[i] = unicode.ToLower(runes[i])
			}
			return string(runes)
		}
	}
	return input
}

func ToLowerURL(input string) string {
	if strings.HasPrefix(strings.ToLower(input), "http") {
		return strings.ToLower(input[:1]) + input[1:]
	}
	return input
}

// FixURI strips diacritics, lowercases, and replaces non-alphanumeric
// characters with dashes, producing a clean URL-safe slug.
func FixURI(input string) string {
	input = strings.TrimSpace(input)
	// Decompose into base characters + combining marks, then remove the marks
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ := transform.String(t, input)
	result = strings.ToLower(result)
	// Replace any character that isn't a letter, digit, or dash with a dash
	result = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(result, "-")
	// Collapse multiple dashes and trim
	result = regexp.MustCompile(`-{2,}`).ReplaceAllString(result, "-")
	result = strings.Trim(result, "-")
	return result
}
