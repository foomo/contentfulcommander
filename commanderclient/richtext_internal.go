package commanderclient

import (
	"encoding/json"
	"fmt"
)

// Private RichText node type constants
const (
	nodeTypeDocument        = "document"
	nodeTypeParagraph       = "paragraph"
	nodeTypeHeading1        = "heading-1"
	nodeTypeHeading2        = "heading-2"
	nodeTypeHeading3        = "heading-3"
	nodeTypeHeading4        = "heading-4"
	nodeTypeHeading5        = "heading-5"
	nodeTypeHeading6        = "heading-6"
	nodeTypeText            = "text"
	nodeTypeHyperlink       = "hyperlink"
	nodeTypeEntryHyperlink  = "entry-hyperlink"
	nodeTypeAssetHyperlink  = "asset-hyperlink"
	nodeTypeEmbeddedEntry   = "embedded-entry-block"
	nodeTypeEmbeddedAsset   = "embedded-asset-block"
	nodeTypeUnorderedList   = "unordered-list"
	nodeTypeOrderedList     = "ordered-list"
	nodeTypeListItem        = "list-item"
	nodeTypeBlockquote      = "blockquote"
	nodeTypeHR              = "hr"
	nodeTypeTable           = "table"
	nodeTypeTableRow        = "table-row"
	nodeTypeTableHeaderCell = "table-header-cell"
	nodeTypeTableCell       = "table-cell"
)

// Private mark type constants
const (
	markTypeBold      = "bold"
	markTypeItalic    = "italic"
	markTypeUnderline = "underline"
	markTypeCode      = "code"
)

// Path format for hierarchical text node addressing
const nodePathFormat = "%s-%03d"

// RichTextNode represents a node in a Contentful RichText document
type RichTextNode struct {
	NodeType string          `json:"nodeType"`
	Value    string          `json:"value"`
	Data     map[string]any  `json:"data"`
	Marks    []RichTextMark  `json:"marks"`
	Content  []*RichTextNode `json:"content"`
}

// RichTextMark represents text formatting (bold, italic, etc.)
type RichTextMark struct {
	Type string `json:"type"`
}

func (n *RichTextNode) MarshalJSON() ([]byte, error) {
	if n == nil {
		return []byte("null"), nil
	}
	if n.Data == nil {
		n.Data = make(map[string]interface{})
	}
	if n.NodeType == "text" {
		// For text nodes, include Data, Value and Marks, but not Content
		return json.Marshal(&struct {
			NodeType string                 `json:"nodeType"`
			Data     map[string]interface{} `json:"data"`
			Value    string                 `json:"value"`
			Marks    []RichTextMark         `json:"marks"`
		}{
			NodeType: n.NodeType,
			Data:     n.Data,
			Value:    n.Value,
			Marks:    n.Marks,
		})
	} else {
		// For non-text nodes, exclude Data and Marks
		return json.Marshal(&struct {
			NodeType string                 `json:"nodeType"`
			Data     map[string]interface{} `json:"data"`
			Content  []*RichTextNode        `json:"content"`
		}{
			NodeType: n.NodeType,
			Content:  n.Content,
			Data:     n.Data,
		})
	}
}

// parseRichText converts a raw Contentful field value to a richTextNode
func parseRichText(value any) (*RichTextNode, error) {
	if value == nil {
		return nil, nil
	}
	node := &RichTextNode{}
	bytes, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal value: %w", err)
	}
	if err := json.Unmarshal(bytes, node); err != nil {
		return nil, fmt.Errorf("failed to parse RichText: %w", err)
	}
	return node, nil
}

// isDocument returns true if this node is a RichText document root
func (n *RichTextNode) isDocument() bool {
	return n != nil && n.NodeType == nodeTypeDocument
}

// extractText collects all text node values with hierarchical path keys
// Returns map[path]text where path is like "000-001-002"
func (n *RichTextNode) extractText() map[string]string {
	result := make(map[string]string)
	n.extractTextRecursive(result, "000")
	return result
}

func (n *RichTextNode) extractTextRecursive(textByPath map[string]string, path string) {
	if n.NodeType == nodeTypeText && len(n.Value) > 0 {
		textByPath[path] = n.Value
	}
	for i, child := range n.Content {
		child.extractTextRecursive(textByPath, fmt.Sprintf(nodePathFormat, path, i))
	}
}

// replaceText updates text nodes using a path->value map
func (n *RichTextNode) replaceText(replacements map[string]string) {
	n.replaceTextRecursive(replacements, "000")
}

func (n *RichTextNode) replaceTextRecursive(textByPath map[string]string, path string) {
	if value, ok := textByPath[path]; ok {
		n.Value = value
	}
	for i, child := range n.Content {
		child.replaceTextRecursive(textByPath, fmt.Sprintf(nodePathFormat, path, i))
	}
}

// walkHyperlinks visits all hyperlink nodes and calls fn for each
func (n *RichTextNode) walkHyperlinks(fn func(node *RichTextNode) error) error {
	return n.walkHyperlinksRecursive(fn)
}

func (n *RichTextNode) walkHyperlinksRecursive(fn func(node *RichTextNode) error) error {
	// Check if this is a hyperlink node
	if n.NodeType == nodeTypeHyperlink ||
		n.NodeType == nodeTypeEntryHyperlink ||
		n.NodeType == nodeTypeAssetHyperlink {
		if err := fn(n); err != nil {
			return err
		}
	}

	// Recurse into children
	for _, child := range n.Content {
		if err := child.walkHyperlinksRecursive(fn); err != nil {
			return err
		}
	}

	return nil
}

// getHyperlinkURI returns the URI from a hyperlink node's data
func (n *RichTextNode) getHyperlinkURI() string {
	if n.Data == nil {
		return ""
	}
	if uri, ok := n.Data["uri"].(string); ok {
		return uri
	}
	return ""
}

// setHyperlinkURI sets the URI in a hyperlink node's data
func (n *RichTextNode) setHyperlinkURI(uri string) {
	if n.Data == nil {
		n.Data = make(map[string]any)
	}
	n.Data["uri"] = uri
}
