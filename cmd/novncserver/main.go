package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/abshkbh/arrakis/pkg/config"
)

const (
	baseDir = "/tmp/novncserver"
)

type novncServer struct {
	port string
}

// Health check endpoint
func (s *novncServer) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status": "healthy", "service": "novnc"}`)
}

// Proxy endpoint for noVNC
func (s *novncServer) proxyHandler(w http.ResponseWriter, r *http.Request) {
	// This would typically proxy to the noVNC service
	// For now, we'll return a simple response indicating the service is running
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <title>NoVNC Access</title>
</head>
<body>
    <h1>NoVNC Server</h1>
    <p>NoVNC service is running on port %s</p>
    <p>Connect to VNC server at localhost:5901</p>
</body>
</html>`, s.port)
}

func main() {
	var novncConfig *config.NoVNCServerConfig
	var configFile string

	app := &cli.App{
		Name:  "arrakis-novncserver",
		Usage: "NoVNC server for remote desktop access",
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
			novncConfig, err = config.GetNoVNCServerConfig(configFile)
			if err != nil {
				return fmt.Errorf("novnc server config not found: %v", err)
			}
			log.Infof("novnc server config: %v", novncConfig)
			return nil
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.WithError(err).Fatal("novnc server exited with error")
	}

	// Ensure base directory exists
	err = os.MkdirAll(baseDir, os.ModePerm)
	if err != nil {
		log.Fatalf("Failed to create base directory: %v", err)
	}

	// Create NoVNC server
	s := &novncServer{port: novncConfig.Port}
	r := mux.NewRouter()

	// Register routes
	r.HandleFunc("/health", s.healthCheck).Methods("GET")
	r.HandleFunc("/", s.proxyHandler).Methods("GET")
	r.PathPrefix("/").HandlerFunc(s.proxyHandler)

	// Start HTTP server
	srv := &http.Server{
		Addr:    ":" + novncConfig.Port,
		Handler: r,
	}

	go func() {
		log.Printf("NoVNC server listening on port: %s", novncConfig.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start novnc server: %v", err)
		}
	}()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info("Shutting down NoVNC server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Info("NoVNC server exited")
}
