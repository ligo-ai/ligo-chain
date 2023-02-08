package node

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/ligo-ai/ligo-chain//p2p"
	"github.com/ligo-ai/ligo-chain//p2p/nat"
	"github.com/ligo-ai/ligo-chain/network/rpc"
)

const (
	DefaultHTTPHost = "localhost"
	DefaultHTTPPort = 9915
	DefaultWSHost   = "localhost"
	DefaultWSPort   = 9916
)

var DefaultConfig = Config{
	GeneralDataDir:   DefaultDataDir(),
	DataDir:          DefaultDataDir(),
	HTTPPort:         DefaultHTTPPort,
	HTTPModules:      []string{"net", "web3"},
	HTTPVirtualHosts: []string{"localhost"},
	HTTPTimeouts:     rpc.DefaultHTTPTimeouts,
	WSPort:           DefaultWSPort,
	WSModules:        []string{"net", "web3"},
	P2P: p2p.Config{
		ListenAddr: ":9910",
		MaxPeers:   200,
		NAT:        nat.Any(),
	},
}

func DefaultDataDir() string {

	home := homeDir()
	if home != "" {

		if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "ligochain")
		} else {
			return filepath.Join(home, ".ligochain")
		}
	}

	return ""
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}
