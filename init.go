package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"

	"github.com/docker/go-units"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	lotusapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/build"
	"github.com/filecoin-project/lotus/chain/actors"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/actors/builtin/power"
	"github.com/filecoin-project/lotus/chain/types"
	power2 "github.com/filecoin-project/specs-actors/v2/actors/builtin/power"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

var initCmd = &cli.Command{
	Name: "init",
	Action: func(_ *cli.Context) error {
		ctx := context.Background()
		api, closer, err := LotusClient(ctx)
		if err != nil {
			return err
		}
		defer closer()

		addr, err := Init(ctx, api)
		if err != nil {
			return err
		}

		fmt.Println(addr)

		return nil
	},
}

func Init(ctx context.Context, api lotusapi.FullNode) (address.Address, error) {
	p2pSk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return address.Undef, fmt.Errorf("failed to generate priv key: %w", err)
	}

	peerid, err := peer.IDFromPrivateKey(p2pSk)
	if err != nil {
		return address.Undef, xerrors.Errorf("peer ID from private key: %w", err)
	}

	owner, err := api.WalletDefaultAddress(ctx)
	if err != nil {
		return address.Undef, fmt.Errorf("failed to get default wallet: %w", err)
	}

	ssize, err := units.RAMInBytes("32GiB")
	if err != nil {
		return address.Undef, fmt.Errorf("failed to parse sector size: %w", err)
	}

	worker, err := api.WalletNew(ctx, types.KTBLS)
	if err != nil {
		return address.Address{}, fmt.Errorf("failed to create worker wallet address: %w", err)
	}

	// make sure the worker account exists on chain
	_, err = api.StateLookupID(ctx, worker, types.EmptyTSK)
	if err != nil {
		signed, err := api.MpoolPushMessage(ctx, &types.Message{
			From:  owner,
			To:    worker,
			Value: types.NewInt(0),
		}, nil)
		if err != nil {
			return address.Undef, xerrors.Errorf("push worker init: %w", err)
		}

		log.Infof("Initializing worker account %s, message: %s", worker, signed.Cid())
		log.Infof("Waiting for confirmation")

		mw, err := api.StateWaitMsg(ctx, signed.Cid(), build.MessageConfidence, lotusapi.LookbackNoLimit, true)
		// mw, err := api.StateWaitMsg(ctx, signed.Cid(), build.MessageConfidence)
		if err != nil {
			return address.Undef, xerrors.Errorf("waiting for worker init: %w", err)
		}
		if mw.Receipt.ExitCode != 0 {
			return address.Undef, xerrors.Errorf("initializing worker account failed: exit code %d", mw.Receipt.ExitCode)
		}
	}

	nv, err := api.StateNetworkVersion(ctx, types.EmptyTSK)
	if err != nil {
		return address.Undef, xerrors.Errorf("getting network version: %w", err)
	}

	spt, err := miner.SealProofTypeFromSectorSize(abi.SectorSize(ssize), nv)
	if err != nil {
		return address.Undef, xerrors.Errorf("getting seal proof type: %w", err)
	}

	params, err := actors.SerializeParams(&power2.CreateMinerParams{
		Owner:         owner,
		Worker:        worker,
		SealProofType: spt,
		Peer:          abi.PeerID(peerid),
	})
	if err != nil {
		return address.Undef, xerrors.Errorf("failed to serialize params: %w", err)
	}

	sender := owner

	createStorageMinerMsg := &types.Message{
		To:    power.Address,
		From:  sender,
		Value: big.Zero(),

		Method: power.Methods.CreateMiner,
		Params: params,

		GasLimit:   0,
		GasPremium: types.NewInt(0),
	}

	signed, err := api.MpoolPushMessage(ctx, createStorageMinerMsg, nil)
	if err != nil {
		return address.Undef, xerrors.Errorf("pushing createMiner message: %w", err)
	}

	log.Infof("Pushed CreateMiner message: %s", signed.Cid())
	log.Infof("Waiting for confirmation")

	mw, err := api.StateWaitMsg(ctx, signed.Cid(), build.MessageConfidence, lotusapi.LookbackNoLimit, true)
	// mw, err := api.StateWaitMsg(ctx, signed.Cid(), build.MessageConfidence)
	if err != nil {
		return address.Undef, xerrors.Errorf("waiting for createMiner message: %w", err)
	}

	if mw.Receipt.ExitCode != 0 {
		return address.Undef, xerrors.Errorf("create miner failed: exit code %d", mw.Receipt.ExitCode)
	}

	var retval power2.CreateMinerReturn
	if err := retval.UnmarshalCBOR(bytes.NewReader(mw.Receipt.Return)); err != nil {
		return address.Undef, xerrors.Errorf("failed to unmarshal cbor: %w", err)
	}

	log.Infof("New miners address is: %s (%s)", retval.IDAddress, retval.RobustAddress)
	return retval.IDAddress, nil
}
