package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/filecoin-project/go-address"
	jsonrpc "github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/go-state-types/abi"
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
	start     time.Time
	finish    time.Time

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

	start, _ := time.Parse(time.Kitchen, "9:00AM")
	finish, _ := time.Parse(time.Kitchen, "5:00PM")

	return &Service{api: api, closer: closer, threshold: thresholdFIL, start: start, finish: finish, owner: owner}
}

func main() {
	local := []*cli.Command{
		buyCmd,
		infoCmd,
		outputCmd,
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

var outputCmd = &cli.Command{
	Name: "output",
	Action: func(c *cli.Context) error {
		t := time.Unix(1598306400, 0)
		fmt.Println(t.String())
		fmt.Println(EpochTimestamp(abi.ChainEpoch(994203)).String())
		return nil
	},
}

var infoCmd = &cli.Command{
	Name: "info",
	Action: func(c *cli.Context) error {
		ctx := context.Background()

		threshold := os.Getenv("THRESHOLD")
		svc := NewService(ctx, threshold)

		cd, err := svc.GetMinerProvingInfo(ctx)
		if err != nil {
			return err
		}

		fmt.Println(GetZerothDeadlineFromCurrentDeadline(cd).String())

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

			// start lotus-miner process with TRUST_PARAMS=1
			err = svc.StartMiner(ctx)
			if err != nil {
				return err
			}

			content, err := ioutil.ReadFile("~/.lotusminer/token")
			if err != nil {
				log.Printf("reading token failed: %s", err)
				return err
			}
			os.Setenv("MINER_TOKEN", string(content))

			// get the timestamp of the zeroth deadline
			cd, err := svc.GetMinerProvingInfo(ctx)
			if err != nil {
				return err
			}
			zerothDeadline := GetZerothDeadlineFromCurrentDeadline(cd)

			// if the zeroth deadline is between the time range set, backup miner
			if zerothDeadline.Hour() <= svc.start.Hour() && zerothDeadline.Hour() >= svc.finish.Hour() {
				log.Println("backing up miner; in tz")
				svc.BackupMiner(ctx, worker, true)
			} else {
				log.Println("backing up miner; not in tz")
				svc.BackupMiner(ctx, worker, false)
			}
		}
		return nil
	},
}

// BackupMiner creates a backup of the miner
func (svc *Service) BackupMiner(ctx context.Context, worker string, inTZ bool) error {
	err := svc.StopMiner(ctx)
	if err != nil {
		return err
	}

	// write worker address to file
	if inTZ {
		err = ioutil.WriteFile("~/keepminer.list", []byte(fmt.Sprintf("%s\n", worker)), 0644)
		if err != nil {
			return err
		}
	} else {
		err = ioutil.WriteFile("~/sellminer.list", []byte(fmt.Sprintf("%s\n", worker)), 0644)
		if err != nil {
			return err
		}
	}

	err = os.Mkdir(fmt.Sprintf("~/.lotusbackup/%s", worker), 0755)
	if err != nil {
		return err
	}

	{
		args := []string{"backup", fmt.Sprintf("~/.lotusbackup/%s/bak", worker)}
		cmd := exec.CommandContext(ctx, "lotus-miner", args...)
		err = cmd.Run()
		if err != nil {
			return err
		}
	}

	{
		args := []string{"wallet", "export", worker}
		out, err := exec.Command("lotus", args...).Output()
		if err != nil {
			return err
		}
		err = ioutil.WriteFile(fmt.Sprintf("~/.lotusbackup/%s/key", worker), out, 0644)
		if err != nil {
			return err
		}
	}

	err = os.RemoveAll("~/.lotusminer")
	if err != nil {
		return err
	}

	return nil
}

// InitMiner uses the lotus-miner cli to initialize a miner
func (s *Service) InitMiner(ctx context.Context, worker string) error {
	args := []string{"init", "--owner=" + s.owner, "--worker=" + worker, "--no-local-storage"}

	cmd := exec.CommandContext(ctx, "lotus-miner", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

// StartMiner uses the lotus-miner cli to start a miner
func (s *Service) StartMiner(ctx context.Context) error {
	os.Setenv("TRUST_PARAMS", "1")
	args := []string{"run"}

	cmd := exec.CommandContext(ctx, "lotus-miner", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		return err
	}
	return nil
}

// StopMiner uses the lotus-miner cli to stop a miner
func (s *Service) StopMiner(ctx context.Context) error {
	args := []string{"stop"}
	cmd := exec.CommandContext(ctx, "lotus-miner", args...)
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

func (s *Service) GetMinerProvingInfo(ctx context.Context) (*dline.Info, error) {
	head, err := s.api.ChainHead(ctx)
	if err != nil {
		return nil, xerrors.Errorf("getting chain head: %w", err)
	}

	maddr, err := GetMinerAddress(ctx)
	if err != nil {
		return nil, err
	}

	mact, err := s.api.StateGetActor(ctx, maddr, head.Key())
	if err != nil {
		return nil, err
	}

	stor := store.ActorStore(ctx, blockstore.NewAPIBlockstore(s.api))

	mas, err := miner.Load(stor, mact)
	if err != nil {
		return nil, err
	}

	ts, err := s.api.ChainGetTipSet(ctx, head.Key())
	if err != nil {
		return nil, xerrors.Errorf("loading tipset %s: %w", head.Key(), err)
	}

	cd, err := mas.DeadlineInfo(ts.Height())
	if err != nil {
		return nil, xerrors.Errorf("failed to get deadline info: %w", err)
	}

	return cd, nil
}

// GetZerothDeadlineFromCurrentDeadline returns the hour of day that the zeroth deadline
// gets challenged
func GetZerothDeadlineFromCurrentDeadline(dl *dline.Info) time.Time {
	di0do := dl.CurrentEpoch - (dl.CurrentEpoch - dl.Open + abi.ChainEpoch(int64(dl.Index)*int64(miner.WPoStChallengeWindow)))
	return EpochTimestamp(di0do)
}
