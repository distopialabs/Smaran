# Plan to implement Key Transparency in Samurai.

Create a new binary in `cmd/ktserver/main.go`.
The binary takes 2 arguments as inputs:
- `--addr` to know which IP and port to bind to (defaults `0.0.0.0:3191`)
- `--protocol` which takes one of two values: either `samurai` or `optiks`

This binary works as a gRPC server listening to the following gRPC commands:
- `Get(user)`
- `Put(user, key)`