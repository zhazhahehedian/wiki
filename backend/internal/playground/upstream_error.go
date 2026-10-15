package playground

import (
	"encoding/json"
	"io"
	"net/http"
)

// Provider messages may contain user data or credentials. Keep only a small
// allowlist of machine-readable categories; never retain the original body.
func readUpstreamError(response *http.Response) *UpstreamHTTPError {
	err := &UpstreamHTTPError{StatusCode: response.StatusCode}
	const limit = 16 * 1024
	body, readErr := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if readErr != nil || len(body) > limit {
		return err
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return err
	}
	for _, code := range []string{payload.Error.Code, payload.Error.Type} {
		switch code {
		case "context_length_exceeded", "context_window_exceeded", "prompt_too_long":
			err.Reason = "context_limit"
		case "model_not_found", "model_not_available", "no_available_channel", "no_available_channels":
			err.Reason = "model_unavailable"
		case "overloaded_error", "overloaded", "server_overloaded", "engine_overloaded":
			err.Reason = "overloaded"
		case "rate_limit_exceeded", "rate_limit_error":
			err.Reason = "rate_limited"
		case "insufficient_quota", "insufficient_balance":
			err.Reason = "quota_exhausted"
		case "unsupported_parameter", "invalid_parameter":
			err.Reason = "invalid_parameters"
		case "invalid_request_error", "invalid_request":
			err.Reason = "invalid_request"
		}
		if err.Reason != "" {
			break
		}
	}
	return err
}
