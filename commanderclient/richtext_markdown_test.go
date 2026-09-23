package commanderclient

import (
	"encoding/json"
	"reflect"
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
		{name: "tilde code block", markdown: "~~~\ncode\n~~~"},
		{name: "code after escaped backtick", markdown: "\\` then `code`"},
		{name: "HTML after escaped angle bracket", markdown: "\\<a <b>bold</b>"},
		{name: "blockquote after escaped one", markdown: "\\> fine\n\n> Quote"},
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
		"- not a list",
		"+ not a list",
		"1. not a list",
		"2) not a list",
		"  - indented dash",
		"line one\n10. line two",
		"3.14 is pi",
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
		if len(node.Content) != 1 || node.Content[0].NodeType != nodeTypeParagraph {
			t.Fatalf("expected a single paragraph for %q (markdown %q), got %#v", original, markdown, node.Content)
		}
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

func TestRichTextMarkdownMarkedWhitespaceRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		nodes []*RichTextNode
	}{
		{name: "italic leading space", nodes: []*RichTextNode{
			{NodeType: nodeTypeText, Value: " foo", Data: map[string]any{}, Marks: []RichTextMark{{Type: markTypeItalic}}},
		}},
		{name: "bold trailing space", nodes: []*RichTextNode{
			{NodeType: nodeTypeText, Value: "foo ", Data: map[string]any{}, Marks: []RichTextMark{{Type: markTypeBold}}},
			{NodeType: nodeTypeText, Value: "bar", Data: map[string]any{}},
		}},
		{name: "italic whitespace only", nodes: []*RichTextNode{
			{NodeType: nodeTypeText, Value: " ", Data: map[string]any{}, Marks: []RichTextMark{{Type: markTypeItalic}}},
			{NodeType: nodeTypeText, Value: "x", Data: map[string]any{}},
		}},
		{name: "empty bold", nodes: []*RichTextNode{
			{NodeType: nodeTypeText, Value: "", Data: map[string]any{}, Marks: []RichTextMark{{Type: markTypeBold}}},
			{NodeType: nodeTypeText, Value: "x", Data: map[string]any{}},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := &RichTextNode{
				NodeType: nodeTypeDocument,
				Data:     map[string]any{},
				Content:  []*RichTextNode{{NodeType: nodeTypeParagraph, Data: map[string]any{}, Content: tt.nodes}},
			}
			var want strings.Builder
			for _, n := range tt.nodes {
				want.WriteString(n.Value)
			}

			markdown, _, err := RichTextToMarkdown(doc)
			if err != nil {
				t.Fatalf("RichTextToMarkdown returned error: %v", err)
			}
			value, err := MarkdownToRichText(markdown)
			if err != nil {
				t.Fatalf("MarkdownToRichText(%q) returned error: %v", markdown, err)
			}
			node := value.(*RichTextNode)
			if len(node.Content) != 1 || node.Content[0].NodeType != nodeTypeParagraph {
				t.Fatalf("expected a single paragraph for markdown %q, got %#v", markdown, node.Content)
			}
			var got strings.Builder
			for _, n := range node.Content[0].Content {
				got.WriteString(n.Value)
			}
			// The marked text itself (without its edge whitespace) must keep its marks.
			if core := strings.TrimSpace(tt.nodes[0].Value); core != "" {
				if first := findTextNode(node.Content[0].Content, core); first == nil || len(first.Marks) != len(tt.nodes[0].Marks) {
					t.Fatalf("marks lost on %q (markdown %q): %#v", core, markdown, first)
				}
			}
			if got.String() != want.String() {
				t.Fatalf("round trip mismatch: want %q -> markdown %q -> %q", want.String(), markdown, got.String())
			}
		})
	}
}

func TestRichTextMarkdownListItemNewlineRoundTrip(t *testing.T) {
	values := []string{
		"a\nb",
		"a\n- b",
		"a\n1. b",
		"a\n# b",
	}

	for _, original := range values {
		doc := &RichTextNode{
			NodeType: nodeTypeDocument,
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
						Content:  []*RichTextNode{{NodeType: nodeTypeText, Value: original, Data: map[string]any{}}},
					}},
				}},
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
		if len(node.Content) != 1 || node.Content[0].NodeType != nodeTypeUnorderedList || len(node.Content[0].Content) != 1 {
			t.Fatalf("expected a single one-item list for %q (markdown %q), got %#v", original, markdown, node.Content)
		}
		got := node.Content[0].Content[0].Content[0].Content[0].Value
		if got != original {
			t.Fatalf("round trip mismatch: original %q -> markdown %q -> %q", original, markdown, got)
		}
	}
}

func TestMarkdownToRichTextListLazyContinuation(t *testing.T) {
	value, err := MarkdownToRichText("- one\ncontinued\n- two\n\nafter")
	if err != nil {
		t.Fatalf("MarkdownToRichText returned error: %v", err)
	}
	node := value.(*RichTextNode)
	if len(node.Content) != 2 || node.Content[0].NodeType != nodeTypeUnorderedList || node.Content[1].NodeType != nodeTypeParagraph {
		t.Fatalf("expected list then paragraph, got %#v", node.Content)
	}
	items := node.Content[0].Content
	if len(items) != 2 {
		t.Fatalf("expected two list items, got %d", len(items))
	}
	if got := items[0].Content[0].Content[0].Value; got != "one\ncontinued" {
		t.Fatalf("expected continuation joined into first item, got %q", got)
	}
}

