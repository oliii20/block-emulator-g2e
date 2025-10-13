// Account, AccountState
// Some basic operation about accountState

package core

import (
	"blockEmulator/utils"
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"log"
	"math/big"
)

type Account struct {
	AcAddress utils.Address
	PublicKey []byte
}

// AccoutState record the details of an account, it will be saved in status trie
type AccountState struct {
	AcAddress   utils.Address // this part is not useful, abort
	Nonce       uint64
	Balance     *big.Int
	StorageRoot []byte // only for smart contract account
	CodeHash    []byte // only for smart contract account
}

// Reduce the balance of an account
// func (接收者) 函数名(参数列表) 返回值列表 {
// 函数体
// }
func (as *AccountState) Deduct(val *big.Int) bool { // AccountState 类型的一个方法 Deduct
	if as.Balance.Cmp(val) < 0 { // compare two big.Int
		return false
	}
	as.Balance.Sub(as.Balance, val)
	return true
}

// Increase the balance of an account
func (s *AccountState) Deposit(value *big.Int) {
	s.Balance.Add(s.Balance, value)
}

// Encode AccountState in order to store in the MPT
func (as *AccountState) Encode() []byte {
	var buff bytes.Buffer
	encoder := gob.NewEncoder(&buff)
	err := encoder.Encode(as) // 将 as 编码到 buff 中 即将as结构体序列化为字节流
	if err != nil {
		log.Panic(err)
	}
	return buff.Bytes()
}

// Decode AccountState
func DecodeAS(b []byte) *AccountState {
	var as AccountState

	decoder := gob.NewDecoder(bytes.NewReader(b))
	err := decoder.Decode(&as)
	if err != nil {
		log.Panic(err)
	}
	return &as
}

// Hash AccountState for computing the MPT Root
func (as *AccountState) Hash() []byte { // 传回一个字节切片
	h := sha256.Sum256(as.Encode())
	return h[:]
}
