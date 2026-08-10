package ports

import (
	"context"
	"encoding/json"
)

// Tool 是 Agent 可调用的工具。Invoke 返回的字符串直接作为
// role=tool 消息内容回喂 LLM；实现必须尊重 ctx 取消。
type Tool interface {
	Name() string
	Description() string
	ParametersSchema() json.RawMessage
	Invoke(ctx context.Context, argsJSON string) (string, error)
}
