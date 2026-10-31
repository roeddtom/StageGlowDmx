package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
	httpListen := flag.String("http-listen", "", "optional additional HTTP address when TLS is enabled (trusted LAN only)")
	data := flag.String("data", "./data", "configuration directory")
	tlsCert := flag.String("tls-cert", "", "optional HTTPS certificate PEM file (requires tls-key)")
	tlsKey := flag.String("tls-key", "", "optional HTTPS private key PEM file (requires tls-cert)")
	flag.Parse()
	if (*tlsCert == "") != (*tlsKey == "") {
		return fmt.Errorf("tls-cert and tls-key must be supplied together")
	}
	if *httpListen != "" && *tlsCert == "" {
		return fmt.Errorf("http-listen requires tls-cert and tls-key")
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, server.NewWithOutput(store, frontendHandler(), controller), *listen, *httpListen, *tlsCert, *tlsKey)
}
