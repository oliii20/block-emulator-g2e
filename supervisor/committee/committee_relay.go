package committee

import (
	"blockEmulator/core"
	"blockEmulator/message"
	"blockEmulator/networks"
	"blockEmulator/params"
	"blockEmulator/supervisor/signal"
	"blockEmulator/supervisor/supervisor_log"
	"blockEmulator/utils"
	"encoding/csv"
	"encoding/json"
	"io"
	"log"
	"math/big"
	"os"
	"strconv"
	"time"
)

type RelayCommitteeModule struct {
	csvPath      string
	dataTotalNum int
	nowDataNum   int
	batchDataNum int
	IpNodeTable  map[uint64]map[uint64]string
	sl           *supervisor_log.SupervisorLog
	Ss           *signal.StopSignal // to control the stop message sending
}

func NewRelayCommitteeModule(Ip_nodeTable map[uint64]map[uint64]string, Ss *signal.StopSignal, slog *supervisor_log.SupervisorLog, csvFilePath string, dataNum, batchNum int) *RelayCommitteeModule {
	return &RelayCommitteeModule{
		csvPath:      csvFilePath,
		dataTotalNum: dataNum,
		batchDataNum: batchNum,
		nowDataNum:   0,
		IpNodeTable:  Ip_nodeTable,
		Ss:           Ss,
		sl:           slog,
	}
}

// transfrom, data to transaction
// check whether it is a legal txs meesage. if so, read txs and put it into the txlist
func data2tx(data []string, nonce uint64) (*core.Transaction, bool) {
	if data[3] == "0" && data[4] == "0" && len(data[0]) > 16 && len(data[1]) > 16 && data[0] != data[1] {
		val, ok := new(big.Float).SetString(data[5]) //下注金额value
		if !ok {
			log.Panic("convert string to float failed\n")
		}
		new_val := new(big.Int)
		val.Int(new_val)

		// fmt.Println(data[5])
		height, _ := strconv.ParseUint(data[6], 10, 64)
		pair_i, _ := strconv.ParseUint(data[7], 10, 64)
		pair_j, _ := strconv.ParseUint(data[8], 10, 64)
		tx := core.NewTransaction(data[0][2:], data[1][2:], new_val, nonce, time.Now(), height, pair_i, pair_j)
		return tx, true
	}
	return &core.Transaction{}, false
}

func (rthm *RelayCommitteeModule) HandleOtherMessage([]byte) {}

func (rthm *RelayCommitteeModule) txSending(txlist []*core.Transaction) {
	// the txs will be sent
	sendToShard := make(map[uint64][]*core.Transaction)

	for idx := 0; idx <= len(txlist); idx++ {
		if idx > 0 && (idx%params.InjectSpeed == 0 || idx == len(txlist)) {
			// send to shard
			for sid := uint64(0); sid < uint64(params.ShardNum); sid++ {
				it := message.InjectTxs{
					Txs:       sendToShard[sid],
					ToShardID: sid,
				}
				itByte, err := json.Marshal(it)
				if err != nil {
					log.Panic(err)
				}
				send_msg := message.MergeMessage(message.CInject, itByte)
				go networks.TcpDial(send_msg, rthm.IpNodeTable[sid][0])
			}
			sendToShard = make(map[uint64][]*core.Transaction)
			time.Sleep(time.Second)
		}
		if idx == len(txlist) {
			break
		}
		tx := txlist[idx]
		sendersid := uint64(utils.Addr2Shard(tx.Sender))
		sendToShard[sendersid] = append(sendToShard[sendersid], tx)
	}
}

// read transactions, the Number of the transactions is - batchDataNum
func (rthm *RelayCommitteeModule) MsgSendingControl() {
	txfile, err := os.Open(rthm.csvPath)
	if err != nil {
		log.Panic(err)
	}
	defer txfile.Close()
	reader := csv.NewReader(txfile)
	txlist := make([]*core.Transaction, 0) // save the txs in this epoch (round)

	for {
		data, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Panic(err)
		}
		if tx, ok := data2tx(data, uint64(rthm.nowDataNum)); ok {
			txlist = append(txlist, tx)
			rthm.nowDataNum++
		}

		// re-shard condition, enough edges
		if len(txlist) == int(rthm.batchDataNum) || rthm.nowDataNum == rthm.dataTotalNum {
			rthm.txSending(txlist)
			// reset the variants about tx sending
			txlist = make([]*core.Transaction, 0)
			rthm.Ss.StopGap_Reset()
		}

		if rthm.nowDataNum == rthm.dataTotalNum {
			break
		}
	}
}

// no operation here
func (rthm *RelayCommitteeModule) HandleBlockInfo(b *message.BlockInfoMsg) {
	rthm.sl.Slog.Printf("received from shard %d in epoch %d.\n", b.SenderShardID, b.Epoch)
	// to do:
}
