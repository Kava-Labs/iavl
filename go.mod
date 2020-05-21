module github.com/kava-labs/iavl

go 1.13

require (
	github.com/kava-labs/tendermint v0.33.3-pinned
	github.com/kava-labs/tm-db v0.4.1-kava
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.5.1
	github.com/tendermint/go-amino v0.14.1
	golang.org/x/crypto v0.0.0-20200406173513-056763e48d71
)

replace github.com/tendermint/tm-db => github.com/kava-labs/tm-db v0.4.1-kava

replace github.com/tendermint/tendermint => github.com/kava-labs/tendermint v0.33.3-pinned
