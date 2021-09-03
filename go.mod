module github.com/lanzafame/fil-miner-buyer

require (
	github.com/docker/go-units v0.4.0
	github.com/filecoin-project/go-address v0.0.6
	github.com/filecoin-project/go-jsonrpc v0.1.4-0.20210217175800-45ea43ac2bec
	github.com/filecoin-project/go-state-types v0.1.1-0.20210810190654-139e0e79e69e
	github.com/filecoin-project/lotus v1.11.1
	github.com/filecoin-project/specs-actors v0.9.14
	github.com/filecoin-project/specs-actors/v2 v2.3.5
	github.com/hako/durafmt v0.0.0-20200710122514-c0fb7b4da026
	github.com/ipfs/go-datastore v0.4.5
	github.com/ipfs/go-log/v2 v2.3.0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/urfave/cli/v2 v2.2.0
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
)

go 1.16

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
