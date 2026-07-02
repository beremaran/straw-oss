package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/control"
	"github.com/beremaran/straw/v2/internal/natsx"
)

func main() {
	configPath := flag.String("config", "", "path to the control config file")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "missing required -config flag")
		os.Exit(2)
	}

	controlConfig, err := config.LoadControl(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := natsx.ValidateServers(controlConfig.NATS.Servers); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := natsx.ValidateMaxPayload(controlConfig.NATS.MaxPayloadBytes, controlConfig.Transport.MaxFrameDataBytes, controlConfig.Request.MaxInlineRequestBodyBytes, controlConfig.Request.MaxInlineResponseBodyBytes); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	handler := control.NewRequestHandler(
		controlConfig.Request.MaxInlineRequestBodyBytes,
		controlConfig.Request.MaxInlineResponseBodyBytes,
		controlConfig.Request.MaxTimeoutMs,
	)

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/requests", handler)

	addr := fmt.Sprintf("%s:%d", controlConfig.Server.Host, controlConfig.Server.APIPort)
	log.Printf("control: listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("control: %v", err)
	}
}
