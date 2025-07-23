package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
// If vmName is provided, it looks for that specific VM. Otherwise, returns the first available VM.
func (s *cdpServer) discoverCDPPort(vmName string) (string, VM, error) {
	resp, err := http.Get(s.restAPIURL + "/v1/vms")
	if err != nil {
		return "", VM{}, fmt.Errorf("failed to query VM API: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", VM{}, fmt.Errorf("failed to read response: %v", err)
	}

	log.Debugf("VM API response: %s", string(body))

	var vmResponse VMResponse
	if err := json.Unmarshal(body, &vmResponse); err != nil {
		return "", VM{}, fmt.Errorf("failed to parse VM response: %v", err)
	}

	log.Infof("Found %d VMs in response", len(vmResponse.VMs))

	// Find the requested VM or first running VM with CDP port forwarding
	for _, vm := range vmResponse.VMs {
		log.Infof("Checking VM '%s' with status '%s'", vm.VMName, vm.Status)
		if vm.Status == "RUNNING" {
			// If specific VM requested, skip others
			if vmName != "" && vm.VMName != vmName {
				continue
			}
			
			log.Infof("VM '%s' has %d port forwards", vm.VMName, len(vm.PortForwards))
			for _, pf := range vm.PortForwards {
				log.Debugf("Port forward: guest:%s -> host:%s (%s)", pf.GuestPort, pf.HostPort, pf.Description)
				if pf.GuestPort == "9223" && pf.Description == "cdp" {
					log.Infof("Found running VM '%s' with CDP port forwarded from guest:%s to host:%s", 
						vm.VMName, pf.GuestPort, pf.HostPort)
					return pf.HostPort, vm, nil
				}
			}
		}
	}

	if vmName != "" {
		return "", VM{}, fmt.Errorf("VM '%s' not found or not running with CDP", vmName)
	}
	return "", VM{}, fmt.Errorf("no running VM found with CDP port forwarding")
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// WebSocket proxy handler for DevTools connections
func (s *cdpServer) websocketProxy(w http.ResponseWriter, r *http.Request, hostPort string, vm VM) {
	log.Infof("WebSocket connection request: %s", r.URL.Path)
	
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
		// Remove vm parameter from forwarded query string
		values := r.URL.Query()
		values.Del("vm")
		if len(values) > 0 {
			targetPath += "?" + values.Encode()
		}
	}

	// Connect directly to guest IP instead of using host port forwarding
	// Since cloud-hypervisor isn't listening on host ports, we'll connect directly to guest
	guestIP := vm.Network.GuestIP
	chromeURL := fmt.Sprintf("ws://%s:9223%s", guestIP, targetPath)
	log.Infof("Proxying WebSocket directly to guest Chrome: %s", chromeURL)

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

// proxyHandler handles all CDP requests and proxies them to the appropriate VM
func (s *cdpServer) proxyHandler(w http.ResponseWriter, r *http.Request) {
	// Extract VM name from URL path if present
	var vmName string
	vars := mux.Vars(r)
	if name, exists := vars["vmName"]; exists {
		vmName = name
	}
	
	// Also check for VM in query parameters
	if vmQuery := r.URL.Query().Get("vm"); vmQuery != "" {
		vmName = vmQuery
	}

	// Discover the CDP port for the VM
	hostPort, vm, err := s.discoverCDPPort(vmName)
	if err != nil {
		log.Errorf("Failed to discover CDP port: %v", err)
		http.Error(w, fmt.Sprintf("503 Service Unavailable - %v", err), http.StatusServiceUnavailable)
		return
	}

	if vmName != "" {
		log.Infof("Proxying request to VM '%s' directly at guest IP %s", vmName, vm.Network.GuestIP)
	} else {
		log.Infof("Proxying request to first available VM directly at guest IP %s", vm.Network.GuestIP)
	}

	// Handle WebSocket upgrade
	if websocket.IsWebSocketUpgrade(r) {
		s.websocketProxy(w, r, hostPort, vm)
		return
	}

	// Handle HTTP requests - connect directly to guest IP instead of host port
	targetURL := fmt.Sprintf("http://%s:9223%s", vm.Network.GuestIP, r.URL.Path)
	if r.URL.RawQuery != "" {
		// Remove vm parameter from forwarded query string
		values := r.URL.Query()
		values.Del("vm")
		if len(values) > 0 {
			targetURL += "?" + values.Encode()
		}
	}

	// Create the proxy request
	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		log.Errorf("Failed to create proxy request: %v", err)
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for name, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(name, value)
		}
	}

	// Perform the request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Errorf("Failed to proxy request to %s: %v", targetURL, err)
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	// Copy status code and body
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Errorf("Failed to copy response body: %v", err)
	}
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
		port:       cdpConfig.Port,            // Use configured port (from config.yaml)
		restAPIURL: "http://127.0.0.1:7000",  // REST API to query VM port mappings
	}

	// NOTE: Chrome should be running inside guest VMs with dynamic port forwarding
	log.Info("CDP server will proxy to Chrome running in guest VMs via dynamic port discovery")

	// Give guest VM time to start Chrome (if needed)
	time.Sleep(2 * time.Second)

	r := mux.NewRouter()
	r.StrictSlash(true) // Automatically handle trailing slashes

	// Register CDP routes with VM selection support
	r.HandleFunc("/health", s.healthCheck).Methods("GET")
	
	// VM-specific routes (e.g., /vm/testsandbox/json/version)
	r.HandleFunc("/vm/{vmName}/json/version", s.proxyHandler).Methods("GET")
	r.HandleFunc("/vm/{vmName}/json", s.proxyHandler).Methods("GET")
	r.HandleFunc("/vm/{vmName}/json/list", s.proxyHandler).Methods("GET")
	r.PathPrefix("/vm/{vmName}/devtools/").HandlerFunc(s.proxyHandler)
	
	// Default routes (first available VM)
	r.HandleFunc("/json/version", s.proxyHandler).Methods("GET")
	r.HandleFunc("/json", s.proxyHandler).Methods("GET")
	r.HandleFunc("/json/list", s.proxyHandler).Methods("GET")
	r.PathPrefix("/devtools/").HandlerFunc(s.proxyHandler)

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
