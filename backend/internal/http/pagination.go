package http

import (
	"net/http"
	"strconv"
)

type Pagination struct {
	Limit  int
	Offset int
}

const (
	defaultLimit = 20
	maxLimit     = 100
)

func ParsePagination(r *http.Request) (Pagination, error) {
	p := Pagination{Limit: defaultLimit, Offset: 0}

	if s := r.URL.Query().Get("limit"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 || v > maxLimit {
			return p, NewAPIError(http.StatusBadRequest, CodeValidationFailed,
				"limit must be int in [1,"+strconv.Itoa(maxLimit)+"]")
		}
		p.Limit = v
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 0 {
			return p, NewAPIError(http.StatusBadRequest, CodeValidationFailed,
				"offset must be non-negative int")
		}
		p.Offset = v
	}
	return p, nil
}

func WriteListResponse(w http.ResponseWriter, total int, items any) {
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	WriteJSON(w, http.StatusOK, items)
}
