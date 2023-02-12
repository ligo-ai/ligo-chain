package main

import (
	"path/filepath"

	"github.com/ligo-ai/ligo-chain/accounts/keystore"
	tdmTypes "github.com/ligo-ai/ligo-chain/consensus/tendermint/types"
	"github.com/ligo-ai/ligo-chain/log"
	ligonode "github.com/ligo-ai/ligo-chain/node"
	"github.com/ligo-ai/ligo-chain/utils"
	cfg "github.com/ligo-libs/config-go"
	"gopkg.in/urfave/cli.v1"
)

const (
	MainChain    = "ligochain"
	TestnetChain = "testnet"
)

type Chain struct {
	Id       string
	Config   cfg.Config
	LigoNode *ligonode.Node
}

func LoadMainChain(ctx *cli.Context, chainId string) *Chain {

	chain := &Chain{Id: chainId}
	config := utils.GetTendermintConfig(chainId, ctx)
	chain.Config = config

	log.Info("Starting full node...")
	stack := makeFullNode(ctx, GetCMInstance(ctx).cch, chainId)
	chain.LigoNode = stack

	return chain
}

func LoadSideChain(ctx *cli.Context, chainId string) *Chain {

	log.Infof("now load side: %s", chainId)

	chain := &Chain{Id: chainId}
	config := utils.GetTendermintConfig(chainId, ctx)
	chain.Config = config

	log.Infof("chainId: %s, makeFullNode", chainId)
	cch := GetCMInstance(ctx).cch
	stack := makeFullNode(ctx, cch, chainId)
	if stack == nil {
		return nil
	} else {
		chain.LigoNode = stack
		return chain
	}
}

func StartChain(ctx *cli.Context, chain *Chain, startDone chan<- struct{}) error {

	go func() {
		utils.StartNode(ctx, chain.LigoNode)

		if startDone != nil {
			startDone <- struct{}{}
		}
	}()

	return nil
}

func CreateSideChain(ctx *cli.Context, chainId string, validator tdmTypes.PrivValidator, keyJson []byte, validators []tdmTypes.GenesisValidator) error {

	config := utils.GetTendermintConfig(chainId, ctx)

	if len(keyJson) > 0 {
		keystoreDir := config.GetString("keystore")
		keyJsonFilePath := filepath.Join(keystoreDir, keystore.KeyFileName(validator.Address))
		saveKeyError := keystore.WriteKeyStore(keyJsonFilePath, keyJson)
		if saveKeyError != nil {
			return saveKeyError
		}
	}

	privValFile := config.GetString("priv_validator_file_root")
	validator.SetFile(privValFile + ".json")
	validator.Save()

	err := initEthGenesisFromExistValidator(chainId, config, validators)
	if err != nil {
		return err
	}

	init_ligochain(chainId, config.GetString("ligo_genesis_file"), ctx)

	init_em_files(config, chainId, config.GetString("ligo_genesis_file"), validators)

	return nil
}
