// Here the blockchain structrue is defined
// each node in this system will maintain a blockchain object.

package chain

import (
	"blockEmulator/core"
	"blockEmulator/params"
	"blockEmulator/storage"
	"blockEmulator/utils"
	"bytes"
	"errors"
	"fmt"
	"log"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/bits-and-blooms/bitset"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/trie"
)

type BlockChain struct {
	db           ethdb.Database      // the leveldb database to store in the disk, for status trie
	triedb       *trie.Database      // the trie database which helps to store the status trie
	ChainConfig  *params.ChainConfig // the chain configuration, which can help to identify the chain
	CurrentBlock *core.Block         // the top block in this blockchain
	Storage      *storage.Storage    // Storage is the bolt-db to store the blocks
	Txpool       *core.TxPool        // the transaction pool
	PartitionMap map[string]uint64   // the partition map which is defined by some algorithm can help account parition
	pmlock       sync.RWMutex

	// --- Game2Earn ---
	G2EBook *G2EBook
}

// Get the transaction root, this root can be used to check the transactions
func GetTxTreeRoot(txs []*core.Transaction) []byte {
	// use a memory trie database to do this, instead of disk database
	triedb := trie.NewDatabase(rawdb.NewMemoryDatabase())
	transactionTree := trie.NewEmpty(triedb)
	for _, tx := range txs {
		transactionTree.Update(tx.TxHash, []byte{0}) // 插入交易
	}
	return transactionTree.Hash().Bytes()
}

// Get bloom filter //布隆过滤器，能快速判断一个元素是否在一个集合中
func GetBloomFilter(txs []*core.Transaction) *bitset.BitSet {
	bs := bitset.New(2048) //2048 bits
	for _, tx := range txs {
		bs.Set(utils.ModBytes(tx.TxHash, 2048))
	}
	return bs
}

// Write Partition Map
func (bc *BlockChain) Update_PartitionMap(key string, val uint64) {
	bc.pmlock.Lock()
	defer bc.pmlock.Unlock()
	bc.PartitionMap[key] = val
}

// Get parition (if not exist, return default)
func (bc *BlockChain) Get_PartitionMap(key string) uint64 {
	bc.pmlock.RLock()
	defer bc.pmlock.RUnlock()
	if _, ok := bc.PartitionMap[key]; !ok {
		return uint64(utils.Addr2Shard(key))
	}
	return bc.PartitionMap[key]
}

// Send a transaction to the pool (need to decide which pool should be sended)
func (bc *BlockChain) SendTx2Pool(txs []*core.Transaction) {
	bc.Txpool.AddTxs2Pool(txs)
}

