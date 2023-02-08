package main

import (
	"crypto/rand"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ligo-ai/ligo-chain/params"
)

const (
	ipcAPIs  = "admin:1.0 debug:1.0 eth:1.0 miner:1.0 net:1.0 personal:1.0 rpc:1.0 shh:1.0 txpool:1.0 web3:1.0"
	httpAPIs = "eth:1.0 net:1.0 rpc:1.0 web3:1.0"
)

func TestConsoleWelcome(t *testing.T) {
	coinbase := "0x8605cdbbdb6d264aa742e77020dcbc58fcdce182"

	ligochain := runligochain(t,
		"--port", "0", "--maxpeers", "0", "--nodiscover", "--nat", "none",
		"--etherbase", coinbase, "--shh",
		"console")

	ligochain.SetTemplateFunc("goos", func() string { return runtime.GOOS })
	ligochain.SetTemplateFunc("goarch", func() string { return runtime.GOARCH })
	ligochain.SetTemplateFunc("gover", runtime.Version)
	ligochain.SetTemplateFunc("ligover", func() string { return params.Version })
	ligochain.SetTemplateFunc("niltime", func() string { return time.Unix(0, 0).Format(time.RFC1123) })
	ligochain.SetTemplateFunc("apis", func() string { return ipcAPIs })

	ligochain.Expect(`
Welcome to the ligochain JavaScript console!

instance: ligochain/v{{ligover}}/{{goos}}-{{goarch}}/{{gover}}
coinbase: {{.Etherbase}}
at block: 0 ({{niltime}})
 datadir: {{.Datadir}}
 modules: {{apis}}

> {{.InputLine "exit"}}
`)
	ligochain.ExpectExit()
}

func TestIPCAttachWelcome(t *testing.T) {
	coinbase := "0x8605cdbbdb6d264aa742e77020dcbc58fcdce182"
	var ipc string
	if runtime.GOOS == "windows" {
		ipc = `\\.\pipe\ligochain` + strconv.Itoa(trulyRandInt(100000, 999999))
	} else {
		ws := tmpdir(t)
		defer os.RemoveAll(ws)
		ipc = filepath.Join(ws, "ligochain.ipc")
	}
	ligochain := runligochain(t,
		"--port", "0", "--maxpeers", "0", "--nodiscover", "--nat", "none",
		"--etherbase", coinbase, "--shh", "--ipcpath", ipc)

	time.Sleep(2 * time.Second)
	testAttachWelcome(t, ligochain, "ipc:"+ipc, ipcAPIs)

	ligochain.Interrupt()
	ligochain.ExpectExit()
}

func TestHTTPAttachWelcome(t *testing.T) {
	coinbase := "0x8605cdbbdb6d264aa742e77020dcbc58fcdce182"
	port := strconv.Itoa(trulyRandInt(1024, 65536))
	ligochain := runligochain(t,
		"--port", "0", "--maxpeers", "0", "--nodiscover", "--nat", "none",
		"--etherbase", coinbase, "--rpc", "--rpcport", port)

	time.Sleep(2 * time.Second)
	testAttachWelcome(t, ligochain, "http://localhost:"+port, httpAPIs)

	ligochain.Interrupt()
	ligochain.ExpectExit()
}

func TestWSAttachWelcome(t *testing.T) {
	coinbase := "0x8605cdbbdb6d264aa742e77020dcbc58fcdce182"
	port := strconv.Itoa(trulyRandInt(1024, 65536))

	ligochain := runligochain(t,
		"--port", "0", "--maxpeers", "0", "--nodiscover", "--nat", "none",
		"--etherbase", coinbase, "--ws", "--wsport", port)

	time.Sleep(2 * time.Second)
	testAttachWelcome(t, ligochain, "ws://localhost:"+port, httpAPIs)

	ligochain.Interrupt()
	ligochain.ExpectExit()
}

func testAttachWelcome(t *testing.T, ligochain *testligochain, endpoint, apis string) {
	attach := runligochain(t, "attach", endpoint)
	defer attach.ExpectExit()
	attach.CloseStdin()

	attach.SetTemplateFunc("goos", func() string { return runtime.GOOS })
	attach.SetTemplateFunc("goarch", func() string { return runtime.GOARCH })
	attach.SetTemplateFunc("gover", runtime.Version)
	attach.SetTemplateFunc("ligover", func() string { return params.Version })
	attach.SetTemplateFunc("etherbase", func() string { return ligochain.Etherbase })
	attach.SetTemplateFunc("niltime", func() string { return time.Unix(0, 0).Format(time.RFC1123) })
	attach.SetTemplateFunc("ipc", func() bool { return strings.HasPrefix(endpoint, "ipc") })
	attach.SetTemplateFunc("datadir", func() string { return ligochain.Datadir })
	attach.SetTemplateFunc("apis", func() string { return apis })

	attach.Expect(`
Welcome to the ligochain JavaScript console!

instance: ligochain/v{{ligover}}/{{goos}}-{{goarch}}/{{gover}}
coinbase: {{etherbase}}
at block: 0 ({{niltime}}){{if ipc}}
 datadir: {{datadir}}{{end}}
 modules: {{apis}}

> {{.InputLine "exit" }}
`)
	attach.ExpectExit()
}

func trulyRandInt(lo, hi int) int {
	num, _ := rand.Int(rand.Reader, big.NewInt(int64(hi-lo)))
	return int(num.Int64()) + lo
}
