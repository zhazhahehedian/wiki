package worker

import (
	"path/filepath"
	"strings"
)

// effectiveMimeType 修正浏览器上传时给出的泛化 MIME:
// .md 文件常被标成 application/octet-stream, 导致 Markdown parser 永远不被选中,
// 结构感知切片(sectionize)随之失效。存量文档 reingest 时也走这里, 无需迁移数据。
func effectiveMimeType(mime, title string) string {
	if mime != "" && mime != "application/octet-stream" {
		return mime
	}
	switch strings.ToLower(filepath.Ext(title)) {
	case ".md", ".markdown":
		return "text/markdown"
	}
	return mime
}
