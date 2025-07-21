package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/abshkbh/arrakis/out/gen/serverapi"
	"github.com/abshkbh/arrakis/pkg/config"
)

var (
	apiClient *serverapi.APIClient
)

// parseErrorResponse attempts to parse the HTTP response body as an ErrorResponse.
// It returns a formatted error with the error message from the response if successful,
// or falls back to a generic error if parsing fails.
func parseErrorResponse(operation string, httpResp *http.Response, err error) error {
	if httpResp == nil {
		return fmt.Errorf("failed to %s: %v", operation, err)
	}

	defer httpResp.Body.Close()
	body, readErr := io.ReadAll(httpResp.Body)
	if readErr != nil {
		return fmt.Errorf("failed to %s: %v (error reading response body: %v)", operation, err, readErr)
	}

	var errorResp serverapi.ErrorResponse
	if jsonErr := json.Unmarshal(body, &errorResp); jsonErr == nil && errorResp.Error != nil {
		// Successfully parsed ErrorResponse
		return fmt.Errorf("failed to %s: %s (HTTP %d)", operation, errorResp.Error.GetMessage(), httpResp.StatusCode)
	}

	// Couldn't parse as ErrorResponse, use raw body
	return fmt.Errorf("failed to %s: %s (HTTP %d)", operation, string(body), httpResp.StatusCode)
}

func stopVM(vmName string) error {
	req := apiClient.DefaultAPI.V1VmsNamePatch(context.Background(), vmName)

	req = req.V1VmsNamePatchRequest(serverapi.V1VmsNamePatchRequest{
		Status: serverapi.PtrString("stopped"),
	})

	_, httpResp, err := req.Execute()
	if err != nil {
		return parseErrorResponse("stop VM", httpResp, err)
	}

	log.Infof("successfully stopped VM: %s", vmName)
	return nil
}

func destroyVM(vmName string) error {
	_, httpResp, err := apiClient.DefaultAPI.V1VmsNameDelete(context.Background(), vmName).Execute()
	if err != nil {
		return parseErrorResponse("destroy VM", httpResp, err)
	}

	log.Infof("successfully destroyed VM: %s", vmName)
	return nil
}

func destroyAllVMs() error {
	_, httpResp, err := apiClient.DefaultAPI.V1VmsDelete(context.Background()).Execute()
	if err != nil {
		return parseErrorResponse("destroy all VMs", httpResp, err)
	}

	log.Infof("destroyed all VMs")
	return nil
}

func startVM(vmName string, kernel string, rootfs string, entryPoint string, snapshotId string) error {
	var startVMRequest *serverapi.StartVMRequest
	if snapshotId != "" {
		// If snapshot ID is provided, restore the VM from the snapshot
		startVMRequest = &serverapi.StartVMRequest{
			VmName:     serverapi.PtrString(vmName),
			SnapshotId: serverapi.PtrString(snapshotId),
		}
	} else {
		startVMRequest = &serverapi.StartVMRequest{
			VmName:     serverapi.PtrString(vmName),
			Kernel:     serverapi.PtrString(kernel),
			Rootfs:     serverapi.PtrString(rootfs),
			EntryPoint: serverapi.PtrString(entryPoint),
		}
	}

	resp, httpResp, err := apiClient.DefaultAPI.V1VmsPost(context.Background()).StartVMRequest(*startVMRequest).Execute()
	if err != nil {
		return parseErrorResponse("start VM", httpResp, err)
	}

	resp_bytes, err := resp.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}
	log.Infof("started VM: %v", string(resp_bytes))
	return nil
}

