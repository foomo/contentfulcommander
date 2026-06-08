package commanderclient

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	markdownHeadingPattern       = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	markdownUnorderedListPattern = regexp.MustCompile(`^(\s*)[-+*]\s+(.+)$`)
	markdownOrderedListPattern   = regexp.MustCompile(`^(\s*)\d+[.)]\s+(.+)$`)
	markdownImagePattern         = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	markdownHTMLPattern          = regexp.MustCompile(`<[A-Za-z][^>]*>`)
	tableDividerCellPattern      = regexp.MustCompile(`^:?-+:?$`)
)

// RichTextToMarkdown converts a Contentful RichText document to basic Markdown.
func RichTextToMarkdown(value any) (markdown string, warnings []string, err error) {
	node, err := parseRichText(value)
	if err != nil {
		return "", nil, err
	}
	if !node.isDocument() {
		return "", nil, fmt.Errorf("value is not a Contentful RichText document")
	}

	markdown, warnings = richTextDocumentToMarkdown(node)
	return markdown, warnings, nil
}

// MarkdownToRichText converts supported basic Markdown to a Contentful RichText document.
func MarkdownToRichText(markdown string) (any, error) {
	if err := IsSupportedRichTextMarkdown(markdown); err != nil {
		return nil, err
	}

	content, err := markdownToRichTextBlocks(markdown)
	if err != nil {
		return nil, err
	}

	return &RichTextNode{
		NodeType: nodeTypeDocument,
		Data:     map[string]any{},
		Content:  content,
	}, nil
}

// IsSupportedRichTextMarkdown validates that Markdown uses the supported v1 subset.
func IsSupportedRichTextMarkdown(markdown string) error {
	_, err := markdownToRichTextBlocks(markdown)
	return err
}

func markdownToRichTextBlocks(markdown string) ([]*RichTextNode, error) {
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	markdown = strings.ReplaceAll(markdown, "\r", "\n")

	if strings.TrimSpace(markdown) == "" {
		return []*RichTextNode{}, nil
	}
	if markdownHTMLPattern.MatchString(markdown) {
		return nil, fmt.Errorf("raw HTML is not supported in RichText Markdown")
	}
	if markdownImagePattern.MatchString(markdown) {
		return nil, fmt.Errorf("images are not supported in RichText Markdown")
	}
	if strings.Contains(markdown, "`") {
		return nil, fmt.Errorf("code spans and code blocks are not supported in RichText Markdown")
	}

	lines := strings.Split(markdown, "\n")
	var blocks []*RichTextNode

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if err := validateMarkdownLine(line); err != nil {
			return nil, err
		}

		if match := markdownHeadingPattern.FindStringSubmatch(line); match != nil {
			level := len(match[1])
			if level > 3 {
				return nil, fmt.Errorf("heading level %d is not supported", level)
			}
			content, err := markdownInlineToRichText(match[2])
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, &RichTextNode{
				NodeType: fmt.Sprintf("heading-%d", level),
				Data:     map[string]any{},
				Content:  content,
			})
			continue
		}

		if listType, _, ok, err := parseMarkdownListLine(line); err != nil {
			return nil, err
		} else if ok {
			listNode := &RichTextNode{
				NodeType: listType,
				Data:     map[string]any{},
			}
			for ; i < len(lines); i++ {
				currentLine := lines[i]
				if strings.TrimSpace(currentLine) == "" {
					break
				}
				currentListType, currentItemText, currentOK, currentErr := parseMarkdownListLine(currentLine)
				if currentErr != nil {
					return nil, currentErr
				}
				if !currentOK || currentListType != listType {
					i--
					break
				}
				content, err := markdownInlineToRichText(currentItemText)
				if err != nil {
					return nil, err
				}
				listNode.Content = append(listNode.Content, &RichTextNode{
					NodeType: nodeTypeListItem,
					Data:     map[string]any{},
					Content: []*RichTextNode{{
						NodeType: nodeTypeParagraph,
						Data:     map[string]any{},
						Content:  content,
					}},
				})
			}
			blocks = append(blocks, listNode)
			continue
		}

		if isTableStart(lines, i) {
			tableNode, lastLine, err := parseMarkdownTable(lines, i)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, tableNode)
			i = lastLine
			continue
		}

		paragraphLines := []string{line}
		for i+1 < len(lines) {
			nextLine := lines[i+1]
			if strings.TrimSpace(nextLine) == "" {
				break
			}
			if markdownHeadingPattern.MatchString(nextLine) {
				break
			}
			if _, _, ok, err := parseMarkdownListLine(nextLine); err != nil {
				return nil, err
			} else if ok {
				break
			}
			if isTableStart(lines, i+1) {
				break
			}
			if err := validateMarkdownLine(nextLine); err != nil {
				return nil, err
			}
			paragraphLines = append(paragraphLines, nextLine)
			i++
		}

		content, err := markdownInlineToRichText(strings.Join(paragraphLines, "\n"))
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, &RichTextNode{
			NodeType: nodeTypeParagraph,
			Data:     map[string]any{},
			Content:  content,
		})
	}

	return blocks, nil
}

