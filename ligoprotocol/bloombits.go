package ligoprotocol

import (
	"time"

	"github.com/ligo-ai/ligo-chain//core"
	"github.com/ligo-ai/ligo-chain//core/bloombits"
	"github.com/ligo-ai/ligo-chain//core/rawdb"
	"github.com/ligo-ai/ligo-chain//core/types"
	"github.com/ligo-ai/ligo-chain/ligodb"
	"github.com/ligo-ai/ligo-chain/params"
	"github.com/ligo-ai/ligo-chain//common"
	"github.com/ligo-ai/ligo-chain//common/bitutil"
)

const (
	bloomServiceThreads = 16

	bloomFilterThreads = 3

	bloomRetrievalBatch = 16

	bloomRetrievalWait = time.Duration(0)
)

func (ligoChain *LigoAI) startBloomHandlers() {
	for i := 0; i < bloomServiceThreads; i++ {
		go func() {
			for {
				select {
				case <-ligoChain.shutdownChan:
					return

				case request := <-ligoChain.bloomRequests:
					task := <-request
					task.Bitsets = make([][]byte, len(task.Sections))
					for i, section := range task.Sections {
						head := rawdb.ReadCanonicalHash(ligoChain.chainDb, (section+1)*params.BloomBitsBlocks-1)
						if compVector, err := rawdb.ReadBloomBits(ligoChain.chainDb, task.Bit, section, head); err == nil {
							if blob, err := bitutil.DecompressBytes(compVector, int(params.BloomBitsBlocks)/8); err == nil {
								task.Bitsets[i] = blob
							} else {
								task.Error = err
							}
						} else {
							task.Error = err
						}
					}
					request <- task
				}
			}
		}()
	}
}

const (
	bloomConfirms = 256

	bloomThrottling = 100 * time.Millisecond
)

type BloomIndexer struct {
	size uint64

	db  ligodb.Database
	gen *bloombits.Generator

	section uint64
	head    common.Hash
}

func NewBloomIndexer(db ligodb.Database, size uint64) *core.ChainIndexer {
	backend := &BloomIndexer{
		db:   db,
		size: size,
	}
	table := rawdb.NewTable(db, string(rawdb.BloomBitsIndexPrefix))

	return core.NewChainIndexer(db, table, backend, size, bloomConfirms, bloomThrottling, "bloombits")
}

func (b *BloomIndexer) Reset(section uint64, lastSectionHead common.Hash) error {
	gen, err := bloombits.NewGenerator(uint(b.size))
	b.gen, b.section, b.head = gen, section, common.Hash{}
	return err
}

func (b *BloomIndexer) Process(header *types.Header) {
	b.gen.AddBloom(uint(header.Number.Uint64()-b.section*b.size), header.Bloom)
	b.head = header.Hash()
}

func (b *BloomIndexer) Commit() error {
	batch := b.db.NewBatch()
	for i := 0; i < types.BloomBitLength; i++ {
		bits, err := b.gen.Bitset(uint(i))
		if err != nil {
			return err
		}
		rawdb.WriteBloomBits(batch, uint(i), b.section, b.head, bitutil.CompressBytes(bits))
	}
	return batch.Write()
}
