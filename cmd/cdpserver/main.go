package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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
	baseDir = "/tmp/cdpserver"
)

type cdpServer struct {
	port       string  // External port for our CDP server
	restAPIURL string  // REST API URL to query VM info
}

// VM represents a VM from the REST API
type VM struct {
	VMName       string        `json:"vmName"`
	Status       string        `json:"status"`
	IP           string        `json:"ip"`
	PortForwards []PortForward `json:"portForwards"`
}

type PortForward struct {
	Description string `json:"description"`
	GuestPort   string `json:"guestPort"`
	HostPort    string `json:"hostPort"`
}

type VMResponse struct {
	VMs []VM `json:"vms"`
}

// discoverCDPPort queries the REST API to find the dynamic CDP port for any running VM
func (s *cdpServer) discoverCDPPort() (string, error) {
	resp, err := http.Get(s.restAPIURL + "/v1/vms")
	if err != nil {
		return "", fmt.Errorf("failed to query VM API: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	var vmResponse VMResponse
	if err := json.Unmarshal(body, &vmResponse); err != nil {
		return "", fmt.Errorf("failed to parse VM response: %v", err)
	}

	// Find the first running VM with CDP port forwarding
	for _, vm := range vmResponse.VMs {
		if vm.Status == "RUNNING" {
			for _, pf := range vm.PortForwards {
				if pf.GuestPort == "9222" && pf.Description == "cdp" {
					log.Infof("Found running VM '%s' with CDP port forwarded from guest:%s to host:%s", 
						vm.VMName, pf.GuestPort, pf.HostPort)
					return pf.HostPort, nil
				}
			}
		}
	}

	return "", fmt.Errorf("no running VM found with CDP port forwarding")
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// WebSocket proxy handler for DevTools connections
func (s *cdpServer) websocketProxy(w http.ResponseWriter, r *http.Request) {
	log.Infof("WebSocket connection request: %s", r.URL.Path)
	
	// Discover the dynamic CDP port for the running VM
	cdpPort, err := s.discoverCDPPort()
	if err != nil {
		log.Errorf("Failed to discover CDP port: %v", err)
		http.Error(w, "No VM with CDP available", http.StatusServiceUnavailable)
		return
	}
	
	// Upgrade the HTTP connection to WebSocket
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade WebSocket: %v", err)
		return
	}
	defer func() {
		if err := clientConn.Close(); err != nil {
			log.Debugf("Error closing client connection: %v", err)
		}
	}()

	// Extract the target path - Chrome expects the same path structure
	targetPath := r.URL.Path
	if r.URL.RawQuery != "" {
		targetPath += "?" + r.URL.RawQuery
	}

	// Connect to local Chrome DevTools WebSocket via dynamic port
	chromeURL := fmt.Sprintf("ws://127.0.0.1:%s%s", cdpPort, targetPath)
	log.Infof("Proxying WebSocket to Chrome: %s", chromeURL)

	chromeConn, _, err := websocket.DefaultDialer.Dial(chromeURL, nil)
	if err != nil {
		log.Errorf("Failed to connect to Chrome DevTools at %s: %v", chromeURL, err)
		// Send close message to client instead of just returning
		clientConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(1002, "Chrome not available"))
		return
	}
	defer func() {
		if err := chromeConn.Close(); err != nil {
			log.Debugf("Error closing Chrome connection: %v", err)
		}
	}()

	log.Infof("Successfully connected to Chrome DevTools, starting proxy")

	// Proxy messages in both directions
	done := make(chan struct{})

	// Client -> Chrome
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("Panic in client->chrome proxy: %v", r)
			}
			select {
			case <-done:
			default:
				close(done)
			}
		}()
		for {
			select {
			case <-done:
				return
			default:
				messageType, data, err := clientConn.ReadMessage()
				if err != nil {
					log.Debugf("Client connection closed: %v", err)
					return
				}
				if err := chromeConn.WriteMessage(messageType, data); err != nil {
					log.Debugf("Failed to write to Chrome: %v", err)
					return
				}
			}
		}
	}()

	// Chrome -> Client
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("Panic in chrome->client proxy: %v", r)
			}
			select {
			case <-done:
			default:
				close(done)
			}
		}()
		for {
			select {
			case <-done:
				return
			default:
				messageType, data, err := chromeConn.ReadMessage()
				if err != nil {
					log.Debugf("Chrome connection closed: %v", err)
					return
				}
				if err := clientConn.WriteMessage(messageType, data); err != nil {
					log.Debugf("Failed to write to client: %v", err)
					return
				}
			}
		}
	}()

	// Wait for either connection to close
	<-done
	log.Debug("WebSocket proxy connection closed")
}

