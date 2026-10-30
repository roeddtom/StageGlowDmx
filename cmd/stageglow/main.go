package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"stageglowdmx/internal/config"
	"stageglowdmx/internal/engine"
	"stageglowdmx/internal/output"
	"stageglowdmx/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	data := flag.String("data", "./data", "configuration directory")
	tlsCert := flag.String("tls-cert", "", "optional HTTPS certificate PEM file (requires tls-key)")
	tlsKey := flag.String("tls-key", "", "optional HTTPS private key PEM file (requires tls-cert)")
	flag.Parse()
	if (*tlsCert == "") != (*tlsKey == "") {
		return fmt.Errorf("tls-cert and tls-key must be supplied together")
	}
	store, err := config.Open(*data)
	if err != nil {
		return err
	}
	lighting := engine.New(store.Get(), nil, time.Now().UnixNano())
	controller := output.New(store.Get(), lighting, nil)
	defer func() {
		if err := controller.Shutdown(); err != nil {
			log.Printf("Output shutdown: %v", err)
		}
	}()
	outputCtx, stopOutput := context.WithCancel(context.Background())
	defer stopOutput()
	go controller.Run(outputCtx)
	srv := &http.Server{
		Addr:              *listen,
		Handler:           server.NewWithOutput(store, frontendHandler(), controller),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	served := make(chan error, 1)
	protocol := "http"
	if *tlsCert != "" {
		protocol = "https"
	}
	go func() {
		if *tlsCert != "" {
			served <- srv.ListenAndServeTLS(*tlsCert, *tlsKey)
		} else {
			served <- srv.ListenAndServe()
		}
	}()
	log.Printf("StageGlow listening on %s://%s; USB output requires explicit enablement, Art-Net unavailable", protocol, *listen)
	select {
	case err := <-served:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("HTTP service: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			_ = srv.Close()
			return fmt.Errorf("HTTP shutdown: %w", err)
		}
		return nil
	}
}
