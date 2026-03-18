# KT implementation from Samurai

Read [KT.md](KT.md) for more context on how we implemented Optiks.

KT for Samurai, would have the same structure. We will implement a `SamuraiKTServer` struct, which mimics the `OptiksServer` struct, but with these differences:

We use this mpt
```go
    mpt *trie.Trie

	// trieDB is the backing database for the MPT (in-memory).
	trieDB *triedb.Database
```

To store user name => Samurai Commitment, instead of (user || version) => value.

We add a new field in the struct:

```go

    samurai_accounts map[string]AccountInfo
```

which maps each user to its Samurai Account Struct.
Each new value version gets appended into this AccountInfo and the changed commitment is updated in mpt.

Use the API in `internal/tree` as much as possible. But do not call `AccountInfo::Save`. We want all operations to be in-memory.

The proof for the user now has two parts:
- MptProof: This is the witness generated for the user from the MPT.
- SamuraiProof: It is a proof for all value versions in Samurai. (The range 0--currentVersions[user])

Implement the proof generation and verification part the same.

Finally, update cmd/ktbench and cmd/ktserver to not be noops anymore. Do the necessary wiring.
Make samurai the default protocol in both these binaries.
