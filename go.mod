module github.com/mike76-dev/hostscore

go 1.23.1

toolchain go1.23.4

require (
	github.com/go-sql-driver/mysql v1.7.1
	github.com/julienschmidt/httprouter v1.3.0
	gitlab.com/NebulousLabs/merkletree v0.0.0-20200118113624-07fbf710afc4
	go.sia.tech/core v0.13.3-0.20250616154238-4c58987023c7
	go.sia.tech/coreutils v0.16.3-0.20250618174006-041c22c13758
	go.sia.tech/mux v1.4.0
	go.uber.org/zap v1.27.0
	golang.org/x/term v0.32.0
	lukechampine.com/flagg v1.1.1
	lukechampine.com/frand v1.5.1
)

require (
	go.etcd.io/bbolt v1.4.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.39.0 // indirect
	golang.org/x/tools v0.34.0 // indirect
)

require (
	go.sia.tech/jape v0.11.1
	golang.org/x/sys v0.33.0 // indirect
)