func listAllVMs() error {
	resp, httpResp, err := apiClient.DefaultAPI.V1VmsGet(context.Background()).Execute()
	if err != nil {
		return parseErrorResponse("list all VMs", httpResp, err)
	}

	// Format output to better show port forwards with descriptions
	fmt.Println("Available VMs:")
	fmt.Println("-------------")

	for _, vm := range resp.GetVms() {
		fmt.Printf("VM Name: %s\n", vm.GetVmName())
		fmt.Printf("Status: %s\n", vm.GetStatus())
		fmt.Printf("IP Address: %s\n", vm.GetIp())
		fmt.Printf("Tap Device: %s\n", vm.GetTapDeviceName())

		// Print port forwards with descriptions
		if len(vm.GetPortForwards()) > 0 {
			fmt.Println("Port Forwards:")
			for _, pf := range vm.GetPortForwards() {
				fmt.Printf("  %s -> %s: %s\n",
					pf.GetHostPort(),
					pf.GetGuestPort(),
					pf.GetDescription())
			}
		}
		fmt.Println("-------------")
	}

	return nil
}

func createApiClient(serverAddr string, clientConfig *config.ClientConfig) (*serverapi.APIClient, error) {
	host, port, err := net.SplitHostPort(serverAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse server address: %v", err)
	}

	scheme := "http"
	if clientConfig.TLS.Enabled {
		scheme = "https"
	}

	serverConfiguration := &serverapi.ServerConfiguration{
		URL:         fmt.Sprintf("%s://{host}:{port}", scheme),
		Description: "Development server",
		Variables: map[string]serverapi.ServerVariable{
			"host": {
				Description:  "host",
				DefaultValue: host,
			},
			"port": {
				Description:  "port",
				DefaultValue: port,
			},
		},
	}

	configuration := serverapi.NewConfiguration()
	configuration.Servers = serverapi.ServerConfigurations{
		*serverConfiguration,
	}

	// Configure TLS if enabled
	if clientConfig.TLS.Enabled {
		tlsConfig := &tls.Config{}
		
		// Load client certificate if provided
		if clientConfig.TLS.CertFile != "" && clientConfig.TLS.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(clientConfig.TLS.CertFile, clientConfig.TLS.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("failed to load client certificate: %v", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}

		// For self-signed certificates, you might want to add the CA or skip verification
		// WARNING: Only use InsecureSkipVerify for development/testing
		if clientConfig.TLS.CertFile != "" {
			// Try to load the server certificate as a CA
			caCert, err := ioutil.ReadFile(clientConfig.TLS.CertFile)
			if err == nil {
				caCertPool := x509.NewCertPool()
				if caCertPool.AppendCertsFromPEM(caCert) {
					tlsConfig.RootCAs = caCertPool
				}
			}
		}

		// For development with self-signed certificates
		tlsConfig.InsecureSkipVerify = true

		transport := &http.Transport{
			TLSClientConfig: tlsConfig,
		}
		configuration.HTTPClient = &http.Client{
			Transport: transport,
		}
	}

	apiClient = serverapi.NewAPIClient(configuration)

	return apiClient, nil
}

func snapshotVM(vmName string, snapshotId string) error {
	req := serverapi.V1VmsNameSnapshotsPostRequest{}
	if snapshotId != "" {
		req.SetSnapshotId(snapshotId)
	}

	resp, httpResp, err := apiClient.DefaultAPI.V1VmsNameSnapshotsPost(context.Background(), vmName).V1VmsNameSnapshotsPostRequest(req).Execute()
	if err != nil {
		return parseErrorResponse("create snapshot", httpResp, err)
	}
	log.Infof("successfully created snapshot for VM %s with ID %s", vmName, resp.GetSnapshotId())
	return nil
}

func restoreVM(vmName string, snapshotId string) error {
	return startVM(vmName, "", "", "", snapshotId)
}

func pauseVM(vmName string) error {
	req := apiClient.DefaultAPI.V1VmsNamePatch(context.Background(), vmName)
	req = req.V1VmsNamePatchRequest(serverapi.V1VmsNamePatchRequest{
		Status: serverapi.PtrString("paused"),
	})
	_, httpResp, err := req.Execute()
	if err != nil {
		return parseErrorResponse("pause VM", httpResp, err)
	}
	log.Infof("successfully paused VM: %s", vmName)
	return nil
}

func resumeVM(vmName string) error {
	req := apiClient.DefaultAPI.V1VmsNamePatch(context.Background(), vmName)
	req = req.V1VmsNamePatchRequest(serverapi.V1VmsNamePatchRequest{
		Status: serverapi.PtrString("stopped"),
	})
	_, httpResp, err := req.Execute()
	if err != nil {
		return parseErrorResponse("resume VM", httpResp, err)
	}
	log.Infof("successfully resumed VM: %s", vmName)
	return nil
}

func uploadFiles(vmName string, fileSpecs []string) error {
	if len(fileSpecs) == 0 || len(fileSpecs)%2 != 0 {
		return fmt.Errorf("invalid number of file specifications: must be even")
	}

	apiFiles := make([]serverapi.VmFileUploadRequestFilesInner, len(fileSpecs)/2)
	for i := 0; i < len(fileSpecs); i += 2 {
		sourcePath := fileSpecs[i]
		destPath := fileSpecs[i+1]

		log.Infof("uploading file from %s to %s", sourcePath, destPath)

		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %v", sourcePath, err)
		}

		apiFiles[i/2] = serverapi.VmFileUploadRequestFilesInner{
			Path:    destPath,
			Content: string(content),
		}
	}

	req := apiClient.DefaultAPI.V1VmsNameFilesPost(context.Background(), vmName)
	req = req.VmFileUploadRequest(serverapi.VmFileUploadRequest{
		Files: apiFiles,
	})

	_, httpResp, err := req.Execute()
	if err != nil {
		return parseErrorResponse("upload files", httpResp, err)
	}

	log.Infof("successfully uploaded %d files to VM: %s", len(fileSpecs)/2, vmName)
	return nil
}

