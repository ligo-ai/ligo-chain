package rawdb

import (
	"encoding/binary"

	"github.com/ligo-ai/ligo-chain/log"
	"github.com/ligo-ai/ligo-chain/ligodb"
	"github.com/ligo-ai/ligo-chain/common"
)

// ReadDataPruneTrieRootHash retrieves the root hash of a data prune process trie
func ReadDataPruneTrieRootHash(db ligodb.Reader, scan, prune uint64) common.Hash {
	data, _ := db.Get(dataPruneNumberKey(scan, prune))
	if len(data) == 0 {
		return common.Hash{}
	}
	return common.BytesToHash(data)
}

// WriteCanonicalHash stores the hash assigned to a canonical block number.
func WriteDataPruneTrieRootHash(db ligodb.Writer, hash common.Hash, scan, prune uint64) {
	if err := db.Put(dataPruneNumberKey(scan, prune), hash.Bytes()); err != nil {
		log.Crit("Failed to store number to hash mapping", "err", err)
	}
}

// DeleteCanonicalHash removes the number to hash canonical mapping.
func DeleteDataPruneTrieRootHash(db ligodb.Writer, scan, prune uint64) {
	if err := db.Delete(dataPruneNumberKey(scan, prune)); err != nil {
		log.Crit("Failed to delete number to hash mapping", "err", err)
	}
}

// ReadHeadScanNumber retrieves the latest scaned number.
func ReadHeadScanNumber(db ligodb.Reader) *uint64 {
	data, _ := db.Get(headDataScanKey)
	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)
	return &number
}

// WriteHeadScanNumber stores the number of the latest scaned block.
func WriteHeadScanNumber(db ligodb.Writer, scan uint64) {
	if err := db.Put(headDataScanKey, encodeBlockNumber(scan)); err != nil {
		log.Crit("Failed to store last scan number", "err", err)
	}
}

// ReadHeadPruneNumber retrieves the latest pruned number.
func ReadHeadPruneNumber(db ligodb.Reader) *uint64 {
	data, _ := db.Get(headDataPruneKey)
	if len(data) != 8 {
		return nil
	}
	number := binary.BigEndian.Uint64(data)
	return &number
}

// WriteHeadPruneNumber stores the number of the latest pruned block.
func WriteHeadPruneNumber(db ligodb.Writer, prune uint64) {
	if err := db.Put(headDataPruneKey, encodeBlockNumber(prune)); err != nil {
		log.Crit("Failed to store last prune number", "err", err)
	}
}
