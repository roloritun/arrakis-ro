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

// Serve the standard noVNC client interface
func (s *novncServer) proxyHandler(w http.ResponseWriter, r *http.Request) {
	// Serve the standard noVNC vnc.html with proper websocket configuration
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	
	// Use the standard noVNC client from CDN with proper websockify configuration
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>noVNC</title>
    <meta charset="utf-8">
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/novnc@1.4.0/app/styles/base.css">
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/novnc@1.4.0/app/styles/ui.css">
    <link rel="icon" sizes="16x16" type="image/png" href="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAABmJLR0QA/wD/AP+gvaeTAAAACXBIWXMAAAsTAAALEwEAmpwYAAAAB3RJTUUH3gQSFCURp1uHSgAAAZZJREFUOMulkz1LA0EQhp9JQhBbwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sLwcJCG1sL">
</head>

<body id="noVNC_body">
    <div id="noVNC_container">
        <!-- Status/errors -->
        <div id="noVNC_status_bar">
            <table border="0">
                <tr>
                    <td>
                        <div id="noVNC_status"></div>
                    </td>
                    <td width="1%">
                        <div id="noVNC_buttons">
                            <input type="button" value="Send CtrlAltDel" id="sendCtrlAltDelButton">
                            <span id="noVNC_xvp_buttons">
                                <input type="button" value="Shutdown" id="xvpShutdownButton">
                                <input type="button" value="Reboot" id="xvpRebootButton">
                                <input type="button" value="Reset" id="xvpResetButton">
                            </span>
                        </div>
                    </td>
                </tr>
            </table>
        </div>

        <!-- Scrollable VNC area -->
        <div id="noVNC_viewport">
            <canvas id="noVNC_canvas" width="640" height="480" style="cursor: none;">
                Canvas not supported.
            </canvas>
        </div>

        <!-- Settings Panel -->
        <div id="noVNC_settings">
            <ul>
                <li><input id="noVNC_setting_encrypt" type="checkbox"> Encrypt</li>
                <li><input id="noVNC_setting_true_color" type="checkbox" checked> True Color</li>
                <li><input id="noVNC_setting_cursor" type="checkbox" checked> Local Cursor</li>
                <li><input id="noVNC_setting_clip" type="checkbox" checked> Clip to Window</li>
                <li><input id="noVNC_setting_resize" type="checkbox" checked> Resize</li>
                <li><input id="noVNC_setting_shared" type="checkbox"> Shared Mode</li>
                <li><input id="noVNC_setting_view_only" type="checkbox"> View Only</li>
                <li><input id="noVNC_setting_path" type="input" value="websockify"> Path</li>
                <li><input id="noVNC_setting_repeaterID" type="input"> Repeater ID</li>
                <li><input id="noVNC_setting_logging" type="input"> Logging</li>
            </ul>
        </div>

        <!-- Connection Panel -->
        <div id="noVNC_controls">
            <ul>
                <li><label><strong>Host:</strong> <input id="noVNC_setting_host" value="` + r.Host + `"/></label></li>
                <li><label><strong>Port:</strong> <input id="noVNC_setting_port" value="` + s.port + `"/></label></li>
                <li><label><strong>Password:</strong> <input id="noVNC_setting_password" type="password" value="elara0000"/></label></li>
                <li><input id="noVNC_connect_button" type="button" value="Connect"/></li>
            </ul>
        </div>
    </div>

    <!-- Include standard noVNC JavaScript -->
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/util/logging.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/util/base64.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/websock.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/des.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/input/keysymdef.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/input/xtscancodes.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/input/util.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/input/devices.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/display.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/inflator.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/lib/rfb.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/novnc@1.4.0/app/ui.js"></script>

    <script>
        // Standard noVNC UI initialization
        var UI = {};

        // Initialize noVNC
        function initNoVNC() {
            var host = document.getElementById('noVNC_setting_host').value;
            var port = document.getElementById('noVNC_setting_port').value;
            var password = document.getElementById('noVNC_setting_password').value;
            var path = document.getElementById('noVNC_setting_path').value;
            
            // Build WebSocket URL for our websockify endpoint
            var url = 'ws://' + host + '/' + path;
            
            UI.rfb = new RFB(document.getElementById('noVNC_canvas'), url, {
                credentials: { password: password }
            });

            UI.rfb.addEventListener("connect", UI.connected);
            UI.rfb.addEventListener("disconnect", UI.disconnected);
            UI.rfb.addEventListener("credentialsrequired", UI.credentialsRequired);
            UI.rfb.addEventListener("securityfailure", UI.securityFailed);
        }

        UI.connected = function() {
            document.getElementById('noVNC_status').textContent = 'Connected';
            document.getElementById('noVNC_connect_button').value = 'Disconnect';
            document.getElementById('noVNC_connect_button').onclick = UI.disconnect;
        };

        UI.disconnected = function() {
            document.getElementById('noVNC_status').textContent = 'Disconnected';
            document.getElementById('noVNC_connect_button').value = 'Connect';
            document.getElementById('noVNC_connect_button').onclick = UI.connect;
        };

        UI.credentialsRequired = function() {
            document.getElementById('noVNC_status').textContent = 'Credentials Required';
        };

        UI.securityFailed = function() {
            document.getElementById('noVNC_status').textContent = 'Security Failed';
        };

        UI.connect = function() {
            document.getElementById('noVNC_status').textContent = 'Connecting...';
            initNoVNC();
        };

        UI.disconnect = function() {
            if (UI.rfb) {
                UI.rfb.disconnect();
                UI.rfb = null;
            }
        };

        // Setup event handlers
        document.getElementById('noVNC_connect_button').onclick = UI.connect;
        document.getElementById('sendCtrlAltDelButton').onclick = function() {
            if (UI.rfb) UI.rfb.sendCtrlAltDel();
        };

        // Auto-connect after a short delay
        setTimeout(function() {
            document.getElementById('noVNC_connect_button').click();
        }, 1000);
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
