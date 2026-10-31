package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func testAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func TestSimultaneousListenersShareHandlerAndStop(t *testing.T) {
	fixture := httptest.NewTLSServer(http.NotFoundHandler())
	certificate := fixture.TLS.Certificates[0]
	fixture.Close()
	key, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); err != nil {
		t.Fatal(err)
	}
	httpsAddr, httpAddr := testAddress(t), testAddress(t)
	var requests atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		io.WriteString(w, "shared engine")
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- serve(ctx, handler, httpsAddr, httpAddr, certFile, keyFile) }()
	roots := x509.NewCertPool()
	roots.AddCert(fixture.Certificate())
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	for _, url := range []string{"http://" + httpAddr, "https://" + httpsAddr} {
		deadline := time.Now().Add(3 * time.Second)
		for {
			response, err := client.Get(url)
			if err == nil {
				body, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if string(body) != "shared engine" {
					t.Fatalf("body = %q", body)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatal(err)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	if requests.Load() != 2 {
		t.Fatalf("shared requests = %d", requests.Load())
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("listeners did not stop")
	}
	for _, addr := range []string{httpsAddr, httpAddr} {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			t.Fatalf("listener still open: %s", addr)
		}
	}
}

func TestFailedSecondListenerClosesFirst(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	first := testAddress(t)
	if err := serve(context.Background(), http.NotFoundHandler(), first, occupied.Addr().String(), "", ""); err == nil {
		t.Fatal("occupied port accepted")
	}
	reopened, err := net.Listen("tcp", first)
	if err != nil {
		t.Fatalf("partial startup left first listener open: %v", err)
	}
	reopened.Close()
}
