package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"stageglowdmx/internal/config"
	"stageglowdmx/internal/engine"
	"stageglowdmx/internal/output"
)

const MaxConfigBytes = 2 << 20

func New(store *config.Store, frontend http.Handler) http.Handler {
	return NewWithEngine(store, frontend, engine.New(store.Get(), nil, time.Now().UnixNano()))
}

func NewWithEngine(store *config.Store, frontend http.Handler, lighting *engine.Engine) http.Handler {
	return NewWithOutput(store, frontend, output.New(store.Get(), lighting, nil))
}

func NewWithOutput(store *config.Store, frontend http.Handler, controller *output.Controller) http.Handler {
	var commands sync.Mutex
	state := func() any {
		engineState, status := controller.State()
		return struct {
			engine.State
			Output output.Status `json:"output"`
		}{engineState, status}
	}
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
		commands.Lock()
		defer commands.Unlock()
		switch r.URL.Path {
		case "/api/health":
			if requireMethod(w, r, http.MethodGet) {
				writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			}
		case "/api/state":
			if requireMethod(w, r, http.MethodGet) {
				writeJSON(w, http.StatusOK, state())
			}
		case "/api/output/ports":
			if requireMethod(w, r, http.MethodGet) {
				ports, err := output.Ports()
				if err != nil {
					fail(w, http.StatusInternalServerError, err.Error())
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"ports": ports})
			}
		case "/api/output":
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			var command struct {
				Enabled *bool `json:"enabled"`
			}
			if err := decodeCommand(w, r, &command); err != nil || command.Enabled == nil {
				fail(w, http.StatusBadRequest, "output command requires enabled: true or false")
				return
			}
			var err error
			if *command.Enabled {
				err = controller.Enable()
			} else {
				err = controller.Disable()
			}
			if err != nil {
				fail(w, http.StatusConflict, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, state())
		case "/api/simulation", "/api/lighting":
			if !requireMethod(w, r, http.MethodPost) {
				return
			}
			var command struct {
				Action  string   `json:"action"`
				MoodID  *string  `json:"moodId,omitempty"`
				Enabled *bool    `json:"enabled,omitempty"`
				Value   *float64 `json:"value,omitempty"`
			}
			if err := decodeCommand(w, r, &command); err != nil {
				fail(w, http.StatusBadRequest, "invalid simulation command")
				return
			}
			current, _ := controller.State()
			hardware := r.URL.Path == "/api/lighting"
			emergency := command.Action == "blackout" && command.Enabled != nil && *command.Enabled
			if (hardware && !current.OutputEnabled) || (!hardware && current.OutputEnabled && !emergency) {
				fail(w, http.StatusConflict, "output state changed; refresh and use the matching hardware or simulation controls")
				return
			}
			var err error
			switch {
			case command.Action == "select" && command.MoodID != nil && command.Enabled == nil && command.Value == nil:
				err = controller.Select(*command.MoodID)
			case command.Action == "blackout" && command.Enabled != nil && command.MoodID == nil && command.Value == nil:
				err = controller.Blackout(*command.Enabled, hardware)
			case command.Action == "master" && command.Value != nil && command.MoodID == nil && command.Enabled == nil:
				err = controller.Master(*command.Value)
			default:
				err = fmt.Errorf("use select with moodId, blackout with enabled, or master with value")
			}
			if err != nil {
				fail(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, state())
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
					c := store.Get()
					c.Output = config.Output{Mode: "simulation"}
					writeJSON(w, http.StatusOK, c)
					return
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
			importing := r.URL.Path == "/api/config/import"
			if importing {
				c.Output = config.Output{Mode: "simulation"}
			}
			if err := controller.CanConfigure(c, importing); err != nil {
				fail(w, http.StatusConflict, err.Error())
				return
			}
			if err := store.Save(c); err != nil {
				fail(w, http.StatusInternalServerError, fmt.Sprintf("configuration not saved: %v", err))
				return
			}
			controller.Configure(store.Get())
			if importing {
				_ = controller.Blackout(true, false)
			}
			writeJSON(w, http.StatusOK, c)
		default:
			fail(w, http.StatusNotFound, "API endpoint not found")
		}
	})
}

func decodeCommand(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(destination); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("command must contain exactly one JSON object")
	}
	return nil
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
