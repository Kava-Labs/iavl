module github.com/kava-labs/iavl

go 1.13

require (
	github.com/kava-labs/tendermint v0.33.4-0.20200520221629-77480532c622
	github.com/kava-labs/tm-db v0.4.2-0.20200506040135-3f7b09feebcd
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.5.1
	github.com/tendermint/go-amino v0.14.1
	golang.org/x/crypto v0.0.0-20200406173513-056763e48d71
)

replace github.com/tendermint/tm-db => github.com/kava-labs/tm-db v0.4.2-0.20200506040135-3f7b09feebcd

replace github.com/tendermint/tendermint => github.com/kava-labs/tendermint v0.33.4-0.20200520221629-77480532c622
