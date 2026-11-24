// Package api is the HTTP transport. Handlers parse requests into platform
// shapes, call core services from bootstrap, and serialize the results.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/atb00ker/splitspends/backend/internal/core/shared/errs"
	"github.com/atb00ker/splitspends/backend/internal/middleware"
	"github.com/google/uuid"
)

const maxBody = 1 << 20

type handler func(w http.ResponseWriter, r *http.Request) error

func (a *API) wrap(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			middleware.WriteError(w, r, a.log, err)
		}
	}
}

func decode[T any](r *http.Request) (T, error) {
	return decodeLimit[T](r, maxBody)
}

func decodeLimit[T any](r *http.Request, limit int64) (T, error) {
	var v T
	dec := json.NewDecoder(io.LimitReader(r.Body, limit))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		if errors.Is(err, io.EOF) {
			return v, errs.Validation("", "The request body is empty.")
		}
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			return v, errs.Validation(typeErr.Field, "Field "+typeErr.Field+" has the wrong type.")
		}
		return v, errs.Validation("", "The request body is not valid JSON.")
	}
	return v, nil
}

func details(w http.ResponseWriter, status int, v any) error {
	middleware.WriteJSON(w, status, map[string]any{"data": v})
	return nil
}

func list[T any](w http.ResponseWriter, items []T, total int) error {
	if items == nil {
		items = []T{}
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
	return nil
}

func noContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func pathID(r *http.Request, name, entity string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return uuid.Nil, errs.NotFound(entity)
	}
	return id, nil
}

func optionalID(raw *string, field string) (*uuid.UUID, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return nil, errs.Validation(field, "Invalid id.")
	}
	return &id, nil
}

func queryID(r *http.Request, name string) (*uuid.UUID, error) {
	v := r.URL.Query().Get(name)
	return optionalID(&v, name)
}

func queryDate(r *http.Request, name string) (*time.Time, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.DateOnly, v)
	if err != nil {
		return nil, errs.Validation(name, "Use a YYYY-MM-DD date.")
	}
	return &t, nil
}

func page(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func me(r *http.Request) uuid.UUID {
	if p := middleware.Principal(r.Context()); p != nil {
		return p.UserID
	}
	return uuid.Nil
}
