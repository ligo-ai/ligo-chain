// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"github.com/ligo-ai/ligo-chain/core/types"
	"github.com/ligo-ai/ligo-chain/common"
)

// TxPreEvent is posted when a transaction enters the transaction pool.
type TxPreEvent struct{ Tx *types.Transaction }

//Tx3ProofDataEvent is posted when a tx3ProofData enters
type Tx3ProofDataEvent struct{ Tx3PrfDt *types.TX3ProofData }

// PendingLogsEvent is posted pre mining and notifies of pending logs.
type PendingLogsEvent struct {
	Logs []*types.Log
}

// PendingStateEvent is posted pre mining and notifies of pending state changes.
type PendingStateEvent struct{}

// NewMinedBlockEvent is posted when a block has been imported.
type NewMinedBlockEvent struct{ Block *types.Block }

// RemovedTransactionEvent is posted when a reorg happens
type RemovedTransactionEvent struct{ Txs types.Transactions }

// RemovedLogsEvent is posted when a reorg happens
type RemovedLogsEvent struct{ Logs []*types.Log }

type ChainEvent struct {
	Block *types.Block
	Hash  common.Hash
	Logs  []*types.Log
}

type ChainSideEvent struct {
	Block *types.Block
}

type ChainHeadEvent struct{ Block *types.Block }

// Create Side Chain Event
type CreateSideChainEvent struct {
	ChainId string
}

// Start Mining Event
type StartMiningEvent struct{}

// Stop Mining Event
type StopMiningEvent struct{}
