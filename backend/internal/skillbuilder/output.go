package skillbuilder

import (
	"errors"
	"strings"
)

// OutputError contains only platform-authored diagnostics. Raw model output and
// decoder errors can contain user material and must not reach responses or logs.
type OutputError struct {
	Reason  string
	Message string
}

func (e *OutputError) Error() string { return e.Message }
func (e *OutputError) Unwrap() error { return ErrOutput }

func outputError(reason, message string) error {
	return &OutputError{Reason: reason, Message: message}
}

func parseReply(raw string) (Reply, error) {
	var reply Reply
	if decode([]byte(raw), &reply) != nil || strings.TrimSpace(reply.Message) == "" || len(reply.Message) > 12000 {
		return Reply{}, outputError("invalid_output", "模型返回的内容不是有效的草案 JSON，原草案已保留；请重试或换一个模型")
	}
	if reply.Draft != nil {
		if _, err := Files(*reply.Draft); err != nil {
			message := "生成的草案未通过文件校验，请重试"
			var validation *InvalidError
			if errors.As(err, &validation) {
				message = "生成的草案未通过校验：" + validation.Message
			}
			return Reply{}, outputError("invalid_draft", message)
		}
	}
	return reply, nil
}
