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

	// Bidirectional proxy between WebSocket and VNC
	// Handle WebSocket to VNC direction
	go func() {
		defer vncConn.Close()
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("WebSocket read error: %v", err)
				break
			}
			
			// Only handle binary messages for VNC protocol
			if messageType == websocket.BinaryMessage {
				if _, err := vncConn.Write(message); err != nil {
					log.Printf("VNC write error: %v", err)
					break
				}
			}
		}
	}()

	// Handle VNC to WebSocket direction
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
	
	log.Printf("WebSocket connection closed for %s", r.RemoteAddr)
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
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/rfb.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/util/events.js"></script>
    <style>
        body { 
            margin: 0; 
            padding: 20px; 
            font-family: Arial, sans-serif; 
            background: #f5f5f5;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        h1 {
            color: #333;
            margin-bottom: 20px;
        }
        .controls {
            margin: 10px 0;
            padding: 10px;
            background: #f8f9fa;
            border-radius: 4px;
        }
        .status { 
            margin: 10px 0; 
            padding: 10px; 
            background: #fff3cd; 
            border: 1px solid #ffeaa7;
            border-radius: 4px;
            font-weight: bold;
        }
        .status.connected { 
            background: #d4edda; 
            border-color: #c3e6cb; 
            color: #155724;
        }
        .status.disconnected { 
            background: #f8d7da; 
            border-color: #f5c6cb; 
            color: #721c24;
        }
        #noVNC_container {
            margin: 20px 0;
            border: 2px solid #ddd;
            border-radius: 4px;
            overflow: hidden;
        }
        #noVNC_screen {
            display: block;
        }
        button {
            padding: 8px 16px;
            margin: 5px;
            border: 1px solid #ccc;
            border-radius: 4px;
            background: white;
            cursor: pointer;
        }
        button:hover {
            background: #f8f9fa;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Arrakis Remote Desktop - noVNC</h1>
        
        <div class="controls">
            <button onclick="connectVNC()">Connect</button>
            <button onclick="disconnectVNC()">Disconnect</button>
            <button onclick="sendCtrlAltDel()">Ctrl+Alt+Del</button>
        </div>
        
        <div class="status" id="status">Click Connect to start remote desktop session</div>
        
        <div id="noVNC_container">
            <canvas id="noVNC_screen" width="1024" height="768">
                Canvas not supported.
            </canvas>
        </div>
    </div>
    
    <script>
        let rfb = null;
        
        function updateStatus(text, type = '') {
            const status = document.getElementById('status');
            status.textContent = text;
            status.className = 'status';
            if (type) status.classList.add(type);
        }
        
        function connectVNC() {
            if (rfb) {
                rfb.disconnect();
            }
            
            updateStatus('Connecting to VNC server...');
            
            const host = window.location.host;
            const url = 'ws://' + host + '/websockify';
            
            rfb = new RFB(document.getElementById('noVNC_screen'), url, {
                credentials: { password: 'elara0000' }
            });
            
            rfb.addEventListener("connect", () => {
                updateStatus('Connected to remote desktop', 'connected');
            });
            
            rfb.addEventListener("disconnect", (e) => {
                updateStatus('Disconnected: ' + e.detail.reason, 'disconnected');
                rfb = null;
            });
            
            rfb.addEventListener("credentialsrequired", () => {
                updateStatus('VNC authentication required');
            });
            
            rfb.addEventListener("securityfailure", (e) => {
                updateStatus('Security failure: ' + e.detail.reason, 'disconnected');
            });
            
            // Configure noVNC settings
            rfb.scaleViewport = true;
            rfb.resizeSession = true;
        }
        
        function disconnectVNC() {
            if (rfb) {
                rfb.disconnect();
                rfb = null;
            }
            updateStatus('Disconnected', 'disconnected');
        }
        
        function sendCtrlAltDel() {
            if (rfb) {
                rfb.sendCtrlAltDel();
            }
        }
        
        // Auto-connect on page load
        window.addEventListener('load', () => {
            setTimeout(connectVNC, 1000);
        });
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