// Health check endpoint
func (s *cdpServer) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status": "healthy", "service": "cdp"}`)
}

// CDP endpoints proxy
func (s *cdpServer) versionHandler(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		host = "localhost:3007" // fallback
	}
	
	response := map[string]interface{}{
		"Browser":              "Chromium/120.0.0.0",
		"Protocol-Version":     "1.3",
		"User-Agent":           "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36",
		"V8-Version":           "12.0.267.17",
		"WebKit-Version":       "537.36",
		"webSocketDebuggerUrl": fmt.Sprintf("ws://%s/devtools/browser/", host),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *cdpServer) listHandler(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		host = "localhost:3007" // fallback
	}
	
	response := []map[string]interface{}{
		{
			"description":          "",
			"devtoolsFrontendUrl":  fmt.Sprintf("/devtools/inspector.html?ws=%s/devtools/page/", host),
			"id":                   "page_1",
			"title":                "New Tab",
			"type":                 "page",
			"url":                  "about:blank",
			"webSocketDebuggerUrl": fmt.Sprintf("ws://%s/devtools/page/page_1", host),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Start Chromium with GUI (non-headless) and CDP enabled
func (s *cdpServer) startChrome() error {
	// Try different possible Chrome executable names
	chromeExes := []string{"google-chrome", "chromium-browser", "chromium"}
	var chromeExe string
	
	for _, exe := range chromeExes {
		if _, err := exec.LookPath(exe); err == nil {
			chromeExe = exe
			break
		}
	}
	
	if chromeExe == "" {
		return fmt.Errorf("no Chrome/Chromium executable found. Please install: sudo apt install chromium-browser")
	}
	
	cmd := exec.Command(
		chromeExe,
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--remote-debugging-address=0.0.0.0",
		fmt.Sprintf("--remote-debugging-port=%s", s.chromePort),
		"--disable-extensions",
		"--disable-plugins",
		"--disable-web-security",
		"--disable-features=VizDisplayCompositor",
		"--start-maximized",
		"--display=:1",
	)

	log.Infof("Starting %s with GUI for CDP on internal port %s (visible via noVNC)", chromeExe, s.chromePort)
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
	s := &cdpServer{
		port:       "3007",                    // External port (hardcoded for external access)
		restAPIURL: "http://127.0.0.1:7000",  // REST API to query VM port mappings
	}

	// NOTE: Chrome should be running inside guest VMs with dynamic port forwarding
	log.Info("CDP server will proxy to Chrome running in guest VMs via dynamic port discovery")

	// Give guest VM time to start Chrome (if needed)
	time.Sleep(2 * time.Second)

	r := mux.NewRouter()
	r.StrictSlash(true) // Automatically handle trailing slashes

	// Register CDP routes
	r.HandleFunc("/health", s.healthCheck).Methods("GET")
	r.HandleFunc("/json/version", s.versionHandler).Methods("GET")
	r.HandleFunc("/json", s.listHandler).Methods("GET")
	r.HandleFunc("/json/list", s.listHandler).Methods("GET")

	// Register WebSocket routes for DevTools (order matters - more specific first)
	r.HandleFunc("/devtools/browser/", s.websocketProxy)
	r.HandleFunc("/devtools/page/{pageId}", s.websocketProxy)
	r.PathPrefix("/devtools/").HandlerFunc(s.websocketProxy) // Catch-all for other DevTools WebSocket endpoints

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