func TestRichTextToMarkdownHeadingNewlineWarns(t *testing.T) {
	doc := &RichTextNode{
		NodeType: nodeTypeDocument,
		Data:     map[string]any{},
		Content: []*RichTextNode{{
			NodeType: nodeTypeHeading2,
			Data:     map[string]any{},
			Content:  []*RichTextNode{{NodeType: nodeTypeText, Value: "Title\nsubtitle", Data: map[string]any{}}},
		}},
	}

	markdown, warnings, err := RichTextToMarkdown(doc)
	if err != nil {
		t.Fatalf("RichTextToMarkdown returned error: %v", err)
	}
	if markdown != "## Title subtitle" {
		t.Fatalf("expected newline collapsed to a space, got %q", markdown)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected one newline warning, got %#v", warnings)
	}
}

func findTextNode(nodes []*RichTextNode, value string) *RichTextNode {
	for _, n := range nodes {
		if n.Value == value {
			return n
		}
	}
	return nil
}

// assertRichTextRoundTrip renders doc to Markdown, parses it back and requires the
// rebuilt document to equal the source in both text and block structure.
func assertRichTextRoundTrip(t *testing.T, doc *RichTextNode) {
	t.Helper()
	markdown, warnings, err := RichTextToMarkdown(doc)
	if err != nil {
		t.Fatalf("RichTextToMarkdown returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
	value, err := MarkdownToRichText(markdown)
	if err != nil {
		t.Fatalf("MarkdownToRichText(%q) returned error: %v", markdown, err)
	}
	if !reflect.DeepEqual(value, doc) {
		want, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("marshal source document: %v", err)
		}
		got, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal rebuilt document: %v", err)
		}
		t.Fatalf("round trip mismatch via markdown %q:\nwant %s\n got %s", markdown, want, got)
	}
}

func richTextTextNode(value string) *RichTextNode {
	return &RichTextNode{NodeType: nodeTypeText, Value: value, Data: map[string]any{}, Marks: []RichTextMark{}}
}

func TestRichTextMarkdownParagraphRoundTrip(t *testing.T) {
	texts := []string{
		// Previously rejected on the way back.
		"> Zitat aus der Presse",
		"Rabatt `10%` heute",
		"Größe <XL> verfügbar",
		// Previously working, kept as contrast.
		"Preise < 50 Franken",
		"100% Wolle",
		"Preis: 20.50 statt 30.00",
		"1) Erstens",
		"* Stern",
		"[Kein Link",
		`Pfad C:\Ordner`,
		`2026\. schon escaped`,
		"2026. Neue Kollektion",
		"- Sale",
		// Same class: line-level block syntax inside text.
		">",
		"  > eingerückt",
		"Erste Zeile\n> zweite Zeile",
		"```",
		"~~~ Wellen",
		"---",
		"a | b\n--- | ---",
		"Spalte | Wert\n:-: | -",
		"<b>fett</b> und <br/>",
		"a <b und c> d",
		"Mix `a` <X> > y",
		`\` + "`",
	}

	for _, text := range texts {
		t.Run(text, func(t *testing.T) {
			assertRichTextRoundTrip(t, &RichTextNode{
				NodeType: nodeTypeDocument,
				Data:     map[string]any{},
				Content: []*RichTextNode{{
					NodeType: nodeTypeParagraph,
					Data:     map[string]any{},
					Content:  []*RichTextNode{richTextTextNode(text)},
				}},
			})
		})
	}
}

func TestRichTextMarkdownSyntaxInOtherBlocksRoundTrip(t *testing.T) {
	paragraph := func(text string) *RichTextNode {
		return &RichTextNode{NodeType: nodeTypeParagraph, Data: map[string]any{}, Content: []*RichTextNode{richTextTextNode(text)}}
	}
	tests := []struct {
		name  string
		block *RichTextNode
	}{
		{name: "list item continuation blockquote", block: &RichTextNode{
			NodeType: nodeTypeUnorderedList,
			Data:     map[string]any{},
			Content: []*RichTextNode{{
				NodeType: nodeTypeListItem,
				Data:     map[string]any{},
				Content:  []*RichTextNode{paragraph("a\n> b\n---")},
			}},
		}},
		{name: "heading", block: &RichTextNode{
			NodeType: nodeTypeHeading2,
			Data:     map[string]any{},
			Content:  []*RichTextNode{richTextTextNode("> `Code` <XL>")},
		}},
		{name: "table cell", block: &RichTextNode{
			NodeType: nodeTypeTable,
			Data:     map[string]any{},
			Content: []*RichTextNode{{
				NodeType: nodeTypeTableRow,
				Data:     map[string]any{},
				Content: []*RichTextNode{{
					NodeType: nodeTypeTableHeaderCell,
					Data:     map[string]any{},
					Content:  []*RichTextNode{paragraph("> `Code` <XL>")},
				}},
			}},
		}},
		{name: "hyperlink uri", block: &RichTextNode{
			NodeType: nodeTypeParagraph,
			Data:     map[string]any{},
			Content: []*RichTextNode{{
				NodeType: nodeTypeHyperlink,
				Data:     map[string]any{"uri": "https://example.com/?q=<a>&c=`x`"},
				Content:  []*RichTextNode{richTextTextNode("Link")},
			}},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertRichTextRoundTrip(t, &RichTextNode{
				NodeType: nodeTypeDocument,
				Data:     map[string]any{},
				Content:  []*RichTextNode{tt.block},
			})
		})
	}
}
