package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/abshkbh/arrakis/pkg/config"
)

const (
	baseDir = "/tmp/novncserver"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for simplicity
	},
}

type novncServer struct {
	port string
}

// Health check endpoint
func (s *novncServer) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status": "healthy", "service": "novnc"}`)
}

// WebSocket proxy for VNC connection
func (s *novncServer) websocketHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Connect to VNC server
	vncConn, err := net.Dial("tcp", "localhost:5901")
	if err != nil {
		log.Printf("Failed to connect to VNC server: %v", err)
		return
	}
	defer vncConn.Close()

	// Bidirectional proxy between WebSocket and VNC
	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				break
			}
			if _, err := vncConn.Write(message); err != nil {
				log.Printf("VNC write error: %v", err)
				break
			}
		}
	}()

	// Read from VNC and send to WebSocket
	buffer := make([]byte, 4096)
	for {
		n, err := vncConn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				log.Printf("VNC read error: %v", err)
			}
			break
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); err != nil {
			log.Printf("WebSocket write error: %v", err)
			break
		}
	}
}

// Serve a functional noVNC web interface
func (s *novncServer) proxyHandler(w http.ResponseWriter, r *http.Request) {
	// Serve a working noVNC interface that connects via websocket
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>noVNC Remote Desktop</title>
    <meta charset="utf-8">
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/rfb.js"></script>
    <style>
        body { margin: 0; padding: 20px; font-family: Arial, sans-serif; }
        #screen { border: 1px solid #ccc; }
        .status { margin: 10px 0; padding: 10px; background: #f0f0f0; }
    </style>
</head>
<body>
    <h1>Arrakis Remote Desktop</h1>
    <div class="status" id="status">Connecting...</div>
    <canvas id="screen" width="800" height="600"></canvas>
    
    <script>
        const rfb = new RFB(document.getElementById('screen'), 
                           'ws://' + window.location.host + '/websockify', {
            credentials: { password: 'elara0000' }
        });
        
        rfb.addEventListener("connect", () => {
            document.getElementById('status').textContent = 'Connected to remote desktop';
            document.getElementById('status').style.background = '#d4edda';
        });
        
        rfb.addEventListener("disconnect", () => {
            document.getElementById('status').textContent = 'Disconnected from remote desktop';
            document.getElementById('status').style.background = '#f8d7da';
        });
        
        rfb.scaleViewport = true;
        rfb.resizeSession = true;
    </script>
</body>
</html>`)
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
	r.HandleFunc("/websockify", s.websocketHandler)
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
