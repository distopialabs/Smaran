# OPTIKS

OPTIKS is one of the three key-transparency protocols benchmarked in this
artifact. Unlike CONIKS (which lives in the [`Coniks/`](../Coniks) submodule as a
standalone codebase), OPTIKS is a lightweight protocol that we implement inside
the Smaran server binary as a separate --protocol mode.

## Source locations

| Component            | Path                              |
|----------------------|-----------------------------------|
| OPTIKS server logic  | [`../internal/kt/optiks.go`](../internal/kt/optiks.go) |
| Unit tests           | [`../internal/kt/optiks_test.go`](../internal/kt/optiks_test.go) |
| gRPC/HTTP server     | [`../internal/kt/server.go`](../internal/kt/server.go) |
| Server entry point   | [`../cmd/ktserver/main.go`](../cmd/ktserver/main.go) |
| Benchmark client     | [`../cmd/ktbench/main.go`](../cmd/ktbench/main.go) |
| Design spec          | [`../KT.md`](../KT.md) |

## How to build and run

OPTIKS ships inside the same `ktserver` binary as Smaran. To install and run:

    ../KeyTransparencyScripts/install_optiks.sh
    ktserver --addr 0.0.0.0:3191 --protocol optiks

See [`../KeyTransparencyScripts/README.md`](../KeyTransparencyScripts/README.md) for the full artifact-evaluation workflow.
