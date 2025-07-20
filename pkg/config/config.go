package config

import (
	"fmt"

	"github.com/spf13/viper"
)

const (
	serverConfigKey      = "hostservices.restserver"
	clientConfigKey      = "hostservices.client"
	codeServerConfigKey  = "guestservices.codeserver"
	novncServerConfigKey = "guestservices.novncserver"
	cdpServerConfigKey   = "guestservices.cdpserver"
)

type PortForwardConfig struct {
	Port        string `mapstructure:"port"`
	Description string `mapstructure:"description"`
}

type ServerConfig struct {
	Host               string              `mapstructure:"host"`
	Port               string              `mapstructure:"port"`
	StateDir           string              `mapstructure:"state_dir"`
	BridgeName         string              `mapstructure:"bridge_name"`
	BridgeIP           string              `mapstructure:"bridge_ip"`
	BridgeSubnet       string              `mapstructure:"bridge_subnet"`
	ChvBinPath         string              `mapstructure:"chv_bin"`
	KernelPath         string              `mapstructure:"kernel"`
	RootfsPath         string              `mapstructure:"rootfs"`
	PortForwards       []PortForwardConfig `mapstructure:"port_forwards"`
	InitramfsPath      string              `mapstructure:"initramfs"`
	StatefulSizeInMB   int32               `mapstructure:"stateful_size_in_mb"`
	GuestMemPercentage int32               `mapstructure:"guest_mem_percentage"`
}

func (c ServerConfig) String() string {
	return fmt.Sprintf(`{
Host: %s
Port: %s
StateDir: %s
BridgeName: %s
BridgeIP: %s
BridgeSubnet: %s
KernelPath: %s
ChvBinPath: %s
PortForwards: %+v
InitramfsPath: %s
StatefulSizeInMB: %d
GuestMemPercentage: %d
}`,
		c.Host,
		c.Port,
		c.StateDir,
		c.BridgeName,
		c.BridgeIP,
		c.BridgeSubnet,
		c.KernelPath,
		c.ChvBinPath,
		c.PortForwards,
		c.InitramfsPath,
		c.StatefulSizeInMB,
		c.GuestMemPercentage,
	)
}

type ClientConfig struct {
	ServerHost string `mapstructure:"server_host"`
	ServerPort string `mapstructure:"server_port"`
}

func (c ClientConfig) String() string {
	return fmt.Sprintf(`{
ServerHost: %s
ServerPort: %s
}`, c.ServerHost, c.ServerPort)
}

type CodeServerConfig struct {
	Port string `mapstructure:"port"`
}

func (c CodeServerConfig) String() string {
	return fmt.Sprintf(`{
Port: %s
}`, c.Port)
}

type NoVNCServerConfig struct {
	Port string `mapstructure:"port"`
}

func (c NoVNCServerConfig) String() string {
	return fmt.Sprintf(`{
Port: %s
}`, c.Port)
}

type CDPServerConfig struct {
	Port string `mapstructure:"port"`
}

func (c CDPServerConfig) String() string {
	return fmt.Sprintf(`{
Port: %s
}`, c.Port)
}

func GetServerConfig(configFile string) (*ServerConfig, error) {
	viper.SetConfigFile(configFile)
	err := viper.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}

	restServerConfig := viper.Sub(serverConfigKey)
	if restServerConfig == nil {
		return nil, fmt.Errorf("restserver configuration not found")
	}

	var result ServerConfig
	if err := restServerConfig.Unmarshal(&result); err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %v", err)
	}

	return &result, nil
}

func GetClientConfig(configFile string) (*ClientConfig, error) {
	viper.SetConfigFile(configFile)
	err := viper.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}

	clientConfig := viper.Sub(clientConfigKey)
	if clientConfig == nil {
		return nil, fmt.Errorf("client configuration not found")
	}

	var result ClientConfig
	if err := clientConfig.Unmarshal(&result); err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %v", err)
	}
	return &result, nil
}

func GetCodeServerConfig(configFile string) (*CodeServerConfig, error) {
	viper.SetConfigFile(configFile)
	err := viper.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}

	clientConfig := viper.Sub(clientConfigKey)
	if clientConfig == nil {
		return nil, fmt.Errorf("client configuration not found")
	}

	var result CodeServerConfig
	if err := clientConfig.Unmarshal(&result); err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %v", err)
	}
	return &result, nil
}

func GetNoVNCServerConfig(configFile string) (*NoVNCServerConfig, error) {
	viper.SetConfigFile(configFile)
	err := viper.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}

	novncConfig := viper.Sub(novncServerConfigKey)
	if novncConfig == nil {
		return nil, fmt.Errorf("novnc server configuration not found")
	}

	var result NoVNCServerConfig
	if err := novncConfig.Unmarshal(&result); err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %v", err)
	}
	return &result, nil
}

func GetCDPServerConfig(configFile string) (*CDPServerConfig, error) {
	viper.SetConfigFile(configFile)
	err := viper.ReadInConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}

	cdpConfig := viper.Sub(cdpServerConfigKey)
	if cdpConfig == nil {
		return nil, fmt.Errorf("cdp server configuration not found")
	}

	var result CDPServerConfig
	if err := cdpConfig.Unmarshal(&result); err != nil {
		return nil, fmt.Errorf("error unmarshalling config: %v", err)
	}
	return &result, nil
}