func validateMarkdownLine(line string) error {
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trimmed, "```"), strings.HasPrefix(trimmed, "~~~"):
		return fmt.Errorf("code blocks are not supported in RichText Markdown")
	case strings.HasPrefix(trimmed, ">"):
		return fmt.Errorf("blockquotes are not supported in RichText Markdown")
	case trimmed == "---" || trimmed == "***" || trimmed == "___":
		return fmt.Errorf("horizontal rules are not supported in RichText Markdown")
	}
	return nil
}

func parseMarkdownListLine(line string) (string, string, bool, error) {
	if match := markdownUnorderedListPattern.FindStringSubmatch(line); match != nil {
		if match[1] != "" {
			return "", "", false, fmt.Errorf("nested or indented lists are not supported")
		}
		return nodeTypeUnorderedList, match[2], true, nil
	}
	if match := markdownOrderedListPattern.FindStringSubmatch(line); match != nil {
		if match[1] != "" {
			return "", "", false, fmt.Errorf("nested or indented lists are not supported")
		}
		return nodeTypeOrderedList, match[2], true, nil
	}
	return "", "", false, nil
}

// isTableStart reports whether a GFM table begins at line i: a row containing a pipe
// followed by a divider line (e.g. "| --- | --- |").
func isTableStart(lines []string, i int) bool {
	if i+1 >= len(lines) {
		return false
	}
	if strings.TrimSpace(lines[i]) == "" || !strings.Contains(lines[i], "|") {
		return false
	}
	return isTableDivider(lines[i+1])
}

// isTableDivider reports whether a line is a GFM table divider (each cell is dashes
// with optional alignment colons).
func isTableDivider(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, "-") {
		return false
	}
	cells := splitTableRow(trimmed)
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !tableDividerCellPattern.MatchString(strings.TrimSpace(cell)) {
			return false
		}
	}
	return true
}

// parseMarkdownTable consumes a table starting at the header line and returns the
// table node plus the index of the last consumed line. Body rows are padded/truncated
// to the header's column count (GFM behavior).
func parseMarkdownTable(lines []string, start int) (*RichTextNode, int, error) {
	headerCells := splitTableRow(lines[start])
	columnCount := len(headerCells)

	tableNode := &RichTextNode{NodeType: nodeTypeTable, Data: map[string]any{}}

	headerRow, err := buildTableRow(headerCells, columnCount, true)
	if err != nil {
		return nil, 0, err
	}
	tableNode.Content = append(tableNode.Content, headerRow)

	last := start + 1 // the divider line
	for r := start + 2; r < len(lines); r++ {
		if strings.TrimSpace(lines[r]) == "" || !strings.Contains(lines[r], "|") {
			break
		}
		bodyRow, err := buildTableRow(splitTableRow(lines[r]), columnCount, false)
		if err != nil {
			return nil, 0, err
		}
		tableNode.Content = append(tableNode.Content, bodyRow)
		last = r
	}

	return tableNode, last, nil
}

