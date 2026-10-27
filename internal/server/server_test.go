package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"stageglowdmx/internal/config"
)

func setup(t *testing.T) (http.Handler, *config.Store, string) {
	t.Helper()
	dir, err := os.MkdirTemp(".", ".server-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	store, err := config.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ui := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("offline UI"))
	})
	return New(store, ui), store, dir
}

func request(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://stageglow.local:8080"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func payload(t *testing.T, c config.Config) string {
	t.Helper()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestConfigurationHTTPRoundTrip(t *testing.T) {
	h, store, dir := setup(t)
	initial := request(h, http.MethodGet, "/api/config", "")
	if initial.Code != http.StatusOK {
		t.Fatal(initial.Code, initial.Body.String())
	}
	c, err := config.Decode(initial.Body)
	if err != nil || !reflect.DeepEqual(c, config.Defaults()) {
		t.Fatal("invalid default response", err)
	}
	c.BPM = 140
	saved := request(h, http.MethodPut, "/api/config", payload(t, c))
	if saved.Code != http.StatusOK {
		t.Fatal(saved.Code, saved.Body.String())
	}
	exported := request(h, http.MethodGet, "/api/config/export", "")
	if exported.Header().Get("Content-Disposition") != `attachment; filename="stageglow-config.json"` {
		t.Fatal("export must be downloadable")
	}
	c.BPM = 160
	if w := request(h, http.MethodPut, "/api/config", payload(t, c)); w.Code != http.StatusOK {
		t.Fatal(w.Code, w.Body.String())
	}
	imported := request(h, http.MethodPost, "/api/config/import", exported.Body.String())
	if imported.Code != http.StatusOK || store.Get().BPM != 140 {
		t.Fatal("import round-trip failed", imported.Code, imported.Body.String())
	}
	reopened, err := config.Open(dir)
	if err != nil || reopened.Get().BPM != 140 {
		t.Fatal("HTTP save was not persisted", err)
	}
}

func TestRejectedImportsPreserveStateAndBackup(t *testing.T) {
	h, store, dir := setup(t)
	c := store.Get()
	c.BPM = 150
	if err := store.Save(c); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(dir, "config.json.bak")
	before, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"schemaVersion":999}`,
		payload(t, c) + "{}",
		strings.Replace(payload(t, c), `"bpm":150`, `"bpm":150,"outputEnabled":true`, 1),
		strings.Replace(payload(t, c), `"bpm":150`, `"bpm":0`, 1),
	} {
		w := request(h, http.MethodPost, "/api/config/import", body)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"error"`) {
			t.Fatal("invalid import accepted", w.Code, w.Body.String())
		}
	}
	after, _ := os.ReadFile(backupPath)
	if store.Get().BPM != 150 || string(before) != string(after) {
		t.Fatal("failed import changed live state or backup")
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	w := request(h, http.MethodPost, "/api/config/import", payload(t, c))
	if w.Code != http.StatusInternalServerError || store.Get().BPM != 150 {
		t.Fatal("filesystem/configuration corruption must fail closed with 500", w.Code, w.Body.String())
	}
}

func TestRequestSizeBound(t *testing.T) {
	h, _, _ := setup(t)
	body := `{"schemaVersion":1,"profiles":[],"groups":[],"loadouts":[],"activeLoadoutId":"` +
		strings.Repeat("x", MaxConfigBytes) + `","artNet":{"destinationIp":"","portAddress":0},"bpm":120}`
	w := request(h, http.MethodPut, "/api/config", body)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestFailClosedStateRoutingAndMethods(t *testing.T) {
	h, _, _ := setup(t)
	w := request(h, http.MethodGet, "/api/state", "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"blackout":true,"outputEnabled":false,"engineStatus":"not-implemented"}` {
		t.Fatal("runtime state is not fail-closed", w.Body.String())
	}
	for _, path := range []string{"/api/arm", "/api/blackout", "/api/unknown", "/api"} {
		w := request(h, http.MethodPost, path, "")
		if w.Code != http.StatusNotFound || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
			t.Fatal("unknown API fell back to UI", path, w.Code)
		}
	}
	w = request(h, http.MethodDelete, "/api/config", "")
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET, PUT" {
		t.Fatal("incorrect method response", w.Code, w.Header())
	}
	w = request(h, http.MethodGet, "/fixtures", "")
	if w.Code != http.StatusOK || w.Body.String() != "offline UI" {
		t.Fatal("frontend fallback unavailable")
	}
	w = request(h, http.MethodGet, "/api/health", "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"status":"ok"}` {
		t.Fatal("health endpoint failed")
	}
}

func TestBrowserRequestGuards(t *testing.T) {
	h, store, _ := setup(t)
	tests := []struct {
		name, host, origin, site string
		status                   int
	}{
		{"same origin", "stageglow.local:8080", "http://stageglow.local:8080", "same-origin", 200},
		{"LAN IP", "192.168.1.20:8080", "http://192.168.1.20:8080", "", 200},
		{"localhost", "localhost:8080", "", "", 200},
		{"hostname", "stageglow:8080", "", "", 200},
		{"IPv6 loopback", "[::1]:8080", "http://[::1]:8080", "", 200},
		{"cross origin", "stageglow.local:8080", "https://evil.example", "", 403},
		{"different port", "stageglow.local:8080", "http://stageglow.local:9000", "", 403},
		{"opaque origin", "stageglow.local:8080", "null", "", 403},
		{"origin path", "stageglow.local:8080", "http://stageglow.local:8080/evil", "", 403},
		{"cross site metadata", "stageglow.local:8080", "", "cross-site", 403},
		{"same site metadata", "stageglow.local:8080", "", "same-site", 403},
		{"DNS rebinding", "attacker.example:8080", "http://attacker.example:8080", "same-origin", 403},
		{"host userinfo", "evil@stageglow.local", "", "", 403},
		{"bad port", "stageglow.local:99999", "", "", 403},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPut, "http://stageglow.local:8080/api/config", strings.NewReader(payload(t, store.Get())))
			r.Host = test.host
			r.Header.Set("Origin", test.origin)
			r.Header.Set("Sec-Fetch-Site", test.site)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatal(w.Code, w.Body.String())
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("CORS must not be enabled")
			}
		})
	}
}
