package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/fatih/color"
	"github.com/filecoin-project/go-address"
	jsonrpc "github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/dline"
	lotusapi "github.com/filecoin-project/lotus/api"
	"github.com/filecoin-project/lotus/api/client"
	"github.com/filecoin-project/lotus/blockstore"
	"github.com/filecoin-project/lotus/chain/actors/builtin/miner"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/filecoin-project/specs-actors/actors/builtin"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

type Service struct {
	api    lotusapi.FullNode
	closer jsonrpc.ClientCloser

	threshold types.FIL

	owner string
}

func NewService(ctx context.Context, threshold string) *Service {
	api, closer, err := LotusClient(ctx)
	if err != nil {
		log.Fatalf("connecting with lotus failed: %s", err)
	}

	owner := os.Getenv("OWNER_ADDR")

	thresholdFIL, err := types.ParseFIL(threshold)
	if err != nil {
		log.Fatalf("parsing threshold failed: %s", err)
	}

	return &Service{api: api, closer: closer, threshold: thresholdFIL, owner: owner}
}

func main() {
	local := []*cli.Command{
		buyCmd,
		infoCmd,
	}

	app := &cli.App{
		Name:     "fil-miner-buyer",
		Commands: local,
	}
	app.Setup()

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}

}

var infoCmd = &cli.Command{
	Name: "info",
	Action: func(c *cli.Context) error {
		ctx := context.Background()

		threshold := os.Getenv("THRESHOLD")
		svc := NewService(ctx, threshold)

		err := svc.GetMinerProvingInfo(ctx)
		if err != nil {
			return err
		}

		return nil
	},
}

var buyCmd = &cli.Command{
	Name: "buy",
	Action: func(c *cli.Context) error {
		ctx := context.Background()

		threshold := os.Getenv("THRESHOLD")
		svc := NewService(ctx, threshold)
		defer svc.closer()

		if svc.IsGasPriceBelowThreshold(ctx) {
			worker, err := svc.CreateBLSWallet(ctx)
			if err != nil {
				log.Fatalf("creating BLS wallet failed: %s", err)
				return err
			}
			log.Println(worker)
			log.Println("initing miner")
			err = svc.InitMiner(ctx, worker)
			if err != nil {
				log.Fatalf("init miner failed: %s", err)
				return err
			}
			//TODO: read miner token out of newly created ~/.lotusminer/token file into env var
		}
		return nil
	},
}

