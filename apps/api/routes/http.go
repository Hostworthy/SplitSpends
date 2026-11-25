// Package routes is the HTTP transport. Handlers parse requests into platform
// shapes, call core services from bootstrap, and serialize the results.
package routes

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/middleware"
	"github.com/Hostworthy/SplitSpends/packages/platform"
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

// decode reads a JSON body of at most maxBody bytes into T. Fields T does
// not know are ignored, so an older server keeps accepting newer clients.
func decode[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	return decodeLimit[T](w, r, maxBody)
}

func decodeLimit[T any](w http.ResponseWriter, r *http.Request, limit int64) (T, error) {
	v, found, err := read[T](w, r, limit)
	if err == nil && !found {
		err = errs.Validation("", "The request body is empty.")
	}
	return v, err
}

// decodeOptional is decode for bodies a client may leave out: an empty body
// is the zero value of T.
func decodeOptional[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	v, _, err := read[T](w, r, maxBody)
	return v, err
}

// read decodes exactly one JSON value of at most limit bytes; found is false
// when the body is empty.
func read[T any](w http.ResponseWriter, r *http.Request, limit int64) (v T, found bool, err error) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		if errors.Is(err, io.EOF) {
			return v, false, nil
		}
		return v, false, invalid(err, limit)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if tooLarge(err) {
			return v, false, invalid(err, limit)
		}
		return v, false, errs.Validation("", "The request body must hold only one JSON value.")
	}
	return v, true, nil
}

func invalid(err error, limit int64) error {
	if tooLarge(err) {
		return errs.Validation("", "The request body is too large. Send at most "+strconv.FormatInt(limit>>20, 10)+" MB.")
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return errs.Validation(typeErr.Field, "Field "+typeErr.Field+" has the wrong type.")
	}
	return errs.Validation("", "The request body is not valid JSON.")
}

func tooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

func details[T any](w http.ResponseWriter, status int, v T) error {
	middleware.WriteJSON(w, status, platform.Details[T]{Data: v})
	return nil
}

func list[T any](w http.ResponseWriter, items []T, total int) error {
	if items == nil {
		items = []T{}
	}
	middleware.WriteJSON(w, http.StatusOK, platform.List[T]{Items: items, Total: total})
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

// requiredID parses an id sent in field of a request body.
func requiredID(raw, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errs.Validation(field, "Invalid id.")
	}
	return id, nil
}

func optionalID(raw *string, field string) (*uuid.UUID, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	id, err := requiredID(*raw, field)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// nest puts the field of a validation error under path, so an error in one
// entry of a list names that entry, as in expenses[3].total.
func nest(err error, path string) error {
	de, ok := errs.As(err)
	if !ok {
		return err
	}
	field := path
	if de.Field != "" {
		field += "." + de.Field
	}
	return &errs.Error{Kind: de.Kind, Field: field, Message: de.Message, Cause: de.Cause}
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

// window is page for lists that returned everything before they took a
// limit: without one they still do, so clients that send none keep working.
func window(r *http.Request) (limit, offset int) {
	limit, offset = page(r)
	if r.URL.Query().Get("limit") == "" {
		limit = 0
	}
	return limit, offset
}

// paged writes the page of items that window asks for, with the total.
func paged[T any](w http.ResponseWriter, r *http.Request, items []T) error {
	limit, offset := window(r)
	total := len(items)
	items = items[min(offset, total):]
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	return list(w, items, total)
}

func me(r *http.Request) uuid.UUID {
	if p := middleware.Principal(r.Context()); p != nil {
		return p.UserID
	}
	return uuid.Nil
}

// session is the caller's session or API token.
func session(r *http.Request) uuid.UUID {
	if p := middleware.Principal(r.Context()); p != nil {
		return p.SessionID
	}
	return uuid.Nil
}
