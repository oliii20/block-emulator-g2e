package main

import (
	"blockEmulator/params"
	"blockEmulator/query"
	"blockEmulator/utils"
	"fmt"
)

func main() {
	accountsList := []string{
		"32be343b94f860124dc4fee278fdcbd38c102d88",
		"147184ef469ce9bba3d08af16f0b6d31cac35ac8",
		"5410c4c59a719eef345869c204ec8dcac2dfd718",
	}

	shardId := utils.Addr2Shard(accountsList[0])
	fmt.Printf("shard id is: %d\n", shardId)

	fmt.Println("Start reading")
	mptfp := params.DatabaseWrite_path + fmt.Sprintf("mptDB/ldb/s%d/n%d", shardId, 0)
	chaindbfp := params.DatabaseWrite_path + fmt.Sprintf("chainDB/S%d_N%d", shardId, 0)
	accountState := query.QueryAccountState(chaindbfp, mptfp, uint64(shardId), 0, accountsList[0])
	fmt.Printf("account %s's balance is: %v\n", accountsList[0], accountState.Balance)

}
