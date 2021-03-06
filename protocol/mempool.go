package protocol

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytom/protocol/bc"
	"github.com/bytom/protocol/bc/legacy"
	"github.com/golang/groupcache/lru"
)

var (
	maxCachedErrTxs        = 1000
	maxNewTxChSize         = 1000
	ErrTransactionNotExist = errors.New("transaction are not existed in the mempool")
)

type TxDesc struct {
	Tx       *legacy.Tx
	Added    time.Time
	Height   uint64
	Weight   uint64
	Fee      uint64
	FeePerKB uint64
}

type TxPool struct {
	lastUpdated int64
	mtx         sync.RWMutex
	pool        map[bc.Hash]*TxDesc
	errCache    *lru.Cache
	newTxCh     chan *legacy.Tx
}

func NewTxPool() *TxPool {
	return &TxPool{
		lastUpdated: time.Now().Unix(),
		pool:        make(map[bc.Hash]*TxDesc),
		errCache:    lru.New(maxCachedErrTxs),
		newTxCh:     make(chan *legacy.Tx, maxNewTxChSize),
	}
}

func (mp *TxPool) GetNewTxCh() chan *legacy.Tx {
	return mp.newTxCh
}

func (mp *TxPool) AddTransaction(tx *legacy.Tx, height, fee uint64) *TxDesc {
	txD := &TxDesc{
		Tx:       tx,
		Added:    time.Now(),
		Weight:   tx.TxData.SerializedSize,
		Height:   height,
		Fee:      fee,
		FeePerKB: fee * 1000 / tx.TxHeader.SerializedSize,
	}

	mp.mtx.Lock()
	defer mp.mtx.Unlock()

	mp.pool[tx.Tx.ID] = txD
	atomic.StoreInt64(&mp.lastUpdated, time.Now().Unix())

	mp.newTxCh <- tx
	return txD
}

func (mp *TxPool) AddErrCache(txHash *bc.Hash, err error) {
	mp.mtx.Lock()
	defer mp.mtx.Unlock()

	mp.errCache.Add(txHash, err)
}

func (mp *TxPool) GetErrCache(txHash *bc.Hash) error {
	mp.mtx.Lock()
	defer mp.mtx.Unlock()

	v, ok := mp.errCache.Get(txHash)
	if !ok {
		return nil
	}
	return v.(error)
}

func (mp *TxPool) RemoveTransaction(txHash *bc.Hash) {
	mp.mtx.Lock()
	defer mp.mtx.Unlock()

	if _, ok := mp.pool[*txHash]; ok {
		delete(mp.pool, *txHash)
		atomic.StoreInt64(&mp.lastUpdated, time.Now().Unix())
	}
}

func (mp *TxPool) GetTransaction(txHash *bc.Hash) (*TxDesc, error) {
	mp.mtx.RLock()
	defer mp.mtx.RUnlock()

	if txD, ok := mp.pool[*txHash]; ok {
		return txD, nil
	}

	return nil, ErrTransactionNotExist
}

func (mp *TxPool) GetTransactions() []*TxDesc {
	mp.mtx.RLock()
	defer mp.mtx.RUnlock()

	txDs := make([]*TxDesc, len(mp.pool))
	i := 0
	for _, desc := range mp.pool {
		txDs[i] = desc
		i++
	}
	return txDs
}

func (mp *TxPool) IsTransactionInPool(txHash *bc.Hash) bool {
	mp.mtx.RLock()
	defer mp.mtx.RUnlock()

	if _, ok := mp.pool[*txHash]; ok {
		return true
	}
	return false
}

func (mp *TxPool) IsTransactionInErrCache(txHash *bc.Hash) bool {
	mp.mtx.RLock()
	defer mp.mtx.RUnlock()

	_, ok := mp.errCache.Get(txHash)
	return ok
}

func (mp *TxPool) HaveTransaction(txHash *bc.Hash) bool {
	return mp.IsTransactionInPool(txHash) || mp.IsTransactionInErrCache(txHash)
}

func (mp *TxPool) Count() int {
	mp.mtx.RLock()
	defer mp.mtx.RUnlock()

	count := len(mp.pool)
	return count
}
