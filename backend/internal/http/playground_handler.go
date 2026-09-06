package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/zenith-wang/it-wiki/backend/internal/playground"
)

type PlaygroundHandler struct{ service *playground.Service }

func NewPlaygroundHandler(service *playground.Service) *PlaygroundHandler {
	return &PlaygroundHandler{service}
}
func playgroundError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, playground.ErrNotConfigured):
		WriteError(w, r, NewAPIError(404, "model_not_configured", "请先配置模型接口"))
	case errors.Is(err, playground.ErrInvalid):
		WriteError(w, r, NewAPIError(400, "invalid_model_request", "请检查接口地址、密钥、模型和对话参数"))
	case errors.Is(err, playground.ErrConflict):
		WriteError(w, r, NewAPIError(409, "configuration_conflict", "配置已发生变化，请重新加载后保存"))
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
			_ = send("error", map[string]string{"message": "模型响应中断，请重试"})
		} else {
			playgroundError(w, r, err)
		}
		return
	}
	_ = send("done", struct{}{})
}