// handle transactions and modify the status trie
func (bc *BlockChain) GetUpdateStatusTrie(txs []*core.Transaction) common.Hash {
	fmt.Printf("The len of txs is %d\n", len(txs))
	// the empty block (length of txs is 0) condition
	if len(txs) == 0 {
		return common.BytesToHash(bc.CurrentBlock.Header.StateRoot) // 转为common.Hash，固定32字节的哈希
	}
	// build trie from the triedb (in disk)
	st, err := trie.New(trie.TrieID(common.BytesToHash(bc.CurrentBlock.Header.StateRoot)), bc.triedb)
	if err != nil {
		log.Panic(err)
	}
	cnt := 0
	// handle transactions, the signature check is ignored here
	for i, tx := range txs {
		// fmt.Printf("tx %d: %s, %s\n", i, tx.Sender, tx.Recipient)
		// senderIn := false
		if !tx.Relayed && (bc.Get_PartitionMap(tx.Sender) == bc.ChainConfig.ShardID || tx.HasBroker) {
			// senderIn = true
			// fmt.Printf("the sender %s is in this shard %d, \n", tx.Sender, bc.ChainConfig.ShardID)
			// modify local accountstate
			s_state_enc, _ := st.Get([]byte(tx.Sender)) // 根据地址获取账户状态的编码
			var s_state *core.AccountState
			if s_state_enc == nil {
				// fmt.Println("missing account SENDER, now adding account")
				ib := new(big.Int)
				ib.Add(ib, params.Init_Balance) // 初始余额设置的很大
				s_state = &core.AccountState{
					Nonce:   uint64(i),
					Balance: ib, // 账户状态无存储账户地址
				}
			} else {
				s_state = core.DecodeAS(s_state_enc)
			}
			s_balance := s_state.Balance
			if s_balance.Cmp(tx.Value) == -1 {
				fmt.Printf("the balance is less than the transfer amount\n")
				continue
			}
			s_state.Deduct(tx.Value)
			st.Update([]byte(tx.Sender), s_state.Encode())
			cnt++

			// 2) 如果是下注交易：写入下注账本，并“跳过”接收方入账
			if tx.G2EEnabled {
				meta := tx
				// 关闭窗口检查：nowH <= targetH - beta
				curH := bc.CurrentBlock.Header.Number
				if meta.Height < uint64(curH)+uint64(params.G2E_Beta) {
					// 目标过近 → 本交易无效（根据需要：要么回滚扣款，要么直接跳过）
					// 这里演示直接回滚扣款并跳过
					s_state.Deposit(tx.Value)
					st.Update([]byte(tx.Sender), s_state.Encode())
					cnt--
					continue
				}
				// amt := new(big.Int)
				// amt.SetString(meta.Value, 10)
				arr := [2]uint64{meta.PairI, meta.PairJ}
				bc.G2EBook.appendBet(meta.Height, arr, meta.Sender, meta.Value)

				// 跳过接收方入账（下注不产生即时收入）
				continue

			}
		}

		// recipientIn := false
		if bc.Get_PartitionMap(tx.Recipient) == bc.ChainConfig.ShardID || tx.HasBroker {
			// fmt.Printf("the recipient %s is in this shard %d, \n", tx.Recipient, bc.ChainConfig.ShardID)
			// recipientIn = true
			// modify local state
			r_state_enc, _ := st.Get([]byte(tx.Recipient))
			var r_state *core.AccountState
			if r_state_enc == nil {
				// fmt.Println("missing account RECIPIENT, now adding account")
				ib := new(big.Int)
				ib.Add(ib, params.Init_Balance)
				r_state = &core.AccountState{
					Nonce:   uint64(i),
					Balance: ib,
				}
			} else {
				r_state = core.DecodeAS(r_state_enc)
			}
			r_state.Deposit(tx.Value)
			st.Update([]byte(tx.Recipient), r_state.Encode())
			cnt++
		}
	}
	// commit the memory trie to the database in the disk
	if cnt == 0 {
		return common.BytesToHash(bc.CurrentBlock.Header.StateRoot)
	}

	// --- Game2Earn 清算：如果“下一个区块号 == 某 targetH”，则执行清算进入状态根 ---
	nextBlockH := bc.CurrentBlock.Header.Number + 1
	if eb, ok := bc.G2EBook.getEpoch(nextBlockH); ok {
		// 选择获胜对（两种任选一种，默认简单确定性策略）
		pairStar := chooseWinningPair(nextBlockH, eb) // 函数见下

		// 计算 W, L
		W := new(big.Int)
		L := new(big.Int)
		for pair, sum := range eb.SumByPair {
			if pair == pairStar {
				W.Add(W, sum)
			} else {
				L.Add(L, sum)
			}
		}
		// q = L / W（定点或整数截断均可；演示用整数截断）
		q := new(big.Int)
		if W.Sign() > 0 {
			q.Div(L, W)
		} else {
			q.SetUint64(0)
		}

		// 赢家分润（输家已在下注时扣款，不返还）
		if W.Sign() > 0 {
			for _, e := range eb.ByPair[pairStar] {
				// gain = e.Amount * q
				gain := new(big.Int).Mul(new(big.Int).Set(e.Amount), q)
				// 赢家账户入账
				r_state_enc, _ := st.Get([]byte(e.Player))
				var r_state *core.AccountState
				if r_state_enc == nil {
					ib := new(big.Int)
					ib.Add(ib, params.Init_Balance) // 或者 0，视你的系统初始化而定
					r_state = &core.AccountState{Nonce: 0, Balance: ib}
				} else {
					r_state = core.DecodeAS(r_state_enc)
				}
				r_state.Deposit(gain)
				st.Update([]byte(e.Player), r_state.Encode())
				cnt++
			}
		}
		// 本高度清算完毕，清理账本
		bc.G2EBook.cleanup(uint64(nextBlockH))
	}

	rt, ns := st.Commit(false)
	// if `ns` is nil, the `err = bc.triedb.Update(trie.NewWithNodeSet(ns))` will report an error.
	if ns != nil {
		err = bc.triedb.Update(trie.NewWithNodeSet(ns))
		if err != nil {
			log.Panic()
		}
		err = bc.triedb.Commit(rt, false)
		if err != nil {
			log.Panic(err)
		}
	}
	fmt.Println("modified account number is ", cnt)
	return rt
}

