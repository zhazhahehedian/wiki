package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/zenith-wang/it-wiki/backend/internal/playground"
)

type PlaygroundHandler struct{ service *playground.Service }

func NewPlaygroundHandler(service *playground.Service) *PlaygroundHandler {
	return &PlaygroundHandler{service}
}
func playgroundError(w http.ResponseWriter, r *http.Request, err error) {
	var upstreamHTTP *playground.UpstreamHTTPError
	switch {
	case errors.Is(err, playground.ErrNotConfigured):
		WriteError(w, r, NewAPIError(404, "model_not_configured", "请先配置模型接口"))
	case errors.Is(err, playground.ErrInvalid):
		WriteError(w, r, NewAPIError(400, "invalid_model_request", "请检查接口地址、密钥、模型和对话参数"))
	case errors.Is(err, playground.ErrConflict):
		WriteError(w, r, NewAPIError(409, "configuration_conflict", "配置已发生变化，请重新加载后保存"))
	case errors.Is(err, playground.ErrTimeout):
		WriteError(w, r, NewAPIError(504, "model_response_timeout", "等待模型响应超时，请缩小本次生成范围或稍后重试"))
	case errors.Is(err, playground.ErrInterrupted):
		WriteError(w, r, NewAPIError(502, "model_stream_interrupted", "模型响应在完成前中断，请重试；若反复出现，请检查中转服务"))
	case errors.Is(err, playground.ErrOutputLimit):
		WriteError(w, r, NewAPIError(502, "model_output_limit", "模型达到本轮输出上限，内容被截断；请精简需求或调整最大输出长度"))
	case errors.As(err, &upstreamHTTP):
		message := fmt.Sprintf("模型服务返回 HTTP %d，请检查模型、额度或中转服务状态", upstreamHTTP.StatusCode)
		if upstreamHTTP.StatusCode == 400 || upstreamHTTP.StatusCode == 422 {
			message = fmt.Sprintf("模型服务不接受本次请求参数（HTTP %d），请检查最大输出 Tokens、采样参数和模型名称是否受支持", upstreamHTTP.StatusCode)
		}
		if upstreamHTTP.StatusCode == 503 {
			message = "模型或中转服务返回 HTTP 503，未返回正常生成响应；请稍后重试或检查该模型的可用线路"
		}
		switch upstreamHTTP.Reason {
		case "context_limit":
			message = "上游报告上下文超限：对话、参考说明和输出预算合计过大，请缩短输入或调低最大输出 Tokens"
		case "model_unavailable":
			message = "上游报告当前模型或可用线路不可用，请检查中转服务中的模型配置或切换模型"
		case "overloaded":
			message = "上游报告模型服务负载过高，请稍后重试或切换模型"
		case "rate_limited":
			message = "上游报告请求限流，请稍后重试或检查模型的速率额度"
		case "quota_exhausted":
			message = "上游报告可用额度不足，请检查模型服务的账户额度"
		case "invalid_parameters":
			message = "上游报告请求参数不受支持，请检查最大输出 Tokens、采样参数和模型名称"
		case "invalid_request":
			message = "上游未接受本次请求，请核对该接口支持的输入格式与参数"
		}
		details := map[string]any{"upstream_status": upstreamHTTP.StatusCode}
		if upstreamHTTP.Reason != "" {
			details["upstream_reason"] = upstreamHTTP.Reason
		}
		log.Printf("model upstream rejected request_id=%s upstream_status=%d reason=%q", middleware.GetReqID(r.Context()), upstreamHTTP.StatusCode, upstreamHTTP.Reason)
		WriteError(w, r, WithDetails(NewAPIError(502, "model_upstream_rejected", message), details))
	case errors.Is(err, playground.ErrUpstream):
		WriteError(w, r, NewAPIError(502, "model_service_failed", "模型服务不可用，请检查接口配置或稍后重试"))
	default:
		WriteError(w, r, err)
	}
}
func decodePlayground(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 300*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return playground.ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return playground.ErrInvalid
	}
	return nil
}
func (h *PlaygroundHandler) Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	u, _ := CurrentUserFromContext(r.Context())
	c, e := h.service.Get(r.Context(), u.ID)
	if errors.Is(e, playground.ErrNotConfigured) {
		WriteJSON(w, 200, nil)
		return
	}
	if e != nil {
		playgroundError(w, r, e)
		return
	}
	WriteJSON(w, 200, c)
}
func (h *PlaygroundHandler) Save(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var data playground.SaveRequest
	if e := decodePlayground(w, r, &data); e != nil {
		playgroundError(w, r, e)
		return
	}
	u, _ := CurrentUserFromContext(r.Context())
	c, e := h.service.Save(r.Context(), u.ID, data)
	if e != nil {
		playgroundError(w, r, e)
		return
	}
	WriteJSON(w, 200, c)
}
func (h *PlaygroundHandler) Delete(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUserFromContext(r.Context())
	if e := h.service.Delete(r.Context(), u.ID); e != nil {
		playgroundError(w, r, e)
		return
	}
	w.WriteHeader(204)
}
func (h *PlaygroundHandler) Models(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input playground.ModelsRequest
	if err := decodePlayground(w, r, &input); err != nil {
		playgroundError(w, r, err)
		return
	}
	u, _ := CurrentUserFromContext(r.Context())
	models, err := h.service.DiscoverModels(r.Context(), u.ID, input)
	if err != nil {
		playgroundError(w, r, err)
		return
	}
	WriteJSON(w, 200, map[string]any{"models": models})
}
func (h *PlaygroundHandler) Stream(w http.ResponseWriter, r *http.Request) {
	var data playground.ChatRequest
	if e := decodePlayground(w, r, &data); e != nil {
		playgroundError(w, r, e)
		return
	}
	u, _ := CurrentUserFromContext(r.Context())
	started := false
	send := func(event string, value any) error {
		if !started {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Accel-Buffering", "no")
			started = true
		}
		b, _ := json.Marshal(value)
		if _, e := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); e != nil {
			return e
		}
		return http.NewResponseController(w).Flush()
	}
	err := h.service.Stream(r.Context(), u.ID, data, func(text string) error { return send("delta", map[string]string{"text": text}) })
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		if started {
			message := "模型响应中断，请重试"
			if errors.Is(err, playground.ErrOutputLimit) {
				message = "模型达到本轮输出上限，内容被截断；请精简需求或调整最大输出长度"
			}
			_ = send("error", map[string]string{"message": message})
		} else {
			playgroundError(w, r, err)
		}
		return
	}
	_ = send("done", struct{}{})
}