func runCommand(vmName string, cmd string) error {
	req := serverapi.VmCommandRequest{
		Cmd:      cmd,
		Blocking: serverapi.PtrBool(true),
	}

	resp, httpResp, err := apiClient.DefaultAPI.V1VmsNameCmdPost(context.Background(), vmName).VmCommandRequest(req).Execute()
	if err != nil {
		return parseErrorResponse("run command", httpResp, err)
	}

	if resp.GetError() != "" {
		return fmt.Errorf("command failed: %s\nOutput: %s", resp.GetError(), resp.GetOutput())
	}
	fmt.Println(resp.GetOutput())
	return nil
}

func downloadFiles(vmName string, paths []string) error {
	pathsStr := strings.Join(paths, ",")
	resp, httpResp, err := apiClient.DefaultAPI.V1VmsNameFilesGet(context.Background(), vmName).Paths(pathsStr).Execute()
	if err != nil {
		return parseErrorResponse("download files", httpResp, err)
	}

	for _, file := range resp.GetFiles() {
		log.Infof("Downloaded file: %s", file.GetPath())
		fmt.Printf("Content: %s\n", file.GetContent())
	}
	return nil
}

func listVM(vmName string) error {
	resp, httpResp, err := apiClient.DefaultAPI.V1VmsNameGet(context.Background(), vmName).Execute()
	if err != nil {
		return parseErrorResponse("get VM info", httpResp, err)
	}

	fmt.Printf("VM Name: %s\n", resp.GetVmName())
	fmt.Printf("Status: %s\n", resp.GetStatus())
	fmt.Printf("IP Address: %s\n", resp.GetIp())
	fmt.Printf("Tap Device: %s\n", resp.GetTapDeviceName())

	// Print port forwards with descriptions
	if len(resp.GetPortForwards()) > 0 {
		fmt.Println("Port Forwards:")
		for _, pf := range resp.GetPortForwards() {
			fmt.Printf("  %s -> %s: %s\n",
				pf.GetHostPort(),
				pf.GetGuestPort(),
				pf.GetDescription())
		}
	}

	return nil
}

