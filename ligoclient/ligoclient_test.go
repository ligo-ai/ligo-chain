package ligoclient

import "github.com/ligo-ai/ligo-chain"

var (
	_ = ligochain.ChainReader(&Client{})
	_ = ligochain.TransactionReader(&Client{})
	_ = ligochain.ChainStateReader(&Client{})
	_ = ligochain.ChainSyncReader(&Client{})
	_ = ligochain.ContractCaller(&Client{})
	_ = ligochain.GasEstimator(&Client{})
	_ = ligochain.GasPricer(&Client{})
	_ = ligochain.LogFilterer(&Client{})
	_ = ligochain.PendingStateReader(&Client{})

	_ = ligochain.PendingContractCaller(&Client{})
)
