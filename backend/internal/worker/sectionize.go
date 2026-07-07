package worker

import (
	"regexp"
	"strings"
)

// Section 是 Markdown 文档中一个标题路径下的正文块。
// Path 形如 "部署指南 > 环境变量"；文档开头无标题的前言 Path 为 ""。
type Section struct {
	Path string
	Body string
}

var headingRe = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

// backtickRun 返回 trimmed 行开头连续反引号的个数。
func backtickRun(trimmed string) int {
	n := 0
	for n < len(trimmed) && trimmed[n] == '`' {
		n++
	}
	return n
}

// SectionizeMarkdown 按标题层级把带 # 标记的 Markdown 文本切成 Section 列表。
// 代码围栏内的 # 行不视为标题；正文为空的标题只贡献路径，不产生 Section。
//
// 围栏跟踪是长度感知的：开栏记录起始反引号数，只有整行为反引号且
// 长度 ≥ 开栏长度的行才闭栏，栏内更短的 ``` 行不会误闭。
// parser 输出只会产生反引号围栏（波浪线围栏已被转换），无需处理 ~~~。
func SectionizeMarkdown(text string) []Section {
	type level struct {
		depth int
		title string
	}
	var stack []level
	var out []Section
	var body []string
	fenceLen := 0 // 0 表示不在围栏内；>0 表示开栏反引号数

	path := func() string {
		titles := make([]string, 0, len(stack))
		for _, l := range stack {
			titles = append(titles, l.title)
		}
		return strings.Join(titles, " > ")
	}
	flush := func() {
		content := strings.TrimSpace(strings.Join(body, "\n"))
		if content != "" {
			out = append(out, Section{Path: path(), Body: content})
		}
		body = nil
	}

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		run := backtickRun(trimmed)

		if fenceLen > 0 {
			// 栏内：只有 ≥ 开栏长度的纯反引号行闭栏，其余照收进正文。
			if run >= fenceLen && strings.Trim(trimmed, "`") == "" {
				fenceLen = 0
			}
			body = append(body, line)
			continue
		}
		if run >= 3 {
			fenceLen = run
			body = append(body, line)
			continue
		}
		if m := headingRe.FindStringSubmatch(trimmed); m != nil {
			flush()
			depth := len(m[1])
			for len(stack) > 0 && stack[len(stack)-1].depth >= depth {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, level{depth: depth, title: strings.TrimSpace(m[2])})
			continue
		}
		body = append(body, line)
	}
	flush()
	return out
}
