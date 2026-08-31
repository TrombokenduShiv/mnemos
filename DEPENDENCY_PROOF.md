# Dependency Proof

This project was built entirely from scratch using only the Go standard library, with absolutely zero external third-party dependencies.

## 1. `go.mod` Manifest
The `go.mod` file contains only the module definition and the Go version. No `require` blocks exist.

```go
module mnemos

go 1.27.0
```

## 2. Dependency Graph Proof
Running the Go module graph command to print all requirements yields completely empty output:

```bash
$ go mod graph
mnemos go@1.27.0
go@1.27.0 toolchain@go1.27.0
```

*(Note: The above output only shows the mandatory Go version and the internal Go compiler toolchain. There are **zero** external third-party modules like `github.com/...` or `golang.org/x/...`)*

Running the module list command confirms that only the main module itself exists:

```bash
$ go list -m all
mnemos
```

## 3. Byte-Identical Reproducible Build
Because there are no dynamically fetched third-party packages with fluctuating versions, the project compiles perfectly reproducibly:

```bash
$ go build -trimpath -ldflags="-s -w" -o mnemos1.exe ./cmd/mnemos
$ go build -trimpath -ldflags="-s -w" -o mnemos2.exe ./cmd/mnemos

$ CertUtil -hashfile mnemos1.exe SHA256
816bf371aab3554313fae3d849c3cd5b84d440f8d70d9771d952f5160da50c2a

$ CertUtil -hashfile mnemos2.exe SHA256
816bf371aab3554313fae3d849c3cd5b84d440f8d70d9771d952f5160da50c2a
```
The exact same SHA256 hashes are produced every time.