// buildTableRow builds a table-row node with columnCount cells, parsing each cell's
// inline content. Missing cells are padded empty; extra cells are dropped.
func buildTableRow(cells []string, columnCount int, header bool) (*RichTextNode, error) {
	cellType := nodeTypeTableCell
	if header {
		cellType = nodeTypeTableHeaderCell
	}
	row := &RichTextNode{NodeType: nodeTypeTableRow, Data: map[string]any{}}
	for c := range columnCount {
		raw := ""
		if c < len(cells) {
			raw = cells[c]
		}
		cellText := strings.ReplaceAll(strings.TrimSpace(raw), `\|`, "|")
		content, err := markdownInlineToRichText(cellText)
		if err != nil {
			return nil, err
		}
		row.Content = append(row.Content, &RichTextNode{
			NodeType: cellType,
			Data:     map[string]any{},
			Content: []*RichTextNode{{
				NodeType: nodeTypeParagraph,
				Data:     map[string]any{},
				Content:  content,
			}},
		})
	}
	return row, nil
}

// splitTableRow splits a table row on unescaped pipes, dropping the empty leading and
// trailing cells produced by optional outer pipes.
func splitTableRow(line string) []string {
	parts := splitUnescaped(strings.TrimSpace(line), '|')
	if len(parts) > 0 && strings.TrimSpace(parts[0]) == "" {
		parts = parts[1:]
	}
	if len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// splitUnescaped splits s on every unescaped occurrence of sep.
func splitUnescaped(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep && !isEscapedAt(s, i) {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

func markdownInlineToRichText(text string) ([]*RichTextNode, error) {
	return markdownInlineToRichTextWithMarks(text, nil)
}

func markdownInlineToRichTextWithMarks(text string, marks []RichTextMark) ([]*RichTextNode, error) {
	var nodes []*RichTextNode
	for len(text) > 0 {
		marker, markTypes, markerIndex := nextMarkdownMark(text)
		link, linkIndex := nextMarkdownLink(text)

		// Process a link if it appears before the next mark (or there is no mark).
		if linkIndex != -1 && (markerIndex == -1 || linkIndex < markerIndex) {
			if linkIndex > 0 {
				nodes = appendMarkdownTextNode(nodes, text[:linkIndex], marks)
			}
			linkContent, err := markdownInlineToRichTextWithMarks(link.label, marks)
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, buildHyperlinkNode(link.target, linkContent))
			text = text[link.end:]
			continue
		}

		if markerIndex == -1 {
			nodes = appendMarkdownTextNode(nodes, text, marks)
			break
		}

		if markerIndex > 0 {
			nodes = appendMarkdownTextNode(nodes, text[:markerIndex], marks)
		}

		start := markerIndex + len(marker)
		endOffset := indexUnescaped(text[start:], marker)
		if endOffset == -1 {
			return nil, fmt.Errorf("unclosed Markdown mark %q", marker)
		}
		end := start + endOffset
		if end == start {
			return nil, fmt.Errorf("empty Markdown mark %q is not supported", marker)
		}

		childMarks := copyRichTextMarks(marks)
		for _, markType := range markTypes {
			childMarks = append(childMarks, RichTextMark{Type: markType})
		}
		childNodes, err := markdownInlineToRichTextWithMarks(text[start:end], childMarks)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, childNodes...)
		text = text[end+len(marker):]
	}

	return nodes, nil
}

func nextMarkdownMark(text string) (marker string, markTypes []string, index int) {
	candidates := []struct {
		marker string
		marks  []string
	}{
		{marker: "***", marks: []string{markTypeBold, markTypeItalic}},
		{marker: "___", marks: []string{markTypeBold, markTypeItalic}},
		{marker: "**", marks: []string{markTypeBold}},
		{marker: "__", marks: []string{markTypeBold}},
		{marker: "*", marks: []string{markTypeItalic}},
		{marker: "_", marks: []string{markTypeItalic}},
	}

	index = -1
	for _, candidate := range candidates {
		candidateIndex := indexUnescaped(text, candidate.marker)
		if candidateIndex == -1 {
			continue
		}
		if index == -1 || candidateIndex < index || candidateIndex == index && len(candidate.marker) > len(marker) {
			marker = candidate.marker
			markTypes = candidate.marks
			index = candidateIndex
		}
	}
	return marker, markTypes, index
}

// indexUnescaped returns the index of the first occurrence of marker in s that
// is not preceded by a Markdown backslash escape, or -1 if there is none.
func indexUnescaped(s, marker string) int {
	from := 0
	for {
		idx := strings.Index(s[from:], marker)
		if idx == -1 {
			return -1
		}
		abs := from + idx
		if !isEscapedAt(s, abs) {
			return abs
		}
		from = abs + 1
	}
}

// isEscapedAt reports whether the byte at pos is preceded by an odd number of backslashes.
func isEscapedAt(s string, pos int) bool {
	backslashes := 0
	for i := pos - 1; i >= 0 && s[i] == '\\'; i-- {
		backslashes++
	}
	return backslashes%2 == 1
}

type markdownLink struct {
	label  string
	target string
	end    int
}

// nextMarkdownLink finds the earliest unescaped [label](target) link in text that
// is not an image. It returns the parsed link and the index of its opening '[',
// or index -1 when there is none.
func nextMarkdownLink(text string) (markdownLink, int) {
	search := 0
	for {
		open := indexUnescaped(text[search:], "[")
		if open == -1 {
			return markdownLink{}, -1
		}
		open += search

		// Skip image syntax: an unescaped '!' immediately before '['.
		if open > 0 && text[open-1] == '!' && !isEscapedAt(text, open-1) {
			search = open + 1
			continue
		}

		closeLabel := indexUnescaped(text[open+1:], "]")
		if closeLabel == -1 {
			return markdownLink{}, -1
		}
		closeLabel += open + 1

		if closeLabel+1 >= len(text) || text[closeLabel+1] != '(' {
			search = open + 1
			continue
		}
		openParen := closeLabel + 1

		closeParen := indexUnescaped(text[openParen+1:], ")")
		if closeParen == -1 {
			search = open + 1
			continue
		}
		closeParen += openParen + 1

		label := text[open+1 : closeLabel]
		target := text[openParen+1 : closeParen]
		if label == "" || target == "" {
			search = open + 1
			continue
		}
		return markdownLink{label: label, target: target, end: closeParen + 1}, open
	}
}

// buildHyperlinkNode constructs a hyperlink node from a link target. The entry:/asset:
// schemes map to entry/asset hyperlinks; any other target is treated as an external uri.
func buildHyperlinkNode(target string, content []*RichTextNode) *RichTextNode {
	node := &RichTextNode{
		Data:    map[string]any{},
		Content: content,
	}
	switch {
	case strings.HasPrefix(target, "entry:"):
		node.NodeType = nodeTypeEntryHyperlink
		node.setHyperlinkTarget("Entry", unescapeMarkdownText(strings.TrimPrefix(target, "entry:")))
	case strings.HasPrefix(target, "asset:"):
		node.NodeType = nodeTypeAssetHyperlink
		node.setHyperlinkTarget("Asset", unescapeMarkdownText(strings.TrimPrefix(target, "asset:")))
	default:
		node.NodeType = nodeTypeHyperlink
		node.setHyperlinkURI(unescapeMarkdownText(target))
	}
	return node
}

func appendMarkdownTextNode(nodes []*RichTextNode, value string, marks []RichTextMark) []*RichTextNode {
	value = unescapeMarkdownText(value)
	if value == "" {
		return nodes
	}
	return append(nodes, &RichTextNode{
		NodeType: nodeTypeText,
		Value:    value,
		Data:     map[string]any{},
		Marks:    copyRichTextMarks(marks),
	})
}

func copyRichTextMarks(marks []RichTextMark) []RichTextMark {
	if len(marks) == 0 {
		return []RichTextMark{}
	}
	out := make([]RichTextMark, len(marks))
	copy(out, marks)
	return out
}

func richTextDocumentToMarkdown(node *RichTextNode) (string, []string) {
	var blocks []string
	var warnings []string
	for _, child := range node.Content {
		block, childWarnings := richTextBlockToMarkdown(child)
		warnings = append(warnings, childWarnings...)
		if strings.TrimSpace(block) != "" {
			blocks = append(blocks, block)
		}
	}
	return strings.Join(blocks, "\n\n"), warnings
}

func richTextBlockToMarkdown(node *RichTextNode) (string, []string) {
	if node == nil {
		return "", nil
	}

	switch node.NodeType {
	case nodeTypeParagraph:
		return richTextInlineToMarkdown(node.Content)
	case nodeTypeHeading1, nodeTypeHeading2, nodeTypeHeading3:
		level := strings.TrimPrefix(node.NodeType, "heading-")
		text, warnings := richTextInlineToMarkdown(node.Content)
		return strings.Repeat("#", int(level[0]-'0')) + " " + text, warnings
	case nodeTypeHeading4, nodeTypeHeading5, nodeTypeHeading6:
		text, warnings := richTextInlineToMarkdown(node.Content)
		warnings = append(warnings, fmt.Sprintf("unsupported RichText node %q rendered as plain text", node.NodeType))
		return text, warnings
	case nodeTypeUnorderedList:
		return richTextListToMarkdown(node, false)
	case nodeTypeOrderedList:
		return richTextListToMarkdown(node, true)
	case nodeTypeTable:
		return richTextTableToMarkdown(node)
	case nodeTypeText, nodeTypeHyperlink, nodeTypeEntryHyperlink, nodeTypeAssetHyperlink:
		return richTextInlineNodeToMarkdown(node)
	default:
		text, warnings := richTextInlineToMarkdown(node.Content)
		warnings = append(warnings, fmt.Sprintf("unsupported RichText node %q rendered as plain text where possible", node.NodeType))
		return text, warnings
	}
}

func richTextListToMarkdown(node *RichTextNode, ordered bool) (string, []string) {
	var lines []string
	var warnings []string
	itemNumber := 0
	for _, child := range node.Content {
		if child == nil {
			continue
		}
		if child.NodeType != nodeTypeListItem {
			childMarkdown, childWarnings := richTextBlockToMarkdown(child)
			warnings = append(warnings, childWarnings...)
			warnings = append(warnings, fmt.Sprintf("unsupported child node %q in list rendered as plain text", child.NodeType))
			if childMarkdown != "" {
				lines = append(lines, childMarkdown)
			}
			continue
		}

		itemNumber++
		itemText, childWarnings := richTextListItemToMarkdown(child)
		warnings = append(warnings, childWarnings...)
		prefix := "- "
		if ordered {
			prefix = fmt.Sprintf("%d. ", itemNumber)
		}
		lines = append(lines, prefix+itemText)
	}
	return strings.Join(lines, "\n"), warnings
}

func richTextListItemToMarkdown(node *RichTextNode) (string, []string) {
	var parts []string
	var warnings []string
	for _, child := range node.Content {
		if child == nil {
			continue
		}
		if child.NodeType == nodeTypeParagraph {
			text, childWarnings := richTextInlineToMarkdown(child.Content)
			warnings = append(warnings, childWarnings...)
			parts = append(parts, text)
			continue
		}
		text, childWarnings := richTextBlockToMarkdown(child)
		warnings = append(warnings, childWarnings...)
		warnings = append(warnings, fmt.Sprintf("unsupported RichText list item child %q rendered as plain text", child.NodeType))
		parts = append(parts, text)
	}
	return strings.Join(parts, " "), warnings
}

// richTextTableToMarkdown renders a table node as a GitHub-flavored Markdown table.
// Row 0 becomes the header row (GFM requires one). Content GFM cannot represent —
// block content in cells, header cells outside row 0, ragged rows — degrades with warnings.
func richTextTableToMarkdown(node *RichTextNode) (string, []string) {
	var warnings []string

	var rowNodes []*RichTextNode
	for _, child := range node.Content {
		if child == nil {
			continue
		}
		if child.NodeType != nodeTypeTableRow {
			warnings = append(warnings, fmt.Sprintf("unsupported table child node %q ignored", child.NodeType))
			continue
		}
		rowNodes = append(rowNodes, child)
	}
	if len(rowNodes) == 0 {
		return "", warnings
	}

	rows := make([][]string, len(rowNodes))
	columnCount := 0
	for r, rowNode := range rowNodes {
		var cells []string
		for _, cellNode := range rowNode.Content {
			if cellNode == nil {
				continue
			}
			if cellNode.NodeType == nodeTypeTableHeaderCell && r != 0 {
				warnings = append(warnings, "table header cell outside the first row rendered as a regular cell")
			}
			cellText, cellWarnings := richTextTableCellToMarkdown(cellNode)
			warnings = append(warnings, cellWarnings...)
			cells = append(cells, cellText)
		}
		rows[r] = cells
		if len(cells) > columnCount {
			columnCount = len(cells)
		}
	}
	if columnCount == 0 {
		return "", warnings
	}

	if !rowIsAllHeaderCells(rowNodes[0]) {
		warnings = append(warnings, "table first row used as the Markdown header row")
	}

	var lines []string
	for r, cells := range rows {
		if len(cells) != columnCount {
			warnings = append(warnings, "ragged table row padded to match column count")
			for len(cells) < columnCount {
				cells = append(cells, "")
			}
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
		if r == 0 {
			divider := make([]string, columnCount)
			for i := range divider {
				divider[i] = "---"
			}
			lines = append(lines, "| "+strings.Join(divider, " | ")+" |")
		}
	}
	return strings.Join(lines, "\n"), warnings
}

func rowIsAllHeaderCells(rowNode *RichTextNode) bool {
	hasCell := false
	for _, cell := range rowNode.Content {
		if cell == nil {
			continue
		}
		hasCell = true
		if cell.NodeType != nodeTypeTableHeaderCell {
			return false
		}
	}
	return hasCell
}

// richTextTableCellToMarkdown flattens a cell's content to single-line inline text,
// escaping pipes and collapsing newlines. Block content is flattened with a warning.
func richTextTableCellToMarkdown(node *RichTextNode) (string, []string) {
	var parts []string
	var warnings []string
	for _, child := range node.Content {
		if child == nil {
			continue
		}
		if child.NodeType == nodeTypeParagraph {
			text, childWarnings := richTextInlineToMarkdown(child.Content)
			warnings = append(warnings, childWarnings...)
			parts = append(parts, text)
			continue
		}
		text, childWarnings := richTextBlockToMarkdown(child)
		warnings = append(warnings, childWarnings...)
		warnings = append(warnings, fmt.Sprintf("table cell child %q flattened to text", child.NodeType))
		parts = append(parts, text)
	}
	return escapeTableCell(strings.Join(parts, " ")), warnings
}

// escapeTableCell makes inline text safe for a single GFM table cell.
func escapeTableCell(text string) string {
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "|", `\|`)
	return strings.TrimSpace(text)
}

func richTextInlineToMarkdown(nodes []*RichTextNode) (string, []string) {
	var builder strings.Builder
	var warnings []string
	for _, child := range nodes {
		text, childWarnings := richTextInlineNodeToMarkdown(child)
		warnings = append(warnings, childWarnings...)
		builder.WriteString(text)
	}
	return builder.String(), warnings
}

func richTextInlineNodeToMarkdown(node *RichTextNode) (string, []string) {
	if node == nil {
		return "", nil
	}

	switch node.NodeType {
	case nodeTypeText:
		text := escapeMarkdownText(node.Value)
		var warnings []string
		hasBold := false
		hasItalic := false
		for _, mark := range node.Marks {
			switch mark.Type {
			case markTypeBold:
				hasBold = true
			case markTypeItalic:
				hasItalic = true
			default:
				warnings = append(warnings, fmt.Sprintf("unsupported RichText mark %q ignored", mark.Type))
			}
		}
		switch {
		case hasBold && hasItalic:
			text = "***" + text + "***"
		case hasBold:
			text = "**" + text + "**"
		case hasItalic:
			text = "*" + text + "*"
		}
		return text, warnings
	case nodeTypeHyperlink, nodeTypeEntryHyperlink, nodeTypeAssetHyperlink:
		return richTextHyperlinkToMarkdown(node)
	default:
		text, warnings := richTextInlineToMarkdown(node.Content)
		if node.NodeType != "" {
			warnings = append(warnings, fmt.Sprintf("unsupported RichText inline node %q rendered as plain text where possible", node.NodeType))
		}
		return text, warnings
	}
}

// richTextHyperlinkToMarkdown renders a hyperlink node as [label](target). External
// hyperlinks use their uri; entry/asset hyperlinks use the entry:/asset: id scheme.
func richTextHyperlinkToMarkdown(node *RichTextNode) (string, []string) {
	label, warnings := richTextInlineToMarkdown(node.Content)

	switch node.NodeType {
	case nodeTypeHyperlink:
		uri := node.getHyperlinkURI()
		if uri == "" {
			warnings = append(warnings, "hyperlink node missing uri rendered as plain text")
			return label, warnings
		}
		return "[" + label + "](" + escapeMarkdownURL(uri) + ")", warnings
	case nodeTypeEntryHyperlink, nodeTypeAssetHyperlink:
		_, id, ok := node.getHyperlinkTarget()
		if !ok {
			warnings = append(warnings, fmt.Sprintf("%s node missing target rendered as plain text", node.NodeType))
			return label, warnings
		}
		scheme := "entry"
		if node.NodeType == nodeTypeAssetHyperlink {
			scheme = "asset"
		}
		return "[" + label + "](" + scheme + ":" + escapeMarkdownURL(id) + ")", warnings
	default:
		return label, warnings
	}
}

func escapeMarkdownText(text string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`*`, `\*`,
		`_`, `\_`,
		`[`, `\[`,
		`]`, `\]`,
		`(`, `\(`,
		`)`, `\)`,
		`#`, `\#`,
	)
	return replacer.Replace(text)
}

// escapeMarkdownURL escapes only the characters that would break the (...) of a
// Markdown link target; reversed by unescapeMarkdownText.
func escapeMarkdownURL(url string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`(`, `\(`,
		`)`, `\)`,
	)
	return replacer.Replace(url)
}

// unescapeMarkdownText reverses escapeMarkdownText, stripping the backslash from
// escaped Markdown metacharacters so text nodes round-trip to their original value.
func unescapeMarkdownText(text string) string {
	if !strings.Contains(text, `\`) {
		return text
	}
	var builder strings.Builder
	builder.Grow(len(text))
	for i := 0; i < len(text); i++ {
		if text[i] == '\\' && i+1 < len(text) && isMarkdownEscapable(text[i+1]) {
			builder.WriteByte(text[i+1])
			i++
			continue
		}
		builder.WriteByte(text[i])
	}
	return builder.String()
}

func isMarkdownEscapable(c byte) bool {
	switch c {
	case '\\', '*', '_', '[', ']', '(', ')', '#':
		return true
	default:
		return false
	}
}
