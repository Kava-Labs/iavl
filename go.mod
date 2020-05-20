module github.com/kava-labs/iavl

go 1.13

require (
	github.com/kava-labs/tendermint v0.33.4-0.20200520050724-b26f703498c9
	github.com/kava-labs/tm-db v0.4.1-stable
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.5.1
	github.com/tendermint/go-amino v0.14.1
	golang.org/x/crypto v0.0.0-20200406173513-056763e48d71
)

replace github.com/kava-labs/tendermint => github.com/kava-labs/tendermint v0.33.4-0.20200520050724-b26f703498c9

replace github.com/tendermint/tm-db => github.com/kava-labs/tm-db v0.4.1-stable