// InitMiner uses the lotus-miner cli to initialize a miner
func (s *Service) InitMiner(ctx context.Context, worker string) error {
	args := []string{"--owner=" + s.owner, "--worker=" + worker, "--no-local-storage"}

	cmd := exec.CommandContext(ctx, "lotus-miner", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

// CreateBLSWallet creates a BLS wallet that will be the worker address
func (s *Service) CreateBLSWallet(ctx context.Context) (string, error) {
	nk, err := s.api.WalletNew(ctx, types.KeyType("bls"))
	if err != nil {
		return "", err
	}

	return nk.String(), nil
}

// IsGasPriceBelowThreshold checks if the gas price is below the threshold
func (s *Service) IsGasPriceBelowThreshold(ctx context.Context) bool {
	est, err := s.GetGasPrice(ctx)
	if err != nil {
		return false
	}

	return est < s.threshold.Int64()
}

// GetGasPrice gets the estimated gas price for the next 5 blocks
func (s *Service) GetGasPrice(ctx context.Context) (int64, error) {
	nblocks := 2
	addr := builtin.SystemActorAddr // TODO: make real when used in GasEstimateGasPremium

	est, err := s.api.GasEstimateGasPremium(ctx, uint64(nblocks), addr, 10000, types.EmptyTSK)
	if err != nil {
		return 0, err
	}

	fmt.Printf("%d blocks: %s (%s)\n", nblocks, est, types.FIL(est))
	return est.Int64(), nil
}

// LotusClient returns a JSONRPC client for the Lotus API
func LotusClient(ctx context.Context) (lotusapi.FullNode, jsonrpc.ClientCloser, error) {
	authToken := os.Getenv("LOTUS_TOKEN")
	headers := http.Header{"Authorization": []string{"Bearer " + authToken}}
	addr := os.Getenv("LOTUS_API")

	var api lotusapi.FullNodeStruct
	closer, err := jsonrpc.NewMergeClient(ctx, "ws://"+addr+"/rpc/v0", "Filecoin", []interface{}{&api.Internal, &api.CommonStruct.Internal}, headers)

	return &api, closer, err
}

func LotusMinerClient(ctx context.Context) (lotusapi.StorageMiner, jsonrpc.ClientCloser, error) {
	authToken := os.Getenv("LOTUSMINER_TOKEN")
	headers := http.Header{"Authorization": []string{"Bearer " + authToken}}
	addr := os.Getenv("LOTUSMINER_API")

	return client.NewStorageMinerRPCV0(ctx, "ws://"+addr+"/rpc/v0", headers)
}

func GetMinerAddress(ctx context.Context) (address.Address, error) {
	miner, closer, err := LotusMinerClient(ctx)
	if err != nil {
		return address.Address{}, err
	}
	defer closer()

	maddr, err := miner.ActorAddress(ctx)
	if err != nil {
		return address.Address{}, err
	}

	return maddr, nil
}

func (s *Service) GetMinerProvingInfo(ctx context.Context) error {
	head, err := s.api.ChainHead(ctx)
	if err != nil {
		return xerrors.Errorf("getting chain head: %w", err)
	}

	maddr, err := GetMinerAddress(ctx)
	if err != nil {
		return err
	}

	mact, err := s.api.StateGetActor(ctx, maddr, head.Key())
	if err != nil {
		return err
	}

	stor := store.ActorStore(ctx, blockstore.NewAPIBlockstore(s.api))

	mas, err := miner.Load(stor, mact)
	if err != nil {
		return err
	}

	ts, err := s.api.ChainGetTipSet(ctx, head.Key())
	if err != nil {
		return xerrors.Errorf("loading tipset %s: %w", head.Key(), err)
	}

	cd, err := mas.DeadlineInfo(ts.Height())
	if err != nil {
		return xerrors.Errorf("failed to get deadline info: %w", err)
	}

	// cd, err := s.api.StateMinerProvingDeadline(ctx, maddr, head.Key())
	// if err != nil {
	// 	return xerrors.Errorf("getting miner info: %w", err)
	// }

	fmt.Printf("Miner: %s\n", color.BlueString("%s", maddr))

	fmt.Printf("Current Epoch:           %d\n", cd.CurrentEpoch)

	fmt.Printf("Proving Period Boundary: %d\n", cd.PeriodStart%cd.WPoStProvingPeriod)
	fmt.Printf("Proving Period Start:    %s\n", EpochTime(cd.CurrentEpoch, cd.PeriodStart))
	fmt.Printf("Next Period Start:       %s\n\n", EpochTime(cd.CurrentEpoch, cd.PeriodStart+cd.WPoStProvingPeriod))

	fmt.Printf("Deadline Index:       %d\n", cd.Index)
	fmt.Printf("Deadline Open:        %s\n", EpochTime(cd.CurrentEpoch, cd.Open))
	fmt.Printf("Deadline Close:       %s\n", EpochTime(cd.CurrentEpoch, cd.Close))
	fmt.Printf("Deadline Challenge:   %s\n", EpochTime(cd.CurrentEpoch, cd.Challenge))
	fmt.Printf("Deadline FaultCutoff: %s\n", EpochTime(cd.CurrentEpoch, cd.FaultCutoff))

	dl := dline.NewInfo(ts.Height(), 0, ts.Height(), miner.WPoStPeriodDeadlines, miner.WPoStProvingPeriod, miner.WPoStChallengeWindow, miner.WPoStChallengeLookback, miner.FaultDeclarationCutoff)
	fmt.Printf("deadline info for deadline 0: %v", dl)
	fmt.Printf("Deadline Index:       %d\n", dl.Index)
	fmt.Printf("Deadline Open:        %s\n", EpochTime(dl.CurrentEpoch, dl.Open))
	fmt.Printf("Deadline Close:       %s\n", EpochTime(dl.CurrentEpoch, dl.Close))
	fmt.Printf("Deadline Challenge:   %s\n", EpochTime(dl.CurrentEpoch, dl.Challenge))
	fmt.Printf("Deadline FaultCutoff: %s\n", EpochTime(dl.CurrentEpoch, dl.FaultCutoff))

	return nil
}
