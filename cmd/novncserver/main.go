package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
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

// WebSocket proxy for VNC connection (websockify protocol)
func (s *novncServer) websocketHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("WebSocket connection established from %s", r.RemoteAddr)

	// Connect to VNC server (running on localhost:5901)
	vncConn, err := net.Dial("tcp", "localhost:5901")
	if err != nil {
		log.Printf("Failed to connect to VNC server: %v", err)
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "VNC server unavailable"))
		return
	}
	defer vncConn.Close()

	log.Printf("Connected to VNC server at localhost:5901")

	// Channel to signal connection close
	done := make(chan struct{})

	// Handle WebSocket to VNC direction
	go func() {
		defer close(done)
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				return
			}
			
			// Handle both binary and text messages (websockify protocol)
			if messageType == websocket.BinaryMessage || messageType == websocket.TextMessage {
				// For websockify, we may receive base64 encoded data in text messages
				var data []byte
				if messageType == websocket.TextMessage {
					// For text messages, assume they are base64 encoded VNC data
					log.Printf("Received text message, treating as binary")
					data = message
				} else {
					data = message
				}
				
				if _, err := vncConn.Write(data); err != nil {
					log.Printf("VNC write error: %v", err)
					return
				}
			}
		}
	}()

	// Handle VNC to WebSocket direction
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := vncConn.Read(buffer)
			if err != nil {
				if err != io.EOF {
					log.Printf("VNC read error: %v", err)
				}
				close(done)
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); err != nil {
				log.Printf("WebSocket write error: %v", err)
				close(done)
				return
			}
		}
	}()

	// Wait for either direction to close
	<-done
	log.Printf("WebSocket connection closed for %s", r.RemoteAddr)
}

// Serve the standard noVNC client files from /opt/novnc
func (s *novncServer) proxyHandler(w http.ResponseWriter, r *http.Request) {
	// Serve files from the actual noVNC installation at /opt/novnc
	path := r.URL.Path
	
	// Default to index.html (which is symlinked to vnc.html)
	if path == "/" {
		path = "/index.html"
	}
	
	// Serve the file from /opt/novnc
	filePath := "/opt/novnc" + path
	
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// If file doesn't exist, serve the main vnc.html
		filePath = "/opt/novnc/vnc.html"
	}
	
	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("Error reading file %s: %v", filePath, err)
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	
	// Set proper content type
	if strings.HasSuffix(filePath, ".html") {
		w.Header().Set("Content-Type", "text/html")
	} else if strings.HasSuffix(filePath, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	} else if strings.HasSuffix(filePath, ".css") {
		w.Header().Set("Content-Type", "text/css")
	} else if strings.HasSuffix(filePath, ".png") {
		w.Header().Set("Content-Type", "image/png")
	} else if strings.HasSuffix(filePath, ".ico") {
		w.Header().Set("Content-Type", "image/x-icon")
	}
	
	// If it's the main HTML file, modify it to use our websocket endpoint
	if strings.HasSuffix(filePath, ".html") {
		htmlContent := string(content)
		
		// Modify the HTML to use our websockify endpoint and auto-configure
		htmlContent = strings.ReplaceAll(htmlContent, 
			`<script src="app/ui.js"></script>`,
			`<script src="app/ui.js"></script>
			<script>
				// Auto-configure for Arrakis
				window.addEventListener('load', function() {
					setTimeout(function() {
						// Set connection parameters
						if (document.getElementById('noVNC_setting_host')) {
							document.getElementById('noVNC_setting_host').value = window.location.hostname;
						}
						if (document.getElementById('noVNC_setting_port')) {
							document.getElementById('noVNC_setting_port').value = window.location.port || '`+s.port+`';
						}
						if (document.getElementById('noVNC_setting_password')) {
							document.getElementById('noVNC_setting_password').value = 'elara0000';
						}
						if (document.getElementById('noVNC_setting_path')) {
							document.getElementById('noVNC_setting_path').value = 'websockify';
						}
						
						// Auto-connect
						if (document.getElementById('noVNC_connect_button')) {
							document.getElementById('noVNC_connect_button').click();
						}
					}, 1000);
				});
			</script>`)
		
		content = []byte(htmlContent)
	}
	
	w.Write(content)
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
	r.StrictSlash(true) // Automatically handle trailing slashes

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
