// Definition of transaction

package core

import (
	"blockEmulator/utils"
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"log"
	"math/big"
	"time"
)

type Transaction struct {
	Sender    utils.Address
	Recipient utils.Address
	Nonce     uint64
	Signature []byte // not implemented now.
	Value     *big.Int
	TxHash    []byte

	Time time.Time // TimeStamp the tx proposed.

	// used in transaction relaying
	Relayed bool
	// used in broker, if the tx is not a broker1 or broker2 tx, these values should be empty.
	HasBroker      bool
	SenderIsBroker bool
	OriginalSender utils.Address
	FinalRecipient utils.Address
	RawTxHash      []byte

	G2EEnabled bool   // 是否为下注交易
	G2EMeta    []byte // 存放 G2EBetMeta.Encode()，长度为0则非下注

	Height uint64 // 下注区块
	PairI  uint64 // 下注对子
	PairJ  uint64 // 下注对子

}

func (tx *Transaction) PrintTx() string {
	vals := []interface{}{
		tx.Sender[:],
		tx.Recipient[:],
		tx.Value,
		string(tx.TxHash[:]),
	}
	res := fmt.Sprintf("%v\n", vals)
	return res
}

// Encode transaction for storing
func (tx *Transaction) Encode() []byte {
	var buff bytes.Buffer

	enc := gob.NewEncoder(&buff)
	err := enc.Encode(tx)
	if err != nil {
		log.Panic(err)
	}

	return buff.Bytes()
}

// Decode transaction
func DecodeTx(to_decode []byte) *Transaction {
	var tx Transaction

	decoder := gob.NewDecoder(bytes.NewReader(to_decode))
	err := decoder.Decode(&tx)
	if err != nil {
		log.Panic(err)
	}

	return &tx
}

// new a transaction 直接构造成下注交易
func NewTransaction(sender, recipient string, value *big.Int, nonce uint64, proposeTime time.Time, height uint64, pair_i uint64, pair_j uint64) *Transaction {
	tx := &Transaction{
		Sender:    sender,
		Recipient: recipient,
		Value:     value,
		Nonce:     nonce,
		Time:      proposeTime,
		Height:    height,
		PairI:     pair_i,
		PairJ:     pair_j,
	}

	hash := sha256.Sum256(tx.Encode())
	tx.TxHash = hash[:]
	tx.Relayed = false
	tx.FinalRecipient = ""
	tx.OriginalSender = ""
	tx.RawTxHash = nil
	tx.HasBroker = false
	tx.SenderIsBroker = false
	tx.G2EEnabled = true
	return tx
}

//
// // 构造“下注交易”——外观上仍是普通交易（Sender->Recipient），但 G2EEnabled=true
// func NewBetTransaction(playerAddr string, pair [2]uint64, amount *big.Int, targetH uint64, nonce uint64, proposeTime time.Time) *Transaction {
// 	tx := NewTransaction(playerAddr, playerAddr, amount, nonce, proposeTime)
// 	meta := &G2EBetMeta{
// 		Magic:   MagicG2EBet,
// 		Player:  playerAddr,
// 		Pair:    pair,
// 		Amount:  amount.String(),
// 		TargetH: targetH,
// 	}
// 	tx.G2EEnabled = true
// 	tx.G2EMeta = meta.Encode()
// 	// TxHash 已在 NewTransaction 内设置
// 	return tx
// }
// //
