package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"
)

// InitMiner uses the lotus-miner cli to initialize a miner
func (s *Service) InitMiner(ctx context.Context) error {
	args := []string{"init", "--owner=" + s.owner, "--worker=" + s.worker, "--no-local-storage"}

	cmd := exec.CommandContext(ctx, "lotus-miner", args...)
	cmd.Env = append(os.Environ(), s.MinerPathEnv(), "TRUST_PARAMS=1")
	if debug {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = ioutil.Discard
		cmd.Stderr = ioutil.Discard
	}
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

// RestoreMiner uses the lotus-miner cli to restore a miner
func (s *Service) RestoreMiner(ctx context.Context) error {
	// confirm that there is no lotusminer directory
	if _, err := os.Stat(s.MinerPath()); err == nil {
		return nil
	}

	{
		args := []string{"init", "restore", fmt.Sprintf(home(s.h, ".lotusbackup/%s/bak"), s.worker)}

		cmd := exec.CommandContext(ctx, "lotus-miner", args...)
		cmd.Env = append(os.Environ(), "TRUST_PARAMS=1", s.MinerPathEnv())
		if debug {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		} else {
			cmd.Stdout = ioutil.Discard
			cmd.Stderr = ioutil.Discard
		}
		err := cmd.Run()
		if err != nil {
			return err
		}
	}

	// create empty storage.json file
	err := ioutil.WriteFile(s.MinerPath()+"/storage.json", []byte("{}"), 0644)
	if err != nil {
		return fmt.Errorf("error writing storage.json: %s", err)
	}

	return nil
}

// StartMiner uses the lotus-miner cli to start a miner
func (s *Service) StartMiner(ctx context.Context) error {
	args := []string{"run"}

	cmd := exec.CommandContext(ctx, "lotus-miner", args...)
	cmd.Env = append(os.Environ(), s.MinerPathEnv(), "TRUST_PARAMS=1")
	if debug {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = ioutil.Discard
		cmd.Stderr = ioutil.Discard
	}
	err := cmd.Start()
	if err != nil {
		return err
	}
	time.Sleep(time.Second * 10)
	return nil
}

// StopMiner uses the lotus-miner cli to stop a miner
func (s *Service) StopMiner(ctx context.Context) error {
	args := []string{"stop"}

	cmd := exec.CommandContext(ctx, "lotus-miner", args...)
	cmd.Env = append(os.Environ(), s.MinerPathEnv())
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

// BackupMiner creates a backup of the miner
func (s *Service) BackupMiner(ctx context.Context, inTZ int) error {
	var err error
	// write worker address to file
	if inTZ == 1 {
		err = AppendFile(home(s.h, "keepminer.list"), []byte(fmt.Sprintf("%s\n", s.worker)))
		if err != nil {
			return fmt.Errorf("error appending worker to keepminer.list: %w", err)
		}
	} else if inTZ == 0 {
		err = AppendFile(home(s.h, "sellminer.list"), []byte(fmt.Sprintf("%s\n", s.worker)))
		if err != nil {
			return fmt.Errorf("error appending worker to sellminer.list: %w", err)
		}
	} else {
		err = AppendFile(home(s.h, "backupminer.list"), []byte(fmt.Sprintf("%s\n", s.worker)))
		if err != nil {
			return fmt.Errorf("error appending worker to backupminer.list: %w", err)
		}
	}

	err = os.MkdirAll(fmt.Sprintf(home(s.h, ".lotusbackup/%s"), s.worker), 0755)
	if err != nil {
		return fmt.Errorf("error creating lotusbackup directory: %w", err)
	}

	{
		args := []string{"backup", fmt.Sprintf(home(s.h, ".lotusbackup/%s/bak"), s.worker)}
		cmd := exec.CommandContext(ctx, "lotus-miner", args...)
		cmd.Env = append(os.Environ(), s.MinerPathEnv())
		if debug {
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		} else {
			cmd.Stdout = ioutil.Discard
			cmd.Stderr = ioutil.Discard
		}
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("error running lotus-miner backup: %w", err)
		}
	}

	{
		args := []string{"wallet", "export", s.worker}
		out, err := exec.Command("lotus", args...).Output()
		if err != nil {
			return fmt.Errorf("error running lotus wallet export: %w", err)
		}
		err = ioutil.WriteFile(fmt.Sprintf(home(s.h, ".lotusbackup/%s/key"), s.worker), out, 0644)
		if err != nil {
			return fmt.Errorf("error writing wallet export: %w", err)
		}
	}

	return nil
}

// RemoveMinerDir removes the miner directory
func (s *Service) RemoveMinerDir(ctx context.Context) error {
	backuppath := home(s.h, fmt.Sprintf(".lotusbackup/%s/lotusminer", s.worker))

	err := os.Rename(s.MinerPath(), backuppath)
	if err != nil {
		return fmt.Errorf("error removing lotusminer directory: %w", err)
	}

	return nil
}
