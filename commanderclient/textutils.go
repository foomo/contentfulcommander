package commanderclient

import (
	"regexp"
	"strings"
	"unicode"
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

func FixURI(input string) string {
	input = strings.TrimSpace(input)
	input = strings.ToLower(input)
	// replace multiple spaces with single dash
	re := regexp.MustCompile(`\s+`)
	input = re.ReplaceAllString(input, "-")
	return input
}
