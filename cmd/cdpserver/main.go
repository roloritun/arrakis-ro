package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/abshkbh/arrakis/pkg/config"
)

const (
	baseDir = "/tmp/cdpserver"
)

type cdpServer struct {
	port string
}

// Health check endpoint
func (s *cdpServer) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status": "healthy", "service": "cdp"}`)
}

// CDP endpoints proxy
func (s *cdpServer) versionHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"Browser":              "Chromium/120.0.0.0",
		"Protocol-Version":     "1.3",
		"User-Agent":           "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36",
		"V8-Version":           "12.0.267.17",
		"WebKit-Version":       "537.36",
		"webSocketDebuggerUrl": fmt.Sprintf("ws://localhost:%s/devtools/browser/", s.port),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *cdpServer) listHandler(w http.ResponseWriter, r *http.Request) {
	response := []map[string]interface{}{
		{
			"description":          "",
			"devtoolsFrontendUrl":  fmt.Sprintf("/devtools/inspector.html?ws=localhost:%s/devtools/page/", s.port),
			"id":                   "page_1",
			"title":                "New Tab",
			"type":                 "page",
			"url":                  "about:blank",
			"webSocketDebuggerUrl": fmt.Sprintf("ws://localhost:%s/devtools/page/page_1", s.port),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Start Chromium in headless mode with CDP enabled
func (s *cdpServer) startChromeHeadless() error {
	cmd := exec.Command(
		"chromium-browser",
		"--headless",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		"--remote-debugging-address=0.0.0.0",
		fmt.Sprintf("--remote-debugging-port=%s", s.port),
		"--disable-extensions",
		"--disable-plugins",
		"--disable-images",
	)

	log.Info("Starting Chromium in headless mode for CDP")
	return cmd.Start()
}

func main() {
	var cdpConfig *config.CDPServerConfig
	var configFile string

	app := &cli.App{
		Name:  "arrakis-cdpserver",
		Usage: "Chrome DevTools Protocol server for browser automation",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "Path to config file",
				Destination: &configFile,
				Value:       "./config.yaml",
			},
		},
		Action: func(ctx *cli.Context) error {
			var err error
			cdpConfig, err = config.GetCDPServerConfig(configFile)
			if err != nil {
				return fmt.Errorf("cdp server config not found: %v", err)
			}
			log.Infof("cdp server config: %v", cdpConfig)
			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.WithError(err).Fatal("cdp server exited with error")
	}

	// Ensure base directory exists
	err = os.MkdirAll(baseDir, os.ModePerm)
	if err != nil {
		log.Fatalf("Failed to create base directory: %v", err)
	}

	// Create CDP server
	s := &cdpServer{port: cdpConfig.Port}

	// Start Chromium in headless mode
	err = s.startChromeHeadless()
	if err != nil {
		log.Fatalf("Failed to start Chromium: %v", err)
	}

	// Give Chromium time to start
	time.Sleep(2 * time.Second)

	r := mux.NewRouter()

	// Register CDP routes
	r.HandleFunc("/health", s.healthCheck).Methods("GET")
	r.HandleFunc("/json/version", s.versionHandler).Methods("GET")
	r.HandleFunc("/json", s.listHandler).Methods("GET")
	r.HandleFunc("/json/list", s.listHandler).Methods("GET")

	// Start HTTP server
	srv := &http.Server{
		Addr:    ":" + cdpConfig.Port,
		Handler: r,
	}

	go func() {
		log.Printf("CDP server listening on port: %s", cdpConfig.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start cdp server: %v", err)
		}
	}()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info("Shutting down CDP server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Info("CDP server exited")
}
