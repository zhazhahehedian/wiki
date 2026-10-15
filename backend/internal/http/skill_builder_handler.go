package http

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/zenith-wang/it-wiki/backend/internal/playground"
	"github.com/zenith-wang/it-wiki/backend/internal/skillbuilder"
)

type SkillBuilderHandler struct{ service *skillbuilder.Service }

func NewSkillBuilderHandler(s *skillbuilder.Service) *SkillBuilderHandler {
	return &SkillBuilderHandler{s}
}
func decodeSkillBuilder(w http.ResponseWriter, r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(value) != nil {
		return skillbuilder.ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return skillbuilder.ErrInvalid
	}
	return nil
}
func skillBuilderError(w http.ResponseWriter, r *http.Request, e error) {
	if r.Context().Err() != nil {
		return
	}
	var validation *skillbuilder.InvalidError
	var output *skillbuilder.OutputError
	switch {
	case errors.As(e, &validation):
		WriteError(w, r, NewAPIError(400, "invalid_skill_draft", validation.Message))
	case errors.Is(e, skillbuilder.ErrInvalid):
		WriteError(w, r, NewAPIError(400, "invalid_skill_builder_request", "请检查需求、模型、对话轮数和素材大小"))
	case errors.As(e, &output):
		WriteError(w, r, WithDetails(NewAPIError(502, "invalid_skill_generation", output.Message), map[string]any{"reason": output.Reason}))
	case errors.Is(e, playground.ErrNotConfigured), errors.Is(e, playground.ErrInvalid), errors.Is(e, playground.ErrUpstream):
		playgroundError(w, r, e)
	default:
		registryHTTPError(w, r, e)
	}
}
func (h *SkillBuilderHandler) Turn(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	var input skillbuilder.Request
	if e := decodeSkillBuilder(w, r, &input); e != nil {
		skillBuilderError(w, r, e)
		return
	}
	started := time.Now()
	reply, e := h.service.Turn(r.Context(), user, input)
	// Never log conversation text, material, generated content or credentials.
	log.Printf("skill generation finished request_id=%s model=%q success=%t duration=%s", middleware.GetReqID(r.Context()), input.Model, e == nil, time.Since(started))
	if e != nil {
		skillBuilderError(w, r, e)
		return
	}
	WriteJSON(w, 200, reply)
}
func (h *SkillBuilderHandler) Save(w http.ResponseWriter, r *http.Request) {
	user, ok := registryUser(w, r)
	if !ok {
		return
	}
	var input skillbuilder.SaveRequest
	if e := decodeSkillBuilder(w, r, &input); e != nil {
		skillBuilderError(w, r, e)
		return
	}
	detail, e := h.service.Save(r.Context(), user, input)
	if e != nil {
		skillBuilderError(w, r, e)
		return
	}
	WriteJSON(w, 201, detail)
}
