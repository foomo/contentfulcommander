package common

type ReferenceSysAttributes struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	LinkType string `json:"linkType,omitempty"`
}

type ReferenceSys struct {
	Sys ReferenceSysAttributes `json:"sys,omitempty"`
}