// generate (mine) a block, this function return a block
func (bc *BlockChain) GenerateBlock(miner int32) *core.Block {
	var txs []*core.Transaction
	// pack the transactions from the txpool
	if params.UseBlocksizeInBytes == 1 {
		txs = bc.Txpool.PackTxsWithBytes(params.BlocksizeInBytes) // 按字节数打包交易
	} else {
		txs = bc.Txpool.PackTxs(bc.ChainConfig.BlockSize) // 按交易数打包交易
	}

	bh := &core.BlockHeader{
		ParentBlockHash: bc.CurrentBlock.Hash,
		Number:          bc.CurrentBlock.Header.Number + 1,
		Time:            time.Now(),
	}
	// handle transactions to build root
	rt := bc.GetUpdateStatusTrie(txs)

	bh.StateRoot = rt.Bytes()
	bh.TxRoot = GetTxTreeRoot(txs)
	bh.Bloom = *GetBloomFilter(txs)
	bh.Miner = miner
	b := core.NewBlock(bh, txs)

	b.Hash = b.Header.Hash()
	return b
}

// new a genisis block, this func will be invoked only once for a blockchain object
func (bc *BlockChain) NewGenisisBlock() *core.Block {
	body := make([]*core.Transaction, 0)
	bh := &core.BlockHeader{
		Number: 0,
	}
	// build a new trie database by db
	triedb := trie.NewDatabaseWithConfig(bc.db, &trie.Config{
		Cache:     0,
		Preimages: true,
	})
	bc.triedb = triedb
	statusTrie := trie.NewEmpty(triedb)
	bh.StateRoot = statusTrie.Hash().Bytes()
	bh.TxRoot = GetTxTreeRoot(body)
	bh.Bloom = *GetBloomFilter(body)
	b := core.NewBlock(bh, body)
	b.Hash = b.Header.Hash()
	return b
}

// add the genisis block in a blockchain
func (bc *BlockChain) AddGenisisBlock(gb *core.Block) {
	bc.Storage.AddBlock(gb)
	newestHash, err := bc.Storage.GetNewestBlockHash()
	if err != nil {
		log.Panic()
	}
	curb, err := bc.Storage.GetBlock(newestHash)
	if err != nil {
		log.Panic()
	}
	bc.CurrentBlock = curb
}

// add a block
func (bc *BlockChain) AddBlock(b *core.Block) {
	if b.Header.Number != bc.CurrentBlock.Header.Number+1 {
		fmt.Println("the block height is not correct")
		return
	}

	if !bytes.Equal(b.Header.ParentBlockHash, bc.CurrentBlock.Hash) {
		fmt.Println("err parent block hash")
		return
	}

	// if the treeRoot is existed in the node, the transactions is no need to be handled again
	_, err := trie.New(trie.TrieID(common.BytesToHash(b.Header.StateRoot)), bc.triedb)
	if err != nil {
		rt := bc.GetUpdateStatusTrie(b.Body)
		fmt.Println(bc.CurrentBlock.Header.Number+1, "the root = ", rt.Bytes())
	}
	bc.CurrentBlock = b
	bc.Storage.AddBlock(b)
}

// new a blockchain.
// the ChainConfig is pre-defined to identify the blockchain; the db is the status trie database in disk
func NewBlockChain(cc *params.ChainConfig, db ethdb.Database) (*BlockChain, error) {
	fmt.Println("Generating a new blockchain", db)
	chainDBfp := params.DatabaseWrite_path + fmt.Sprintf("chainDB/S%d_N%d", cc.ShardID, cc.NodeID)
	bc := &BlockChain{
		db:           db,
		ChainConfig:  cc,
		Txpool:       core.NewTxPool(),
		Storage:      storage.NewStorage(chainDBfp, cc),
		PartitionMap: make(map[string]uint64),
		G2EBook:      NewG2EBook(), // <--- 新增
	}

	curHash, err := bc.Storage.GetNewestBlockHash()
	if err != nil {
		fmt.Println("There is no existed blockchain in the database. ")
		// if the Storage bolt database cannot find the newest blockhash,
		// it means the blockchain should be built in height = 0
		if err.Error() == "cannot find the newest block hash" {
			genisisBlock := bc.NewGenisisBlock()
			bc.AddGenisisBlock(genisisBlock)
			fmt.Println("New genisis block")
			return bc, nil
		}
		log.Panic()
	}

	// there is a blockchain in the storage
	fmt.Println("Existing blockchain found")
	curb, err := bc.Storage.GetBlock(curHash)
	if err != nil {
		log.Panic()
	}

	bc.CurrentBlock = curb
	triedb := trie.NewDatabaseWithConfig(db, &trie.Config{
		Cache:     0,
		Preimages: true,
	})
	bc.triedb = triedb
	// check the existence of the trie database
	_, err = trie.New(trie.TrieID(common.BytesToHash(curb.Header.StateRoot)), triedb)
	if err != nil {
		log.Panic()
	}
	fmt.Println("The status trie can be built")
	fmt.Println("Generated a new blockchain successfully")
	return bc, nil
}

