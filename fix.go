package main

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/ipfs/go-datastore"
)

func fixMinerMetadata(ctx context.Context, minerRepoPath string, addr address.Address) error {
	r, err := repo.NewFS(minerRepoPath)
	if err != nil {
		return err
	}

	ok, err := r.Exists()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("repo at '%s' is not initialized, run 'lotus-miner init' to set it up", minerRepoPath)
	}

	lr, err := r.Lock(repo.StorageMiner)
	if err != nil {
		return err
	}
	defer lr.Close() //nolint:errcheck

	mds, err := lr.Datastore(context.TODO(), "/metadata")
	if err != nil {
		return err
	}

	if err := mds.Put(datastore.NewKey("miner-address"), addr.Bytes()); err != nil {
		return err
	}

	return nil
}
