package slurm

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/mahendrapaipuri/ceems/internal/security"
	"github.com/mahendrapaipuri/ceems/pkg/api/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreflightsCLI(t *testing.T) {
	manager := slurmScheduler{
		logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		securityContexts: make(map[string]*security.SecurityContext),
	}
	err := preflightsCLI(&manager)
	require.Error(t, err)

	// Add sacct command to PATH
	sacctPath, _ := filepath.Abs("../../testdata")
	t.Setenv("PATH", fmt.Sprintf("%s:%s", os.Getenv("PATH"), sacctPath))

	err = preflightsCLI(&manager)
	require.NoError(t, err)
	assert.Equal(t, sacctPath, manager.cluster.CLI.Path)
}

func TestParseSacctCmdOutput(t *testing.T) {
	units, numUnits := parseSacctCmdOutput(sacctCmdOutput, start, end)
	require.ElementsMatch(t, units, expectedBatchJobs)
	require.Equal(t, 2, numUnits)

	// Job finished in past
	sacctCmdOutput1 := `1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-20T14:37:02+0100|2023-02-20T14:37:07+0100|2023-02-20T15:37:07+0100|01:49:22|3000|0:0|RUNNING|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr`
	units, _ = parseSacctCmdOutput(sacctCmdOutput1, start, end)
	// Check if elapsed time corresponds to real elapsed time of job
	assert.InEpsilon(t, 3600, float64(units[0].TotalTime["walltime"]), 0)

	// Job created but not started
	sacctCmdOutput2 := `1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T14:37:02+0100|NA|NA|01:49:22|3000|0:0|PENDING|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr`
	units, _ = parseSacctCmdOutput(sacctCmdOutput2, start, end)
	// Check if elapsed time corresponds to real elapsed time of job
	assert.Equal(t, 0, int(units[0].TotalTime["walltime"]))

	// Job started inside current interval
	sacctCmdOutput3 := `1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T15:10:00+0100|2023-02-21T15:10:00+0100|NA|01:49:22|3000|0:0|RUNNING|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr`
	units, _ = parseSacctCmdOutput(sacctCmdOutput3, start, end)
	// Check if elapsed time corresponds to real elapsed time of job
	assert.InEpsilon(t, 300, float64(units[0].TotalTime["walltime"]), 0)

	// Job ended inside current interval
	sacctCmdOutput4 := `1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T14:10:00+0100|2023-02-21T14:10:00+0100|2023-02-21T15:10:00+0100|01:49:22|3000|0:0|COMPLETED|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr`
	units, _ = parseSacctCmdOutput(sacctCmdOutput4, start, end)
	// Check if elapsed time corresponds to real elapsed time of job
	assert.InEpsilon(t, 600, float64(units[0].TotalTime["walltime"]), 0)

	// Job started and ended inside current interval
	sacctCmdOutput5 := `1479763|part1|qos1|acc1|grp|1000|usr|1000|2023-02-21T15:10:00+0100|2023-02-21T15:10:00+0100|2023-02-21T15:12:00+0100|01:49:22|3000|0:0|COMPLETED|billing=80,cpu=160,energy=1439089,gres/gpu=8,mem=320G,node=2|compute-0|test_script1|/home/usr`
	units, _ = parseSacctCmdOutput(sacctCmdOutput5, start, end)
	// Check if elapsed time corresponds to real elapsed time of job
	assert.InEpsilon(t, 120, float64(units[0].TotalTime["walltime"]), 0)
}

func TestParseSacctMgrCmdOutput(t *testing.T) {
	users, projects := parseSacctMgrCmdOutput(sacctMgrCmdOutput, current.Format(base.DatetimezoneLayout))
	require.ElementsMatch(t, expectedUsers, users)
	require.ElementsMatch(t, expectedProjects, projects)
}
