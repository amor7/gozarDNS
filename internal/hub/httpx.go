package hub

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/amor7/gozar/internal/web"
)

// apiError carries a stable code the web apps translate, plus an English
// message for logs and curl users.
type apiError struct {
	Status  int    `json:"-"`
	Code    string `json:"error"`
	Message string `json:"message,omitempty"`
}

func (e *apiError) Error() string { return e.Code + ": " + e.Message }

func fail(status int, code, msg string) error { return &apiError{status, code, msg} }

var (
	errBadRequest   = fail(400, "bad_request", "the request could not be read")
	errUnauthorized = fail(401, "unauthorized", "sign in first")
	errNotFound     = fail(404, "not_found", "no such thing")
)

type handler func(w http.ResponseWriter, r *http.Request) error

func (s *Hub) wrap(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		web.SecurityHeaders(w)
		w.Header().Set("Cache-Control", "no-store")
		if err := h(w, r); err != nil {
			var ae *apiError
			if !errors.As(err, &ae) {
				s.log.Error("request failed", "path", r.URL.Path, "err", err)
				ae = &apiError{500, "internal", "something went wrong on the server"}
			}
			writeJSON(w, ae.Status, ae)
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, v any) error {
	if v == nil {
		v = map[string]bool{"ok": true}
	}
	writeJSON(w, http.StatusOK, v)
	return nil
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		return errBadRequest
	}
	return nil
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, errNotFound
	}
	return id, nil
}

func queryInt(r *http.Request, name string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil {
		return v
	}
	return def
}

// requireAJAX blocks cross-site form posts: a custom header cannot be set by
// a plain HTML form, and setting it from script on another origin triggers
// a CORS preflight this server never approves.
func requireAJAX(r *http.Request) error {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return nil
	}
	if r.Header.Get("X-Requested-With") != "gozar" {
		return fail(403, "forbidden", "missing request header")
	}
	return nil
}

func remoteIP(r *http.Request) string {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return ap.Addr().Unmap().String()
}

func trimmed(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > n {
		s = string([]rune(s)[:n])
	}
	return s
}
