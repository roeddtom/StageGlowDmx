package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"stageglowdmx/internal/config"
)

const MaxConfigBytes = 2 << 20

func New(store *config.Store, frontend http.Handler) http.Handler {
	if frontend == nil {
		frontend = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		if !safeHost(r.Host) {
			fail(w, http.StatusForbidden, "Host must be a local hostname or IP address")
			return
		}
		if !sameOrigin(r) {
			fail(w, http.StatusForbidden, "cross-origin requests are not allowed")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			site := r.Header.Get("Sec-Fetch-Site")
			if site != "" && site != "same-origin" && site != "none" {
				fail(w, http.StatusForbidden, "cross-origin configuration changes are not allowed")
				return
			}
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api" {
			frontend.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		switch r.URL.Path {
		case "/api/health":
			if requireMethod(w, r, http.MethodGet) {
				writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			}
		case "/api/state":
			if requireMethod(w, r, http.MethodGet) {
				writeJSON(w, http.StatusOK, struct {
					Blackout      bool   `json:"blackout"`
					OutputEnabled bool   `json:"outputEnabled"`
					EngineStatus  string `json:"engineStatus"`
				}{true, false, "not-implemented"})
			}
		case "/api/config", "/api/config/export", "/api/config/import":
			methods := []string{http.MethodGet}
			if r.URL.Path == "/api/config/import" {
				methods = []string{http.MethodPost}
			} else if r.URL.Path == "/api/config" {
				methods = []string{http.MethodGet, http.MethodPut}
			}
			if !requireMethod(w, r, methods...) {
				return
			}
			if r.Method == http.MethodGet {
				if r.URL.Path == "/api/config/export" {
					w.Header().Set("Content-Disposition", `attachment; filename="stageglow-config.json"`)
				}
				writeJSON(w, http.StatusOK, store.Get())
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, MaxConfigBytes)
			defer r.Body.Close()
			c, err := config.Decode(r.Body)
			if err != nil {
				var sizeErr *http.MaxBytesError
				status := http.StatusBadRequest
				if errors.As(err, &sizeErr) {
					status = http.StatusRequestEntityTooLarge
				}
				fail(w, status, err.Error())
				return
			}
			if err := store.Save(c); err != nil {
				fail(w, http.StatusInternalServerError, fmt.Sprintf("configuration not saved: %v", err))
				return
			}
			writeJSON(w, http.StatusOK, c)
		default:
			fail(w, http.StatusNotFound, "API endpoint not found")
		}
	})
}

func requireMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, method := range methods {
		if r.Method == method {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(methods, ", "))
	fail(w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func safeHost(authority string) bool {
	if authority == "" || strings.ContainsAny(authority, "/\\@?# \t\r\n") {
		return false
	}
	u, err := url.Parse("http://" + authority)
	if err != nil || u.Host != authority || u.User != nil {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	host := strings.ToLower(u.Hostname())
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsUnspecified() && !ip.IsMulticast() && !ip.Equal(net.IPv4bcast)
	}
	if strings.HasSuffix(host, ".") {
		host = strings.TrimSuffix(host, ".")
	}
	if strings.Contains(host, ".") && !strings.HasSuffix(host, ".local") {
		return false
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
				return false
			}
		}
	}
	return true
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return u.Scheme == scheme && strings.EqualFold(u.Host, r.Host)
}
