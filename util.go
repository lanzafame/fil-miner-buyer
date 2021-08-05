package main

import (
	"fmt"
	"os"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/build"
	"github.com/hako/durafmt"
)

var genesisUnixTimestamp = time.Unix(1598306400, 0)

func EpochTimeStr(curr, e abi.ChainEpoch) string {
	switch {
	case curr > e:
		return fmt.Sprintf("%d (%s ago)", e, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(curr-e))).LimitFirstN(2))
	case curr == e:
		return fmt.Sprintf("%d (now)", e)
	case curr < e:
		return fmt.Sprintf("%d (in %s)", e, durafmt.Parse(time.Second*time.Duration(int64(build.BlockDelaySecs)*int64(e-curr))).LimitFirstN(2))
	}

	panic("math broke")
}

func EpochTime(curr, e abi.ChainEpoch) time.Duration {
	return durafmt.Parse(time.Second * time.Duration(int64(build.BlockDelaySecs)*int64(curr-e))).LimitFirstN(2).Duration()
}

func EpochTimestamp(e abi.ChainEpoch) time.Time {
	return genesisUnixTimestamp.Add(DurationSinceGenesis(e))
}

func DurationSinceGenesis(e abi.ChainEpoch) time.Duration {
	return durafmt.Parse(time.Second * time.Duration(int64(build.BlockDelaySecs)*int64(e))).LimitFirstN(2).Duration()
}

func AppendFile(file string, data []byte) error {
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return nil
}

func home(home, path string) string {
	return fmt.Sprintf("%s/%s", home, path)
}
