package utils

import (
	"github.com/ligo-ai/ligo-chain/node"
	"github.com/ligo-ai/ligo-chain/p2p"
	"github.com/ligo-ai/ligo-chain/common"
	"gopkg.in/urfave/cli.v1"
)

type LigoChainP2PServer struct {
	serverConfig p2p.Config
	server       *p2p.Server
}

func NewP2PServer(ctx *cli.Context) *LigoChainP2PServer {

	config := &node.Config{
		GeneralDataDir: MakeDataDir(ctx),
		DataDir:        MakeDataDir(ctx),
		P2P:            node.DefaultConfig.P2P,
	}

	SetP2PConfig(ctx, &config.P2P)

	serverConfig := config.P2P
	serverConfig.PrivateKey = config.NodeKey()
	serverConfig.Name = config.NodeName()
	serverConfig.EnableMsgEvents = true

	if serverConfig.StaticNodes == nil {
		serverConfig.StaticNodes = config.StaticNodes()
	}
	if serverConfig.TrustedNodes == nil {
		serverConfig.TrustedNodes = config.TrustedNodes()
	}
	if serverConfig.NodeDatabase == "" {
		serverConfig.NodeDatabase = config.NodeDB()
	}
	serverConfig.LocalValidators = make([]p2p.P2PValidator, 0)
	serverConfig.Validators = make(map[p2p.P2PValidator]*p2p.P2PValidatorNodeInfo)

	running := &p2p.Server{Config: serverConfig}

	return &LigoChainP2PServer{
		serverConfig: serverConfig,
		server:       running,
	}
}

func (srv *LigoChainP2PServer) Server() *p2p.Server {
	return srv.server
}

func (srv *LigoChainP2PServer) Stop() {
	srv.server.Stop()
}

func (srv *LigoChainP2PServer) BroadcastNewSideChainMsg(sideId string) {
	srv.server.BroadcastMsg(p2p.BroadcastNewSideChainMsg, sideId)
}

func (srv *LigoChainP2PServer) AddLocalValidator(chainId string, address common.Address) {
	srv.server.AddLocalValidator(chainId, address)
}

func (srv *LigoChainP2PServer) RemoveLocalValidator(chainId string, address common.Address) {
	srv.server.RemoveLocalValidator(chainId, address)
}
