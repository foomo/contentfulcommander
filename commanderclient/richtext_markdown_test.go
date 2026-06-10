package commanderclient

import (
	"strings"
	"testing"
)

func TestMarkdownToRichTextAndBackBasicSubset(t *testing.T) {
	input := "# Title\n\nParagraph with **bold** and *italic*\nline two\n\n- One\n- Two\n\n1. First\n2. Second"

	value, err := MarkdownToRichText(input)
	if err != nil {
		t.Fatalf("MarkdownToRichText returned error: %v", err)
	}

	node, err := parseRichText(value)
	if err != nil {
		t.Fatalf("parseRichText returned error: %v", err)
	}
	if !node.isDocument() {
		t.Fatal("expected document node")
	}
	if len(node.Content) != 4 {
		t.Fatalf("expected 4 top-level blocks, got %d", len(node.Content))
	}

	output, warnings, err := RichTextToMarkdown(value)
	if err != nil {
		t.Fatalf("RichTextToMarkdown returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
	if output != input {
		t.Fatalf("expected round trip markdown:\n%s\n\ngot:\n%s", input, output)
	}
}

func TestIsSupportedRichTextMarkdownRejectsUnsupportedConstructs(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
	}{
		{name: "raw HTML", markdown: "<p>Hello</p>"},
		{name: "image", markdown: "![Alt](image.jpg)"},
		{name: "code", markdown: "`code`"},
		{name: "heading 4", markdown: "#### Heading"},
		{name: "blockquote", markdown: "> Quote"},
		{name: "nested list", markdown: "- One\n  - Two"},
		{name: "horizontal rule", markdown: "---"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := IsSupportedRichTextMarkdown(tt.markdown); err == nil {
				t.Fatal("expected unsupported Markdown error")
			}
		})
	}
}

func TestRichTextToMarkdownWarnsAndPreservesSupportedText(t *testing.T) {
	value := &RichTextNode{
		NodeType: nodeTypeDocument,
		Data:     map[string]any{},
		Content: []*RichTextNode{
			{
				NodeType: nodeTypeParagraph,
				Data:     map[string]any{},
				Content: []*RichTextNode{
					{
						NodeType: nodeTypeText,
						Value:    "Plain ",
						Data:     map[string]any{},
						Marks:    []RichTextMark{},
					},
					{
						NodeType: nodeTypeHyperlink,
						Data:     map[string]any{"uri": "https://example.com"},
						Content: []*RichTextNode{{
							NodeType: nodeTypeText,
							Value:    "link",
							Data:     map[string]any{},
							Marks:    []RichTextMark{},
						}},
					},
				},
			},
			{
				NodeType: nodeTypeEmbeddedEntry,
				Data:     map[string]any{},
			},
		},
	}

	markdown, warnings, err := RichTextToMarkdown(value)
	if err != nil {
		t.Fatalf("RichTextToMarkdown returned error: %v", err)
	}
	if markdown != "Plain [link](https://example.com)" {
		t.Fatalf("expected hyperlink to be preserved, got %q", markdown)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected one warning (embedded entry), got %#v", warnings)
	}
}

func TestRichTextToMarkdownRejectsNonDocument(t *testing.T) {
	if _, _, err := RichTextToMarkdown("not rich text"); err == nil {
		t.Fatal("expected non-document RichText error")
	}
}

