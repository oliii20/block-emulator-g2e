package chain

import (
	"math/big"
	"sync"
)

type betEntry struct {
	Player string
	Pair   [2]uint64
	Amount *big.Int
}

type g2eEpochBook struct {
	ByPair    map[[2]uint64][]betEntry
	SumByPair map[[2]uint64]*big.Int
}

type G2EBook struct {
	mu   sync.RWMutex
	data map[uint64]*g2eEpochBook // key: TargetH
}

func NewG2EBook() *G2EBook {
	return &G2EBook{data: make(map[uint64]*g2eEpochBook)}
}

func (bk *G2EBook) appendBet(targetH uint64, pair [2]uint64, player string, amt *big.Int) {
	bk.mu.Lock()
	defer bk.mu.Unlock()
	eb, ok := bk.data[targetH]
	if !ok {
		eb = &g2eEpochBook{ByPair: map[[2]uint64][]betEntry{}, SumByPair: map[[2]uint64]*big.Int{}}
		bk.data[targetH] = eb
	}
	eb.ByPair[pair] = append(eb.ByPair[pair], betEntry{Player: player, Pair: pair, Amount: new(big.Int).Set(amt)})
	if _, ok := eb.SumByPair[pair]; !ok {
		eb.SumByPair[pair] = new(big.Int)
	}
	eb.SumByPair[pair].Add(eb.SumByPair[pair], amt)
}

func (bk *G2EBook) getEpoch(targetH uint64) (*g2eEpochBook, bool) {
	bk.mu.RLock()
	defer bk.mu.RUnlock()
	eb, ok := bk.data[targetH]
	return eb, ok
}

func (bk *G2EBook) cleanup(targetH uint64) {
	bk.mu.Lock()
	defer bk.mu.Unlock()
	delete(bk.data, targetH)
}
