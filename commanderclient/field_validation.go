package commanderclient

import "github.com/foomo/contentful"

// FieldValidationSummary contains normalized field validation values for UI/API serialization.
type FieldValidationSummary struct {
	MinLength     *int   `json:"minLength,omitempty"`
	MaxLength     *int   `json:"maxLength,omitempty"`
	AllowedValues []any  `json:"allowedValues,omitempty"`
	RegexPattern  string `json:"regexPattern,omitempty"`
	RegexFlags    string `json:"regexFlags,omitempty"`
}

// FieldIsEditable returns true when the field exists and is neither disabled nor omitted.
func FieldIsEditable(field *contentful.Field) bool {
	return field != nil && !field.Disabled && !field.Omitted
}

// FieldMaxLength returns the strictest max size validation for the field.
func FieldMaxLength(field *contentful.Field) (int, bool) {
	if field == nil {
		return 0, false
	}

	var max int
	for _, validation := range field.Validations {
		size := fieldValidationSize(validation)
		if size == nil || size.Size == nil || size.Size.Max <= 0 {
			continue
		}
		current := int(size.Size.Max)
		if max == 0 || current < max {
			max = current
		}
	}

	if max == 0 {
		return 0, false
	}
	return max, true
}

// FieldMinLength returns the strictest min size validation for the field.
func FieldMinLength(field *contentful.Field) (int, bool) {
	if field == nil {
		return 0, false
	}

	var min int
	for _, validation := range field.Validations {
		size := fieldValidationSize(validation)
		if size == nil || size.Size == nil || size.Size.Min <= 0 {
			continue
		}
		current := int(size.Size.Min)
		if current > min {
			min = current
		}
	}

	if min == 0 {
		return 0, false
	}
	return min, true
}

// FieldAllowedValues returns the first predefined-values validation for the field.
func FieldAllowedValues(field *contentful.Field) ([]any, bool) {
	if field == nil {
		return nil, false
	}

	for _, validation := range field.Validations {
		predefined := fieldValidationPredefinedValues(validation)
		if predefined == nil || len(predefined.In) == 0 {
			continue
		}
		values := make([]any, len(predefined.In))
		copy(values, predefined.In)
		return values, true
	}

	return nil, false
}

// FieldRegex returns the first regular-expression validation for the field.
func FieldRegex(field *contentful.Field) (pattern string, flags string, ok bool) {
	if field == nil {
		return "", "", false
	}

	for _, validation := range field.Validations {
		regex := fieldValidationRegex(validation)
		if regex == nil || regex.Regex == nil || regex.Regex.Pattern == "" {
			continue
		}
		return regex.Regex.Pattern, regex.Regex.Flags, true
	}

	return "", "", false
}

// GetFieldValidationSummary returns a normalized summary of supported field validations.
func GetFieldValidationSummary(field *contentful.Field) FieldValidationSummary {
	summary := FieldValidationSummary{}

	if minLength, ok := FieldMinLength(field); ok {
		summary.MinLength = &minLength
	}
	if maxLength, ok := FieldMaxLength(field); ok {
		summary.MaxLength = &maxLength
	}
	if allowedValues, ok := FieldAllowedValues(field); ok {
		summary.AllowedValues = allowedValues
	}
	if pattern, flags, ok := FieldRegex(field); ok {
		summary.RegexPattern = pattern
		summary.RegexFlags = flags
	}

	return summary
}

func fieldValidationSize(validation contentful.FieldValidation) *contentful.FieldValidationSize {
	switch typed := validation.(type) {
	case contentful.FieldValidationSize:
		return &typed
	case *contentful.FieldValidationSize:
		return typed
	default:
		return nil
	}
}

func fieldValidationPredefinedValues(validation contentful.FieldValidation) *contentful.FieldValidationPredefinedValues {
	switch typed := validation.(type) {
	case contentful.FieldValidationPredefinedValues:
		return &typed
	case *contentful.FieldValidationPredefinedValues:
		return typed
	default:
		return nil
	}
}

func fieldValidationRegex(validation contentful.FieldValidation) *contentful.FieldValidationRegex {
	switch typed := validation.(type) {
	case contentful.FieldValidationRegex:
		return &typed
	case *contentful.FieldValidationRegex:
		return typed
	default:
		return nil
	}
}
