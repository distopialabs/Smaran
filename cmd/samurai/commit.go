package main

// func generateCommitments(concurrency int, config *config.Config, precomputedData *config.PrecomputedData) {

// 	os.RemoveAll(segmenttree.StoragePath)

// 	accountTrees := make(map[common.Address]*segmenttree.SegmentTree, len(config.TrackedAccounts))
// 	for _, addr := range config.TrackedAccounts {
// 		accountTrees[addr] = segmenttree.New(addr, precomputedData)
// 	}

// 	total_start := time.Now()
// 	for bn := config.StartingBlockNumber; bn <= config.EndingBlockNumber; bn += 1 {
// 		fmt.Println("\nProcessing block", bn)

// 		inner_total_start := time.Now()
// 		start := time.Now()
// 		// TODO: fetch all the account whose value changes in this block.
// 		// TODO: fetch balances for those accounts.
// 		balances, err := ledger.BatchMultiUserBalance(config.TrackedAccounts, bn, config)
// 		if err != nil {
// 			panic(err)
// 		}
// 		fmt.Printf("Time taken to get balances for block %d: %v\n", bn, time.Since(start))

// 		start = time.Now()
// 		rel_bn := bn - config.StartingBlockNumber
// 		var wg sync.WaitGroup
// 		sem := make(chan struct{}, concurrency)

// 		for i, addr := range config.TrackedAccounts {
// 			wg.Add(1)
// 			sem <- struct{}{}

// 			go func(i int, addr common.Address) {
// 				defer wg.Done()
// 				defer func() { <-sem }()

// 				balance := balances[i]
// 				if balance.Cmp(big.NewInt(0)) == 0 {
// 					fmt.Println("Balance is 0 for account", addr.Hex())
// 				}
// 				accountTrees[addr].AddLeafNode(rel_bn, common.BigToHash(balance))
// 			}(i, addr)
// 		}

// 		wg.Wait()
// 		fmt.Printf("Time taken to update account trees for block %d: %v\n", bn, time.Since(start))

// 		fmt.Printf("Time taken to process block %d: %v\n", bn, time.Since(inner_total_start))
// 		// every 100 blocks, print the time elapsed
// 		if bn&127 == 0 {
// 			fmt.Printf("Time elapsed: %v\n", time.Since(total_start))
// 		}
// 	}
// 	fmt.Printf("\nTime taken to process all blocks: %v\n", time.Since(total_start))
// 	start := time.Now()
// 	for _, addr := range config.TrackedAccounts {
// 		accountTrees[addr].FlushIfRemaining(int(config.EndingBlockNumber - config.StartingBlockNumber))
// 	}
// 	fmt.Printf("Time taken to flush account trees: %v\n", time.Since(start))

// 	// queryStartBlock := 20
// 	// queryEndBlock := 200000

// 	// for _, addr := range config.TrackedAccounts {
// 	// 	start := time.Now()
// 	// 	fmt.Println("Generating range proofs for account", addr.Hex())
// 	// 	rangeProofs, balances := proof.GetRangeProofs(addr, queryStartBlock, queryEndBlock, V, weights, srs, config.StartingBlockNumber)
// 	// 	_ = rangeProofs
// 	// 	_ = balances
// 	// 	fmt.Println("Time taken to generate range proofs", time.Since(start))
// 	// 	start = time.Now()
// 	// 	proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, rangeProofs, balances, V, weights, srs)
// 	// 	fmt.Println("Time taken to verify range proofs", time.Since(start))
// 	// 	break
// 	// }

// 	// main2()
// }