func main() {
	app := &cli.App{
		Name:  "arrakis-client",
		Usage: "A CLI for managing VMs",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to config file",
				Value:   "./config.yaml",
			},
		},
		Before: func(ctx *cli.Context) error {
			configPath := ctx.String("config")
			clientConfig, err := config.GetClientConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to get client config: %v", err)
			}
			log.Infof("client config: %v", clientConfig)

			apiClient, err = createApiClient(
				fmt.Sprintf("%s:%s", clientConfig.ServerHost, clientConfig.ServerPort),
				clientConfig,
			)
			if err != nil {
				return fmt.Errorf("failed to initialize api client: %v", err)
			}
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:  "start",
				Usage: "Start a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM to create",
						Required: true,
					},
					&cli.StringFlag{
						Name:    "kernel",
						Aliases: []string{"k"},
						Usage:   "Path of the kernel image to be used",
					},
					&cli.StringFlag{
						Name:    "rootfs",
						Aliases: []string{"r"},
						Usage:   "Path of the rootfs image to be used",
					},
					&cli.StringFlag{
						Name:     "entry-point",
						Aliases:  []string{"e"},
						Usage:    "Entry point of the VM",
						Required: false,
					},
					&cli.StringFlag{
						Name:    "snapshot",
						Aliases: []string{"s"},
						Usage:   "Path to snapshot directory to restore from",
					},
				},
				Action: func(ctx *cli.Context) error {
					return startVM(
						ctx.String("name"),
						ctx.String("kernel"),
						ctx.String("rootfs"),
						ctx.String("entry-point"),
						ctx.String("snapshot"),
					)
				},
			},
			{
				Name:  "stop",
				Usage: "Stop a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM to stop",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return stopVM(ctx.String("name"))
				},
			},
			{
				Name:  "destroy",
				Usage: "Destroy a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM to destroy",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return destroyVM(ctx.String("name"))
				},
			},
			{
				Name:  "destroy-all",
				Usage: "Destroy all VMs",
				Action: func(ctx *cli.Context) error {
					return destroyAllVMs()
				},
			},
			{
				Name:  "list-all",
				Usage: "List all VMs",
				Action: func(ctx *cli.Context) error {
					return listAllVMs()
				},
			},
			{
				Name:  "list",
				Usage: "List VM info",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM to destroy",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return listVM(ctx.String("name"))
				},
			},
			{
				Name:  "snapshot",
				Usage: "Create a snapshot of a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "id",
						Aliases:  []string{"i"},
						Usage:    "Unique identifier for the snapshot",
						Required: false,
					},
				},
				Action: func(ctx *cli.Context) error {
					return snapshotVM(ctx.String("name"), ctx.String("id"))
				},
			},
			{
				Name:  "restore",
				Usage: "Restore a VM from a snapshot",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name to give to the restored VM",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "id",
						Aliases:  []string{"i"},
						Usage:    "ID of the snapshot to restore from",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return restoreVM(ctx.String("name"), ctx.String("id"))
				},
			},
			{
				Name:  "pause",
				Usage: "Pause a running VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM to pause",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return pauseVM(ctx.String("name"))
				},
			},
			{
				Name:  "resume",
				Usage: "Resume a paused VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM to resume",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return resumeVM(ctx.String("name"))
				},
			},
			{
				Name:  "upload",
				Usage: "Upload files to a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM",
						Required: true,
					},
					&cli.StringSliceFlag{
						Name:     "file",
						Aliases:  []string{"f"},
						Usage:    "File(s) to upload in format 'source,destination' (can be specified multiple times)",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return uploadFiles(ctx.String("name"), ctx.StringSlice("file"))
				},
			},
			{
				Name:  "run",
				Usage: "Run a command in a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "cmd",
						Aliases:  []string{"c"},
						Usage:    "Command to run",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return runCommand(ctx.String("name"), ctx.String("cmd"))
				},
			},
			{
				Name:  "download",
				Usage: "Download files from a VM",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Name of the VM",
						Required: true,
					},
					&cli.StringSliceFlag{
						Name:     "path",
						Aliases:  []string{"p"},
						Usage:    "Path(s) to download (can be specified multiple times)",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					return downloadFiles(ctx.String("name"), ctx.StringSlice("path"))
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
