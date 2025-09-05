process blocks:
fetch all the accounts whose balance changes in this block.
fetch the balances for those accounts.
update the segment tree for those accounts.


for each account:
- fetch the segment tree from the database.
- add a leaf
- update the current balance in db.
- compute the new commitment




# how to store the segment tree in the database.

type CurrentBalance struct { # rlp encode and store with the key "user:<address>:current_balance" or "current_balance:<address>"
	Version    uint64
	Balance    *big.Int
	StartBlock uint64
}

type HistoricalBalance struct { # rlp encode and store with the key "historical_balance:<hash(rlp(historical_balance))>"
	CurrentBalance
	EndBlock uint64
}

type AccountBalanceInfo struct {
	Account common.Address # no need to store

	SegmentTree          *SegmentTree # need to store
	CurrentBalance       *CurrentBalance # need to store
	CurrentCommitment    common.Hash # need to store? LXCommitmentV3[4] stores this value
	HistoricalBalancesKV map[common.Hash][]byte # need to store
}

type SegmentTree struct {
	Account      common.Address # no need to store
	LatestLeafIndex uint64 # no need to store. this value is equal to the current version-1

	LXTreeV3       map[int][]common.Hash # need to store
	LXPolynomialV3 map[int]polynomial.Polynomial # need to store
	LXCommitmentV3 map[int]gnark_kzg.Digest

	// LXPrevCIncCommitmentV3 map[int]gnark_kzg.Digest
	PrecomputedData *config.PrecomputedData # always in memory
	// CachedData      *CachedData
	Storage *Storage # no need to store
}




Table Schema:
# CurrentBalance Table:
CurrentBalance struct {
	Version    uint64
	Balance    *big.Int
	StartBlock uint64
}
key: user:<address>:current_balance or current_balance:<address>
value: rlp(CurrentBalance)



# HistoricalBalance Table:
HistoricalBalance struct {
	CurrentBalance
	EndBlock uint64
}
key: historical_balance:<hash(rlp(historical_balance))>
value: rlp(HistoricalBalance)



# Tree Tables:
LXTreeV3 map[int][]common.Hash # need to store

key: user:<address>:tree:<layer=1,2,3,4>:<index>
value: rlp(LXTreeV3[layer=1])


# Commitment Tables:
LXCommitmentV3 map[int]gnark_kzg.Digest # need to store

key: user:<address>:commitment:<layer=1>



XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX

# Polynomial Tables: I dont think we need to store these at all. we never use it.
LXPolynomialV3 map[int]polynomial.Polynomial # need to store

key: user:<address>:polynomial:<layer=1>
value: rlp(LXPolynomialV3[layer=1])
