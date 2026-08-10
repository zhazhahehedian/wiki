package feishu

import (
	"fmt"
	"regexp"
	"strings"
)

type MarkdownNormalizer struct{}

type markdownTableCell struct {
	text         string
	safeMarkdown string
}

var (
	headingLinePattern     = regexp.MustCompile(`^(#+)\s+(.*)$`)
	orderedListLinePattern = regexp.MustCompile(`^(\s*\d+)([.)])(\s+)(.*)$`)
	tildeFenceLinePattern  = regexp.MustCompile(`^(\s*)(~{3,})(.*)$`)
	setextHeadingPattern   = regexp.MustCompile(`^(\s*)(=+)(\s*)$`)
)

func NewMarkdownNormalizer() MarkdownNormalizer { return MarkdownNormalizer{} }

func (MarkdownNormalizer) EscapeText(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`,
		"<", `\<`, ">", `\>`, "#", `\#`, "+", `\+`, "-", `\-`, "|", `\|`, "!", `\!`,
	)
	return replacer.Replace(strings.ReplaceAll(value, "\r\n", "\n"))
}

func (n MarkdownNormalizer) Normalize(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(line) == "" {
			if len(result) > 0 && !blank {
				result = append(result, "")
				blank = true
			}
			continue
		}
		blank = false
		if match := headingLinePattern.FindStringSubmatch(line); match != nil {
			level := len(match[1])
			if level > 6 {
				level = 6
			}
			line = strings.Repeat("#", level) + " " + n.EscapeText(match[2])
		} else {
			line = n.Paragraph(line)
		}
		result = append(result, line)
	}
	for len(result) > 0 && result[len(result)-1] == "" {
		result = result[:len(result)-1]
	}
	if len(result) == 0 {
		return ""
	}
	return strings.Join(result, "\n") + "\n"
}

func (n MarkdownNormalizer) Heading(level int, text string) string {
	if level < 1 {
		level = 1
	}
	if level > 6 {
		level = 6
	}
	return strings.Repeat("#", level) + " " + n.headingText(text)
}

func (n MarkdownNormalizer) headingText(text string) string {
	return n.EscapeText(strings.TrimSpace(text))
}

func (n MarkdownNormalizer) Paragraph(text string) string {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n")), "\n")
	for i, line := range lines {
		switch {
		case orderedListLinePattern.MatchString(line):
			match := orderedListLinePattern.FindStringSubmatch(line)
			lines[i] = n.EscapeText(match[1]) + `\` + match[2] + n.EscapeText(match[3]+match[4])
		case tildeFenceLinePattern.MatchString(line):
			match := tildeFenceLinePattern.FindStringSubmatch(line)
			lines[i] = n.EscapeText(match[1]) + strings.Repeat(`\~`, len(match[2])) + n.EscapeText(match[3])
		case setextHeadingPattern.MatchString(line):
			match := setextHeadingPattern.FindStringSubmatch(line)
			lines[i] = n.EscapeText(match[1]) + strings.Repeat(`\=`, len(match[2])) + n.EscapeText(match[3])
		default:
			lines[i] = n.EscapeText(line)
		}
	}
	return strings.Join(lines, "\n")
}

func (n MarkdownNormalizer) Table(rows [][]string) string {
	typedRows := make([][]markdownTableCell, len(rows))
	for row := range rows {
		typedRows[row] = make([]markdownTableCell, len(rows[row]))
		for column, value := range rows[row] {
			typedRows[row][column] = markdownTableCell{text: value}
		}
	}
	return n.tableCells(typedRows)
}

func (n MarkdownNormalizer) tableCells(rows [][]markdownTableCell) string {
	if len(rows) == 0 {
		return ""
	}
	columns := 0
	for _, row := range rows {
		if len(row) > columns {
			columns = len(row)
		}
	}
	if columns == 0 {
		return ""
	}
	formatRow := func(row []markdownTableCell) string {
		cells := make([]string, columns)
		for i := range cells {
			if i < len(row) {
				if row[i].safeMarkdown != "" {
					cells[i] = strings.Join(strings.Fields(row[i].safeMarkdown), " ")
				} else {
					cells[i] = n.EscapeText(strings.Join(strings.Fields(row[i].text), " "))
				}
			}
		}
		return "| " + strings.Join(cells, " | ") + " |"
	}
	lines := []string{formatRow(rows[0])}
	delimiter := make([]string, columns)
	for i := range delimiter {
		delimiter[i] = "---"
	}
	lines = append(lines, "| "+strings.Join(delimiter, " | ")+" |")
	for _, row := range rows[1:] {
		lines = append(lines, formatRow(row))
	}
	return strings.Join(lines, "\n")
}

func (n MarkdownNormalizer) combineTableCells(parts []markdownTableCell) markdownTableCell {
	if len(parts) == 1 {
		return parts[0]
	}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.safeMarkdown != "" {
			values = append(values, part.safeMarkdown)
		} else if text := strings.Join(strings.Fields(part.text), " "); text != "" {
			values = append(values, n.EscapeText(text))
		}
	}
	return markdownTableCell{safeMarkdown: strings.Join(values, " ")}
}

func (n MarkdownNormalizer) UnsupportedImage(description, sourceURL string) string {
	return "> " + n.UnsupportedInlineImage(description, sourceURL)
}

func (n MarkdownNormalizer) UnsupportedInlineImage(description, sourceURL string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		description = "unsupported image"
	}
	return fmt.Sprintf("[Image: %s](%s)", n.EscapeText(description), sourceURL)
}

func (MarkdownNormalizer) Finalize(parts []string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "\n \t")
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	return strings.Join(filtered, "\n\n") + "\n"
}
