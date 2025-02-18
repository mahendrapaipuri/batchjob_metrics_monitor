//go:build cgo
// +build cgo

package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/mahendrapaipuri/ceems/internal/common"
	"github.com/stretchr/testify/require"
)

const mockCEEMSLBAppName = "mockApp"

var mockCEEMSLBApp = *kingpin.New(
	mockCEEMSLBAppName,
	"Mock Load Balancer App.",
)

func queryLB(address, clusterID string) error {
	req, err := http.NewRequest(http.MethodGet, "http://"+address, nil) //nolint:noctx
	if err != nil {
		return err
	}

	req.Header.Add("X-Grafana-User", "usr1")
	req.Header.Add("X-Ceems-Cluster-Id", clusterID)

	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := resp.Body.Close(); err != nil {
		return err
	}

	if want, have := http.StatusOK, resp.StatusCode; want != have {
		return fmt.Errorf("want /metrics status code %d, have %d. Body:\n%s", want, have, b)
	}

	return nil
}

func makeConfigFile(configFile string, tmpDir string) string {
	configPath := filepath.Join(tmpDir, "config.yml")
	os.WriteFile(configPath, []byte(configFile), 0o600)

	return configPath
}

func TestCEEMSLBMainSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	// Start a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("dummy-response"))
	}))
	defer server.Close()

	// Make config file
	configFileTmpl := `
---
ceems_lb:
  strategy: "round-robin"
  backends:
    - id: "default"
      tsdb:
        - web:
           url: %[1]s
      pyroscope:
        - web:
           url: %[1]s`

	configFile := fmt.Sprintf(configFileTmpl, server.URL)
	configFilePath := makeConfigFile(configFile, tmpDir)

	// Remove test related args and add a dummy arg
	os.Args = append(
		[]string{os.Args[0]},
		"--log.level", "debug",
		"--config.file="+configFilePath,
		"--no-security.drop-privileges",
	)
	a := CEEMSLoadBalancer{
		appName: mockCEEMSLBAppName,
		App:     mockCEEMSLBApp,
	}

	// Start Main
	go func() {
		a.Main()
	}()

	// Query LB
	for i := range 10 {
		if err := queryLB("localhost:9040", "default"); err == nil {
			if err := queryLB("localhost:9040", "default"); err == nil {
				break
			}
		}

		time.Sleep(500 * time.Millisecond)

		if i == 9 {
			t.Errorf("Could not start load balancer after %d attempts", i)
		}
	}
}

func TestCEEMSLBMainFailMissingAddr(t *testing.T) {
	tmpDir := t.TempDir()

	// Make config file
	configFile := `
---
ceems_lb:
  strategy: "round-robin"
  backends:
    - id: "default"
      tsdb:
        - web:
           url: localhost:8000
      pyroscope:
	    - web:
           url: localhost:9000`

	configFilePath := makeConfigFile(configFile, tmpDir)

	// Remove test related args and add a dummy arg
	os.Args = []string{
		os.Args[0],
		"--log.level", "debug",
		"--config.file=" + configFilePath,
		"--no-security.drop-privileges",
		"--web.listen-address", ":9030",
	}

	a, err := NewCEEMSLoadBalancer()
	require.NoError(t, err)

	// Run Main
	require.Error(t, a.Main())
}

func TestCEEMSLBMainFail(t *testing.T) {
	// Remove test related args and add a dummy arg
	os.Args = []string{os.Args[0]}
	a, err := NewCEEMSLoadBalancer()
	require.NoError(t, err)

	// Run Main
	require.Error(t, a.Main())
}

func TestCEEMSLBMissingIDs(t *testing.T) {
	tmpDir := t.TempDir()

	// Make config file
	configFile := `
---
ceems_lb:
  strategy: "round-robin"
  backends:
    - tsdb:
        - web:
            url: http://localhost:9090
`

	configFilePath := makeConfigFile(configFile, tmpDir)
	_, err := common.MakeConfig[CEEMSLBAppConfig](configFilePath)
	require.Error(t, err, "missing IDs")
}

func TestCEEMSLBMissingBackends(t *testing.T) {
	tmpDir := t.TempDir()

	// Make config file
	configFile := `
---
ceems_lb:
  strategy: "round-robin"
  backends:
    - id: default
`

	configFilePath := makeConfigFile(configFile, tmpDir)
	_, err := common.MakeConfig[CEEMSLBAppConfig](configFilePath)
	require.Error(t, err, "missing backends")
}

func TestCEEMSLBMainFailMismatchIDs(t *testing.T) {
	tmpDir := t.TempDir()

	// Make config file
	configFile := `
---
ceems_lb:
  strategy: "round-robin"
  backends:
    - id: "default"
      tsdb:
        - web:
            url: http://localhost:9090

clusters:
  - id: "default-1"`

	configFilePath := makeConfigFile(configFile, tmpDir)
	_, err := common.MakeConfig[CEEMSLBAppConfig](configFilePath)
	require.Error(t, err)
}
