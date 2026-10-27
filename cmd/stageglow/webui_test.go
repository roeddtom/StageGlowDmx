package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFrontendRouting(t *testing.T) {
	handler := frontendAssets(fstest.MapFS{
		"index.html":   {Data: []byte("<html>StageGlow</html>")},
		"assets/ui.js": {Data: []byte("console.log('offline')")},
	})
	for _, tc := range []struct {
		method string
		path   string
		status int
		body   string
	}{
		{"GET", "/", 200, "<html>StageGlow</html>"},
		{"GET", "/fixtures", 200, "<html>StageGlow</html>"},
		{"GET", "/assets/ui.js", 200, "console.log('offline')"},
		{"GET", "/assets/missing.js", 404, "404"},
		{"GET", "/assets/missing", 404, "404"},
		{"GET", "/missing.css", 404, "404"},
		{"POST", "/fixtures", 405, "method not allowed"},
		{"HEAD", "/", 200, ""},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if tc.method == http.MethodHead {
				if response.Body.Len() != 0 {
					t.Fatal("HEAD returned a body")
				}
			} else if !strings.Contains(response.Body.String(), tc.body) {
				t.Fatalf("unexpected body: %q", response.Body.String())
			}
			if tc.status == 200 && response.Header().Get("Content-Security-Policy") == "" {
				t.Fatal("missing content security policy")
			}
		})
	}
}

func TestMissingFrontendIsActionable(t *testing.T) {
	response := httptest.NewRecorder()
	frontendAssets(fstest.MapFS{}).ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	if response.Code != 503 || !strings.Contains(response.Body.String(), "Build the web application") {
		t.Fatalf("unexpected response: %d %q", response.Code, response.Body.String())
	}
}
