module github.com/lanzafame/fil-miner-buyer

require (
	github.com/docker/go-units v0.4.0
	github.com/fatih/color v1.9.0
	github.com/filecoin-project/go-address v0.0.6
	github.com/filecoin-project/go-jsonrpc v0.1.4-0.20210217175800-45ea43ac2bec
	github.com/filecoin-project/go-state-types v0.1.1-0.20210506134452-99b279731c48
	github.com/filecoin-project/lotus v1.10.1
	github.com/filecoin-project/specs-actors v0.9.13
	github.com/filecoin-project/specs-actors/v2 v2.3.5-0.20210114162132-5b58b773f4fb
	github.com/hako/durafmt v0.0.0-20200710122514-c0fb7b4da026
	github.com/ipfs/go-datastore v0.4.5
	github.com/ipfs/go-log/v2 v2.3.0
	github.com/libp2p/go-libp2p-peer v0.2.0
	github.com/mitchellh/go-homedir v1.1.0
	github.com/urfave/cli/v2 v2.2.0
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
)

go 1.16

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi
