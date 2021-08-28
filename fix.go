package main

import (
	"context"
	"fmt"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/lotus/node/repo"
	"github.com/ipfs/go-datastore"
)

func (s Miner) GetDatastore(ctx context.Context) (repo.LockedRepo, error) {
	r, err := repo.NewFS(os.Getenv("LOTUS_MINER_PATH"))
	if err != nil {
		return nil, err
	}

	ok, err := r.Exists()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("repo at '%s' is not initialized, run 'lotus-miner init' to set it up", s.MinerPath())
	}

	lr, err := r.Lock(repo.StorageMiner)
	if err != nil {
		return nil, err
	}

	return lr, nil
}

func (s Miner) getMinerMetadata(ctx context.Context) (string, error) {
	lr, err := s.GetDatastore(ctx)
	if err != nil {
		return "", err
	}

	mds, err := lr.Datastore(ctx, "/metadata")
	if err != nil {
		return "", err
	}

	addrb, err := mds.Get(datastore.NewKey("miner-address"))
	if err != nil {
		return "", err
	}

	addr, err := address.NewFromBytes(addrb)
	if err != nil {
		return "", err
	}
	return addr.String(), nil
}

func (s Miner) fixMinerMetadata(ctx context.Context) error {
	lr, err := s.GetDatastore(ctx)
	if err != nil {
		return err
	}

	mds, err := lr.Datastore(ctx, "/metadata")
	if err != nil {
		return err
	}

	addr, err := address.NewFromString(s.id)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}
	if err := mds.Put(datastore.NewKey("miner-address"), addr.Bytes()); err != nil {
		return err
	}

	return nil
}
