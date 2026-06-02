// Package migrations 包含 goose 迁移：SQL 文件用 embed.FS 加载，
// Go 文件通过 init() 注册到 goose registry。
//
// 启动顺序：cmd/server/main.go 调用 goose.UpContext(ctx, db, ".") 时
// 会同时读取 EmbedMigrations 中的 .sql 和已注册的 Go migrations。
package migrations

import "embed"

//go:embed *.sql
var EmbedMigrations embed.FS
