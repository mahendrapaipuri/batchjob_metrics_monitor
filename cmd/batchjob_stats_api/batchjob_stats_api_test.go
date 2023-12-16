package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var (
	binary, _ = filepath.Abs("../../bin/batchjob_stats")
)

const (
	address = "localhost:19020"
)

func TestBatchjobStatsExecutable(t *testing.T) {
	fmt.Println(binary)
	if _, err := os.Stat(binary); err != nil {
		t.Skipf("batchjob_stats binary not available, try to run `make build` first: %s", err)
	}
	tmpDir := t.TempDir()
	tmpSacctPath := tmpDir + "/sacct"

	sacctPath, err := filepath.Abs("../../pkg/jobstats/fixtures/sacct")
	if err != nil {
		t.Error(err)
	}
	err = os.Link(sacctPath, tmpSacctPath)
	if err != nil {
		t.Error(err)
	}

	jobstats := exec.Command(
		binary, "--path.data", tmpDir, "--slurm.sacct.path", tmpSacctPath,
		"--web.listen-address", address,
	)
	if err := runCommandAndTests(jobstats); err != nil {
		t.Error(err)
	}
}

func runCommandAndTests(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %s", err)
	}
	return nil
}
