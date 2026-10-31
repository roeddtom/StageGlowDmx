package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

func serve(ctx context.Context, handler http.Handler, address, httpAddress, certFile, keyFile string) error {
	var certificate tls.Certificate
	var err error
	if certFile != "" {
		certificate, err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return fmt.Errorf("HTTPS certificate: %w", err)
		}
	}
	addresses := []string{address}
	if httpAddress != "" {
		addresses = append(addresses, httpAddress)
	}
	listeners := make([]net.Listener, 0, len(addresses))
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	// Bind both before serving so a failed second port cannot leave a partial startup.
	for _, addr := range addresses {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("listen %s: %w", addr, err)
		}
		listeners = append(listeners, listener)
	}
	servers := make([]*http.Server, 0, len(listeners))
	served := make(chan error, len(listeners))
	for i, listener := range listeners {
		srv := &http.Server{
			Handler: handler, ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second,
			IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10,
		}
		servers = append(servers, srv)
		protocol := "http"
		if i == 0 && certFile != "" {
			listener = tls.NewListener(listener, &tls.Config{
				Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12,
			})
			protocol = "https"
		}
		log.Printf("StageGlow listening on %s://%s; USB output requires explicit enablement, Art-Net unavailable", protocol, listener.Addr())
		go func(listener net.Listener) { served <- srv.Serve(listener) }(listener)
	}
	select {
	case err = <-served:
		if err == http.ErrServerClosed {
			err = nil
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, srv := range servers {
		if shutdownErr := srv.Shutdown(shutdown); shutdownErr != nil {
			_ = srv.Close()
			if err == nil {
				err = shutdownErr
			}
		}
	}
	return err
}
