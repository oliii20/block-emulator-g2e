// Supervisor is an abstract role in this simulator that may read txs, generate partition infos,
// and handle history data.

package supervisor

import (
	"blockEmulator/message"
	"blockEmulator/networks"
	"blockEmulator/params"
	"blockEmulator/supervisor/committee"
	"blockEmulator/supervisor/measure"
	"blockEmulator/supervisor/signal"
	"blockEmulator/supervisor/supervisor_log"
	"blockEmulator/utils"
	"bufio"
	"fmt"

	// "encoding/csv"
	"encoding/json"
	// "fmt"
	"io"
	"log"
	"net"

	// "os"
	// "strconv"
	"sync"
	"time"
)

type Supervisor struct {
	// basic infos
	IPaddr       string // ip address of this Supervisor
	ChainConfig  *params.ChainConfig
	Ip_nodeTable map[uint64]map[uint64]string

	// tcp control
	listenStop bool
	tcpLn      net.Listener
	tcpLock    sync.Mutex
	// logger module
	sl *supervisor_log.SupervisorLog

	// control components
	Ss *signal.StopSignal // to control the stop message sending

	// supervisor and committee components
	comMod committee.CommitteeModule

	// measure components
	testMeasureMods []measure.MeasureModule

	// // diy, add more structures or classes here ...
	// dbMu sync.Mutex
}

func (d *Supervisor) NewSupervisor(ip string, pcc *params.ChainConfig, committeeMethod string, measureModNames ...string) {
	d.IPaddr = ip
	d.ChainConfig = pcc
	d.Ip_nodeTable = params.IPmap_nodeTable

	d.sl = supervisor_log.NewSupervisorLog()

	d.Ss = signal.NewStopSignal(3 * int(pcc.ShardNums))

	switch committeeMethod {
	case "CLPA_Broker":
		d.comMod = committee.NewCLPACommitteeMod_Broker(d.Ip_nodeTable, d.Ss, d.sl, params.DatasetFile, params.TotalDataSize, params.TxBatchSize, params.ReconfigTimeGap)
	case "CLPA":
		d.comMod = committee.NewCLPACommitteeModule(d.Ip_nodeTable, d.Ss, d.sl, params.DatasetFile, params.TotalDataSize, params.TxBatchSize, params.ReconfigTimeGap)
	case "Broker":
		d.comMod = committee.NewBrokerCommitteeMod(d.Ip_nodeTable, d.Ss, d.sl, params.DatasetFile, params.TotalDataSize, params.TxBatchSize)
	default:
		d.comMod = committee.NewRelayCommitteeModule(d.Ip_nodeTable, d.Ss, d.sl, params.DatasetFile, params.TotalDataSize, params.TxBatchSize)
	}

	d.testMeasureMods = make([]measure.MeasureModule, 0)
	for _, mModName := range measureModNames {
		switch mModName {
		case "TPS_Relay":
			d.testMeasureMods = append(d.testMeasureMods, measure.NewTestModule_avgTPS_Relay())
		case "TPS_Broker":
			d.testMeasureMods = append(d.testMeasureMods, measure.NewTestModule_avgTPS_Broker())
		case "TCL_Relay":
			d.testMeasureMods = append(d.testMeasureMods, measure.NewTestModule_TCL_Relay())
		case "TCL_Broker":
			d.testMeasureMods = append(d.testMeasureMods, measure.NewTestModule_TCL_Broker())
		case "CrossTxRate_Relay":
			d.testMeasureMods = append(d.testMeasureMods, measure.NewTestCrossTxRate_Relay())
		case "CrossTxRate_Broker":
			d.testMeasureMods = append(d.testMeasureMods, measure.NewTestCrossTxRate_Broker())
		case "TxNumberCount_Relay":
			d.testMeasureMods = append(d.testMeasureMods, measure.NewTestTxNumCount_Relay())
		case "TxNumberCount_Broker":
			d.testMeasureMods = append(d.testMeasureMods, measure.NewTestTxNumCount_Broker())
		case "Tx_Details":
			d.testMeasureMods = append(d.testMeasureMods, measure.NewTestTxDetail())
		default:
		}
	}
}

func (d *Supervisor) QueryForAcc(address string) {
	fmt.Println("Start querying ...")
	for {
		time.Sleep(5 * time.Second)
		fmt.Println("Start query")
		sii := message.QueryACC{
			Address: address,
		}
		sByte, err := json.Marshal(sii)
		if err != nil {
			log.Panic()
		}
		msg_send := message.MergeMessage(message.DQueryForAcc, sByte)
		shard_id := utils.Addr2Shard(address)
		go networks.TcpDial(msg_send, d.Ip_nodeTable[uint64(shard_id)][0])
	}
}

func (d *Supervisor) handleReplyAcc(content []byte) {
	bim := new(message.ReplyToAcc)
	err := json.Unmarshal(content, bim)
	if err != nil {
		log.Panic()
	}
	fmt.Printf("AAAAAAAAAAAAAAAAAAAAA Account balance of %v is %v\n", bim.Address, bim.Balance)
}

