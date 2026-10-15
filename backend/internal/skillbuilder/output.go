package skillbuilder

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// OutputError contains only platform-authored diagnostics. Raw model output and
// decoder errors can contain user material and must not reach responses or logs.
type OutputError struct {
	Reason        string
	Message       string
	ResponseBytes int
}

func (e *OutputError) Error() string { return e.Message }
func (e *OutputError) Unwrap() error { return ErrOutput }

func outputError(reason, message string, size int) error {
	return &OutputError{Reason: reason, Message: message, ResponseBytes: size}
}

func parseReply(raw string) (Reply, error) {
	size := len(raw)
	if strings.TrimSpace(raw) == "" {
		return Reply{}, outputError("empty_response", "模型未返回可用的草案内容，请重试或换一个模型", size)
	}
	var reply Reply
	if err := decode([]byte(raw), &reply); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return Reply{}, outputError("incomplete_json", "模型返回的草案在 JSON 结束前停止，内容不完整；尚不能确认是否达到输出上限，请精简需求后重试", size)
		}
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			return Reply{}, outputError("invalid_json", "模型返回的内容不是有效的 JSON 草案，请重试或换一个模型", size)
		}
		return Reply{}, outputError("invalid_response", "模型返回的草案结构或字段不符合要求，请重试或换一个模型", size)
	}
	if strings.TrimSpace(reply.Message) == "" || len(reply.Message) > 12000 {
		return Reply{}, outputError("invalid_response", "模型返回的结果说明为空或过长，请重试或换一个模型", size)
	}
	if reply.Draft != nil {
		if _, err := Files(*reply.Draft); err != nil {
			message := "生成的草案未通过文件校验，请重试"
			var validation *InvalidError
			if errors.As(err, &validation) {
				message = "生成的草案未通过校验：" + validation.Message
			}
			return Reply{}, outputError("invalid_draft", message, size)
		}
	}
	return reply, nil
}