// check a block is valid or not in this blockchain config
func (bc *BlockChain) IsValidBlock(b *core.Block) error {
	if string(b.Header.ParentBlockHash) != string(bc.CurrentBlock.Hash) {
		fmt.Println("the parentblock hash is not equal to the current block hash")
		return errors.New("the parentblock hash is not equal to the current block hash")
	} else if string(GetTxTreeRoot(b.Body)) != string(b.Header.TxRoot) {
		fmt.Println("the transaction root is wrong")
		return errors.New("the transaction root is wrong")
	}
	return nil
}

// add accounts
func (bc *BlockChain) AddAccounts(ac []string, as []*core.AccountState, miner int32) {
	fmt.Printf("The len of accounts is %d, now adding the accounts\n", len(ac))

	bh := &core.BlockHeader{
		ParentBlockHash: bc.CurrentBlock.Hash,
		Number:          bc.CurrentBlock.Header.Number + 1,
		Time:            time.Time{},
	}
	// handle transactions to build root
	rt := bc.CurrentBlock.Header.StateRoot
	if len(ac) != 0 {
		st, err := trie.New(trie.TrieID(common.BytesToHash(bc.CurrentBlock.Header.StateRoot)), bc.triedb)
		if err != nil {
			log.Panic(err)
		}
		for i, addr := range ac {
			if bc.Get_PartitionMap(addr) == bc.ChainConfig.ShardID {
				ib := new(big.Int)
				ib.Add(ib, as[i].Balance)
				new_state := &core.AccountState{
					Balance: ib,
					Nonce:   as[i].Nonce,
				}
				st.Update([]byte(addr), new_state.Encode())
			}
		}

		rrt, ns := st.Commit(false)

		// if `ns` is nil, the `err = bc.triedb.Update(trie.NewWithNodeSet(ns))` will report an error.
		if ns != nil {
			err = bc.triedb.Update(trie.NewWithNodeSet(ns))
			if err != nil {
				log.Panic(err)
			}
			err = bc.triedb.Commit(rrt, false)
			if err != nil {
				log.Panic(err)
			}
			rt = rrt.Bytes()
		}
	}

	emptyTxs := make([]*core.Transaction, 0)
	bh.StateRoot = rt
	bh.TxRoot = GetTxTreeRoot(emptyTxs)
	bh.Bloom = *GetBloomFilter(emptyTxs)
	bh.Miner = 0
	b := core.NewBlock(bh, emptyTxs)
	b.Hash = b.Header.Hash()

	bc.CurrentBlock = b
	bc.Storage.AddBlock(b)
}

// fetch accounts
func (bc *BlockChain) FetchAccounts(addrs []string) []*core.AccountState {
	res := make([]*core.AccountState, 0)
	st, err := trie.New(trie.TrieID(common.BytesToHash(bc.CurrentBlock.Header.StateRoot)), bc.triedb)
	if err != nil {
		log.Panic(err)
	}
	for _, addr := range addrs {
		asenc, _ := st.Get([]byte(addr))
		var state_a *core.AccountState
		if asenc == nil {
			fmt.Printf("************* Account %s not found in trie\n", addr)
			ib := new(big.Int)
			ib.Add(ib, params.Init_Balance)
			state_a = &core.AccountState{
				Nonce:   uint64(0),
				Balance: ib,
			}
		} else {
			state_a = core.DecodeAS(asenc)
		}
		res = append(res, state_a)
	}
	return res
}

// close a blockChain, close the database inferfaces
func (bc *BlockChain) CloseBlockChain() {
	bc.Storage.DataBase.Close()
	bc.triedb.CommitPreimages()
	bc.db.Close()
}

// print the details of a blockchain
func (bc *BlockChain) PrintBlockChain() string {
	vals := []interface{}{
		bc.CurrentBlock.Header.Number,
		bc.CurrentBlock.Hash,
		bc.CurrentBlock.Header.StateRoot,
		bc.CurrentBlock.Header.Time,
		bc.triedb,
		// len(bc.Txpool.RelayPool[1]),
	}
	res := fmt.Sprintf("%v\n", vals)
	fmt.Println(res)
	return res
}

// 简单确定性策略：按高度轮转选择 pair（可替换为 VRF/统计）
// 要求：全网节点拿到相同输入就能得到相同 pair
func chooseWinningPair(h uint64, eb *g2eEpochBook) [2]uint64 {
	// 把已下注的 pair 收集起来，按字典序排序后取 (h mod len)
	keys := make([][2]uint64, 0, len(eb.SumByPair))
	for k := range eb.SumByPair {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	if len(keys) == 0 {
		return [2]uint64{0, 0}
	}
	return keys[int(h)%len(keys)]
}
