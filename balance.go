package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// constants
const balancesJsonPath = "balances.json"
const gethIpcPath = "/mydata/sepolia/geth.ipc"
const address = "0x16bD8c7297df5Aa981B328DBa02466bc7c064EB7"
const startBlock = 4000001
const numBlocks = 2048

// getBalances returns the balances from the json file if it exists, otherwise it gets the balances from the geth node
func getBalances() []*big.Int {
	// check if balances.json exists
	if _, err := os.Stat(balancesJsonPath); os.IsNotExist(err) {
		fmt.Println("Balances.json not found, getting balances from geth node")
		balances := getBalancesFromGeth()
		dumpBalancesToJson(balances)
		return balances
	}

	return getBalancesFromJson()

}

// getBalancesFromGeth gets the balances from the geth node
func getBalancesFromGeth() []*big.Int {
	client, err := ethclient.Dial(gethIpcPath)
	if err != nil {
		log.Fatalf("failed to connect to geth: %v", err)
	}

	address := common.HexToAddress(address)
	var startBlock uint32 = startBlock

	balances := make([]*big.Int, numBlocks)

	for i := startBlock; i < startBlock+numBlocks; i++ {
		balance, err := client.BalanceAt(context.Background(), address, big.NewInt(int64(i)))
		if err != nil {
			log.Fatalln("Failed to get balance at block number", i, "error", err)
		}

		balances[i-startBlock] = balance

		if i%100 == 0 {
			fmt.Printf("Balance at block %d: %s\n", i, balance.String())
		}

	}

	return balances
}

// getBalancesFromJson gets the balances from the json file
func getBalancesFromJson() []*big.Int {
	jsonFile, err := os.Open("balances.json")
	if err != nil {
		log.Fatalf("Failed to open balances.json: %v", err)
	}
	defer jsonFile.Close()
	var balances []*big.Int
	json.NewDecoder(jsonFile).Decode(&balances)

	return balances
}

// dumpBalancesToJson dumps the balances to the json file
func dumpBalancesToJson(balances []*big.Int) {
	jsonFile, err := os.Create("balances.json")
	if err != nil {
		log.Fatalf("Failed to create balances.json: %v", err)
	}
	defer jsonFile.Close()
	json.NewEncoder(jsonFile).Encode(balances)
	fmt.Println("Balances dumped to balances.json")
}