// Supervisor received the block information from the leaders, and handle these
// message to measure the performances.
func (d *Supervisor) handleBlockInfos(content []byte) {
	bim := new(message.BlockInfoMsg)
	err := json.Unmarshal(content, bim) // 反序列化/解包
	if err != nil {
		log.Panic()
	}
	// StopSignal check
	if bim.BlockBodyLength == 0 {
		d.Ss.StopGap_Inc()
	} else {
		d.Ss.StopGap_Reset()
	}

	d.comMod.HandleBlockInfo(bim)

	// measure update
	for _, measureMod := range d.testMeasureMods {
		measureMod.UpdateMeasureRecord(bim)
	}
	// // add codes here ...
	// accountsList := []string{
	// 	"32be343b94f860124dc4fee278fdcbd38c102d88",
	// 	"147184ef469ce9bba3d08af16f0b6d31cac35ac8",
	// 	"5410c4c59a719eef345869c204ec8dcac2dfd718",
	// }

	// shardId := utils.Addr2Shard(accountsList[0])
	// fmt.Printf("shard id is: %d\n", shardId)

	// fmt.Println("Start reading")
	// mptfp := params.DatabaseWrite_path + fmt.Sprintf("mptDB/ldb/s%d/n%d", shardId, 0)
	// chaindbfp := params.DatabaseWrite_path + fmt.Sprintf("chainDB/S%d_N%d", shardId, 0)

	// print("1\n")

	// time.Sleep(500 * time.Millisecond)

	// // 加锁，防止并发访问数据库
	// d.dbMu.Lock()
	// defer d.dbMu.Unlock()

	// accountState := query.QueryAccountState(chaindbfp, mptfp, uint64(shardId), 0, accountsList[0])

	// if accountState == nil {
	// 	fmt.Println("❌ accountState == nil (读取失败)")
	// }
	// fmt.Printf("account %s's balance is: %v\n", accountsList[0], accountState.Balance)
}

// mptfp := params.DatabaseWrite_path + "mptDB/ldb/s0/n0"
// chaindbfp := params.DatabaseWrite_path + fmt.Sprintf("chainDB/S%d_N%d", 0, 0)
// accountState := query.QueryAccountState(chaindbfp, mptfp, 0, 0, "00000000001")
// fmt.Println("The account balance of 00000000001:", accountState.Balance)

// astates := d.chain.FetchAccounts(accountsList)
// for idx, state := range astates {
// 	fmt.Printf("Account %s Balance: %v\n", accountsList[idx], state.Balance)
// }

// // 创建或追加写入 CSV 文件
// filePath := "balances.csv"
// file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
// if err != nil {
// 	log.Fatalf("cannot open csv file: %v", err)
// }
// defer file.Close()

// writer := csv.NewWriter(file)
// defer writer.Flush()

// // 如果是新文件，可以写入表头
// info, _ := file.Stat()
// if info.Size() == 0 {
// 	writer.Write([]string{"Epoch", "Account", "Balance"})
// }

// // 写入数据行
// for idx, state := range astates {
// 	writer.Write([]string{
// 		strconv.Itoa(bim.Epoch),
// 		accountsList[idx],
// 		state.Balance.String(), // 假设 Balance 是 big.Int
// 	})
// }

// read transactions from dataFile. When the number of data is enough,
// the Supervisor will do re-partition and send partitionMSG and txs to leaders.
func (d *Supervisor) SupervisorTxHandling() {
	d.comMod.MsgSendingControl()
	// TxHandling is end
	for !d.Ss.GapEnough() { // wait all txs to be handled
		time.Sleep(time.Second)
	}
	// send stop message
	stopmsg := message.MergeMessage(message.CStop, []byte("this is a stop message~"))
	d.sl.Slog.Println("Supervisor: now sending cstop message to all nodes")
	for sid := uint64(0); sid < d.ChainConfig.ShardNums; sid++ {
		for nid := uint64(0); nid < d.ChainConfig.Nodes_perShard; nid++ {
			networks.TcpDial(stopmsg, d.Ip_nodeTable[sid][nid])
		}
	}
	// make sure all stop messages are sent.
	time.Sleep(time.Duration(params.Delay+params.JitterRange+3) * time.Millisecond)

	d.sl.Slog.Println("Supervisor: now Closing")
	d.listenStop = true
	d.CloseSupervisor()
}

// handle message. only one message to be handled now
func (d *Supervisor) handleMessage(msg []byte) {
	msgType, content := message.SplitMessage(msg)
	switch msgType {
	case message.CBlockInfo:
		d.handleBlockInfos(content)
		// add codes for more functionality
	case message.DReplyToAcc:
		d.handleReplyAcc(content)
	default:
		d.comMod.HandleOtherMessage(msg)
		for _, mm := range d.testMeasureMods {
			mm.HandleExtraMessage(msg)
		}
	}
}

func (d *Supervisor) handleClientRequest(con net.Conn) {
	defer con.Close()
	clientReader := bufio.NewReader(con)
	for {
		clientRequest, err := clientReader.ReadBytes('\n')
		switch err {
		case nil:
			d.tcpLock.Lock()
			d.handleMessage(clientRequest)
			d.tcpLock.Unlock()
		case io.EOF:
			log.Println("client closed the connection by terminating the process")
			return
		default:
			log.Printf("error: %v\n", err)
			return
		}
	}
}

func (d *Supervisor) TcpListen() {
	ln, err := net.Listen("tcp", d.IPaddr)
	if err != nil {
		log.Panic(err)
	}
	d.tcpLn = ln
	for {
		conn, err := d.tcpLn.Accept()
		if err != nil {
			return
		}
		go d.handleClientRequest(conn)
	}
}

// close Supervisor, and record the data in .csv file
func (d *Supervisor) CloseSupervisor() {
	d.sl.Slog.Println("Closing...")
	for _, measureMod := range d.testMeasureMods {
		d.sl.Slog.Println(measureMod.OutputMetricName())
		d.sl.Slog.Println(measureMod.OutputRecord())
		println()
	}
	networks.CloseAllConnInPool()
	d.tcpLn.Close()
}