func TestRichTextMarkdownBoldItalicRoundTrip(t *testing.T) {
	doc := &RichTextNode{
		NodeType: nodeTypeDocument,
		Data:     map[string]any{},
		Content: []*RichTextNode{{
			NodeType: nodeTypeParagraph,
			Data:     map[string]any{},
			Content: []*RichTextNode{{
				NodeType: nodeTypeText,
				Value:    "bold italic",
				Data:     map[string]any{},
				Marks:    []RichTextMark{{Type: markTypeBold}, {Type: markTypeItalic}},
			}},
		}},
	}

	markdown, warnings, err := RichTextToMarkdown(doc)
	if err != nil {
		t.Fatalf("RichTextToMarkdown returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
	if markdown != "***bold italic***" {
		t.Fatalf("expected ***bold italic***, got %q", markdown)
	}

	value, err := MarkdownToRichText(markdown)
	if err != nil {
		t.Fatalf("MarkdownToRichText returned error: %v", err)
	}
	node := value.(*RichTextNode)
	text := node.Content[0].Content[0]
	if text.Value != "bold italic" {
		t.Fatalf("expected round-tripped value, got %q", text.Value)
	}
	if len(text.Marks) != 2 {
		t.Fatalf("expected bold and italic marks, got %#v", text.Marks)
	}
}

func TestRichTextOrderedListNumberingSkipsNonItems(t *testing.T) {
	listItem := func(text string) *RichTextNode {
		return &RichTextNode{
			NodeType: nodeTypeListItem,
			Data:     map[string]any{},
			Content: []*RichTextNode{{
				NodeType: nodeTypeParagraph,
				Data:     map[string]any{},
				Content:  []*RichTextNode{{NodeType: nodeTypeText, Value: text, Data: map[string]any{}}},
			}},
		}
	}

	doc := &RichTextNode{
		NodeType: nodeTypeDocument,
		Data:     map[string]any{},
		Content: []*RichTextNode{{
			NodeType: nodeTypeOrderedList,
			Data:     map[string]any{},
			Content: []*RichTextNode{
				nil,
				{NodeType: nodeTypeEmbeddedEntry, Data: map[string]any{}},
				listItem("first"),
				listItem("second"),
			},
		}},
	}

	markdown, _, err := RichTextToMarkdown(doc)
	if err != nil {
		t.Fatalf("RichTextToMarkdown returned error: %v", err)
	}
	if markdown != "1. first\n2. second" {
		t.Fatalf("expected sequential numbering, got %q", markdown)
	}
}

func TestRichTextMarkdownEscapedTextRoundTrip(t *testing.T) {
	values := []string{
		"snake_case_var",
		"C# is great (really)",
		"a_b",
		`back\slash`,
		"asterisk * and brackets [x]",
	}

	for _, original := range values {
		doc := &RichTextNode{
			NodeType: nodeTypeDocument,
			Data:     map[string]any{},
			Content: []*RichTextNode{{
				NodeType: nodeTypeParagraph,
				Data:     map[string]any{},
				Content:  []*RichTextNode{{NodeType: nodeTypeText, Value: original, Data: map[string]any{}}},
			}},
		}

		markdown, _, err := RichTextToMarkdown(doc)
		if err != nil {
			t.Fatalf("RichTextToMarkdown(%q) returned error: %v", original, err)
		}
		value, err := MarkdownToRichText(markdown)
		if err != nil {
			t.Fatalf("MarkdownToRichText(%q) returned error: %v", markdown, err)
		}
		node := value.(*RichTextNode)
		got := node.Content[0].Content[0].Value
		if got != original {
			t.Fatalf("round trip mismatch: original %q -> markdown %q -> %q", original, markdown, got)
		}
	}
}

func TestRichTextMarkdownHyperlinkRoundTrip(t *testing.T) {
	inputs := []string{
		"Visit [the site](https://example.com) now",
		"A [**bold** link](https://example.com/path)",
		"See [the product](entry:abc123) details",
		"Download [the file](asset:xyz789)",
		`Link with [parens](https://example.com/foo\(bar\)) here`,
	}

	for _, input := range inputs {
		value, err := MarkdownToRichText(input)
		if err != nil {
			t.Fatalf("MarkdownToRichText(%q) error: %v", input, err)
		}
		output, warnings, err := RichTextToMarkdown(value)
		if err != nil {
			t.Fatalf("RichTextToMarkdown error: %v", err)
		}
		if len(warnings) != 0 {
			t.Fatalf("unexpected warnings for %q: %#v", input, warnings)
		}
		if output != input {
			t.Fatalf("round trip mismatch:\n in: %q\nout: %q", input, output)
		}
	}
}

func TestMarkdownToRichTextHyperlinkNodeTypes(t *testing.T) {
	value, err := MarkdownToRichText("[ext](https://x.com) [e](entry:E1) [a](asset:A1)")
	if err != nil {
		t.Fatalf("MarkdownToRichText error: %v", err)
	}
	paragraph := value.(*RichTextNode).Content[0]

	var links []*RichTextNode
	for _, n := range paragraph.Content {
		switch n.NodeType {
		case nodeTypeHyperlink, nodeTypeEntryHyperlink, nodeTypeAssetHyperlink:
			links = append(links, n)
		}
	}
	if len(links) != 3 {
		t.Fatalf("expected 3 hyperlink nodes, got %d", len(links))
	}
	if links[0].NodeType != nodeTypeHyperlink || links[0].getHyperlinkURI() != "https://x.com" {
		t.Fatalf("external link wrong: %#v", links[0])
	}
	if lt, id, ok := links[1].getHyperlinkTarget(); !ok || lt != "Entry" || id != "E1" {
		t.Fatalf("entry link wrong: %q %q %t", lt, id, ok)
	}
	if lt, id, ok := links[2].getHyperlinkTarget(); !ok || lt != "Asset" || id != "A1" {
		t.Fatalf("asset link wrong: %q %q %t", lt, id, ok)
	}
}

func TestRichTextMarkdownTableRoundTrip(t *testing.T) {
	inputs := []string{
		"| Name | Price |\n| --- | --- |\n| Widget | 9.99 |\n| Gadget | 19.99 |",
		"| Name | Note |\n| --- | --- |\n| Widget | **new** |",
		"| Link |\n| --- |\n| [site](https://example.com) |",
	}

	for _, input := range inputs {
		value, err := MarkdownToRichText(input)
		if err != nil {
			t.Fatalf("MarkdownToRichText(%q) error: %v", input, err)
		}
		output, warnings, err := RichTextToMarkdown(value)
		if err != nil {
			t.Fatalf("RichTextToMarkdown error: %v", err)
		}
		if len(warnings) != 0 {
			t.Fatalf("unexpected warnings for %q: %#v", input, warnings)
		}
		if output != input {
			t.Fatalf("table round trip mismatch:\n in: %q\nout: %q", input, output)
		}
	}
}

func tableTextCell(cellType, text string) *RichTextNode {
	return &RichTextNode{
		NodeType: cellType,
		Data:     map[string]any{},
		Content: []*RichTextNode{{
			NodeType: nodeTypeParagraph,
			Data:     map[string]any{},
			Content:  []*RichTextNode{{NodeType: nodeTypeText, Value: text, Data: map[string]any{}}},
		}},
	}
}

func tableRow(cells ...*RichTextNode) *RichTextNode {
	return &RichTextNode{NodeType: nodeTypeTableRow, Data: map[string]any{}, Content: cells}
}

func tableDoc(rows ...*RichTextNode) *RichTextNode {
	return &RichTextNode{
		NodeType: nodeTypeDocument,
		Data:     map[string]any{},
		Content:  []*RichTextNode{{NodeType: nodeTypeTable, Data: map[string]any{}, Content: rows}},
	}
}

func warningsContain(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

func TestRichTextTableReadRaggedRowWarns(t *testing.T) {
	doc := tableDoc(
		tableRow(tableTextCell(nodeTypeTableHeaderCell, "A"), tableTextCell(nodeTypeTableHeaderCell, "B")),
		tableRow(tableTextCell(nodeTypeTableCell, "x")),
	)

	markdown, warnings, err := RichTextToMarkdown(doc)
	if err != nil {
		t.Fatalf("RichTextToMarkdown error: %v", err)
	}
	if !strings.Contains(markdown, "| x |  |") {
		t.Fatalf("expected padded ragged row, got %q", markdown)
	}
	if !warningsContain(warnings, "ragged") {
		t.Fatalf("expected ragged-row warning, got %#v", warnings)
	}
}

func TestRichTextTableReadFlattensBlockCellWithWarning(t *testing.T) {
	listCell := &RichTextNode{
		NodeType: nodeTypeTableCell,
		Data:     map[string]any{},
		Content: []*RichTextNode{{
			NodeType: nodeTypeUnorderedList,
			Data:     map[string]any{},
			Content: []*RichTextNode{{
				NodeType: nodeTypeListItem,
				Data:     map[string]any{},
				Content: []*RichTextNode{{
					NodeType: nodeTypeParagraph,
					Data:     map[string]any{},
					Content:  []*RichTextNode{{NodeType: nodeTypeText, Value: "one", Data: map[string]any{}}},
				}},
			}},
		}},
	}
	doc := tableDoc(
		tableRow(tableTextCell(nodeTypeTableHeaderCell, "A")),
		tableRow(listCell),
	)

	_, warnings, err := RichTextToMarkdown(doc)
	if err != nil {
		t.Fatalf("RichTextToMarkdown error: %v", err)
	}
	if !warningsContain(warnings, "flattened") {
		t.Fatalf("expected flatten warning, got %#v", warnings)
	}
}

func TestRichTextTableReadNoHeaderRowWarns(t *testing.T) {
	doc := tableDoc(
		tableRow(tableTextCell(nodeTypeTableCell, "a"), tableTextCell(nodeTypeTableCell, "b")),
		tableRow(tableTextCell(nodeTypeTableCell, "c"), tableTextCell(nodeTypeTableCell, "d")),
	)

	_, warnings, err := RichTextToMarkdown(doc)
	if err != nil {
		t.Fatalf("RichTextToMarkdown error: %v", err)
	}
	if !warningsContain(warnings, "first row used as the Markdown header") {
		t.Fatalf("expected no-header warning, got %#v", warnings)
	}
}
