package feishu

import (
	"fmt"
	"regexp"
	"strings"
)

type MarkdownNormalizer struct{}

var headingLinePattern = regexp.MustCompile(`^(#+)\s+(.*)$`)

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
			line = n.EscapeText(line)
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
	return strings.Repeat("#", level) + " " + n.EscapeText(strings.TrimSpace(text))
}

func (n MarkdownNormalizer) Paragraph(text string) string {
	return n.EscapeText(strings.TrimSpace(text))
}

func (n MarkdownNormalizer) Table(rows [][]string) string {
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
	formatRow := func(row []string) string {
		cells := make([]string, columns)
		for i := range cells {
			if i < len(row) {
				cells[i] = n.EscapeText(strings.Join(strings.Fields(row[i]), " "))
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

func (n MarkdownNormalizer) UnsupportedImage(description, sourceURL string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		description = "unsupported image"
	}
	return fmt.Sprintf("> [Image: %s](%s)", n.EscapeText(description), sourceURL)
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
