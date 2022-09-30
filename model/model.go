package model

type ContentTypeSysAttributes struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	LinkType string `json:"linkType,omitempty"`
}

type ContentTypeSys struct {
	Sys ContentTypeSysAttributes `json:"sys,omitempty"`
}

type ContentfulSys struct {
	ID          string         `json:"id,omitempty"`
	Type        string         `json:"type,omitempty"`
	LinkType    string         `json:"linkType,omitempty"`
	ContentType ContentTypeSys `json:"contentType,omitempty"`
	CreatedAt   string         `json:"createdAt,omitempty"`
	UpdatedAt   string         `json:"updatedAt,omitempty"`
	Revision    float64        `json:"revision,omitempty"`
	Version     float64        `json:"version,omitempty"`
}

type ContentTypeFieldItemsValidation struct {
	LinkContentType []string `json:"linkContentType,omitempty"`
}

type ContentTypeFieldItems struct {
	Type        string                            `json:"type,omitempty"`
	Validations []ContentTypeFieldItemsValidation `json:"validations,omitempty"`
	LinkType    string                            `json:"linkType,omitempty"`
}

type ContentTypeField struct {
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Disabled    bool                   `json:"disabled,omitempty"`
	Items       *ContentTypeFieldItems `json:"items,omitempty"`
	LinkType    string                 `json:"linkType,omitempty"`
	Localized   bool                   `json:"localized,omitempty"`
	Omitted     bool                   `json:"omitted,omitempty"`
	Required    bool                   `json:"required,omitempty"`
	Validations []interface{}          `json:"validations,omitempty"`
}

type ContentType struct {
	Sys         ContentfulSys      `json:"sys,omitempty"`
	Name        string             `json:"name,omitempty"`
	Description string             `json:"description,omitempty"`
	Fields      []ContentTypeField `json:"fields,omitempty"`
}

type Locale struct {
	Name         string `json:"name,omitempty"`
	Code         string `json:"code,omitempty"`
	FallbackCode string `json:"fallbackCode,omitempty"`
	Default      bool   `json:"default,omitempty"`
}

type ReferenceSysAttributes struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	LinkType string `json:"linkType,omitempty"`
}

type ReferenceSys struct {
	Sys ReferenceSysAttributes `json:"sys,omitempty"`
}
