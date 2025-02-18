//go:build cgo
// +build cgo

// Package cli implements the CLI app of load balancer
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/mahendrapaipuri/ceems/internal/common"
	internal_runtime "github.com/mahendrapaipuri/ceems/internal/runtime"
	"github.com/mahendrapaipuri/ceems/internal/security"
	ceems_api "github.com/mahendrapaipuri/ceems/pkg/api/cli"
	ceems_http "github.com/mahendrapaipuri/ceems/pkg/api/http"
	ceems_api_models "github.com/mahendrapaipuri/ceems/pkg/api/models"
	lb_backend "github.com/mahendrapaipuri/ceems/pkg/lb/backend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/base"
	"github.com/mahendrapaipuri/ceems/pkg/lb/frontend"
	"github.com/mahendrapaipuri/ceems/pkg/lb/serverpool"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
)

// Custom errors.
var (
	ErrMissingIDs  = errors.New("missing ID for backend(s)")
	ErrMissingURLs = errors.New("missing TSDB and Pyroscope URL(s) for backend(s)")
)

// RetryContextKey is the key used to set context value for retry.
type RetryContextKey struct{}

// CEEMSLBAppConfig contains the configuration of CEEMS load balancer app.
type CEEMSLBAppConfig struct {
	LB       CEEMSLBConfig                  `yaml:"ceems_lb"`
	Server   ceems_api.CEEMSAPIServerConfig `yaml:"ceems_api_server"`
	Clusters []ceems_api_models.Cluster     `yaml:"clusters"`
}

// SetDirectory joins any relative file paths with dir.
func (c *CEEMSLBAppConfig) SetDirectory(dir string) {
	c.Server.Web.HTTPClientConfig.SetDirectory(dir)
}

// Validate valides the CEEMS LB config to check if backend servers have IDs set.
func (c *CEEMSLBAppConfig) Validate() error {
	// Fetch all cluster IDs
	var clusterIDs []string

	for _, cluster := range c.Clusters {
		if cluster.ID != "" {
			clusterIDs = append(clusterIDs, cluster.ID)
		}
	}

	// Preflight checks for backends
	for _, backend := range c.LB.Backends {
		if backend.ID == "" {
			return ErrMissingIDs
		}

		if len(backend.TSDBs) == 0 && len(backend.Pyros) == 0 {
			return ErrMissingURLs
		}

		// Clusters config is not always present. Validate only when it is available
		if len(clusterIDs) > 0 && !slices.Contains(clusterIDs, backend.ID) {
			return fmt.Errorf(
				"unknown ID %s found in backends. IDs in ceems_lb.backends and in clusters config should match",
				backend.ID,
			)
		}
	}

	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *CEEMSLBAppConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Set a default config
	*c = CEEMSLBAppConfig{
		CEEMSLBConfig{
			Strategy: "round-robin",
		},
		ceems_api.CEEMSAPIServerConfig{
			Web: ceems_http.WebConfig{
				HTTPClientConfig: config.DefaultHTTPClientConfig,
			},
		},
		[]ceems_api_models.Cluster{},
	}

	type plain CEEMSLBAppConfig

	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	// Validate backend servers config
	if err := c.Validate(); err != nil {
		return err
	}

	// The UnmarshalYAML method of HTTPClientConfig is not being called because it's not a pointer.
	// We cannot make it a pointer as the parser panics for inlined pointer structs.
	// Thus we just do its validation here.
	if err := c.Server.Web.HTTPClientConfig.Validate(); err != nil {
		return err
	}

	return nil
}

// CEEMSLBConfig contains the CEEMS load balancer config.
type CEEMSLBConfig struct {
	Backends []base.Backend `yaml:"backends"`
	Strategy string         `yaml:"strategy"`
}

// CEEMSLoadBalancer represents the `ceems_lb` cli.
type CEEMSLoadBalancer struct {
	appName string
	App     kingpin.Application
}

// NewCEEMSLoadBalancer returns a new CEEMSLoadBalancer instance.
func NewCEEMSLoadBalancer() (*CEEMSLoadBalancer, error) {
	return &CEEMSLoadBalancer{
		appName: base.CEEMSLoadBalancerAppName,
		App:     base.CEEMSLoadBalancerApp,
	}, nil
}

// Main is the entry point of the `ceems_lb` command.
func (lb *CEEMSLoadBalancer) Main() error {
	var (
		webListenAddresses = lb.App.Flag(
			"web.listen-address",
			"Addresses on which to expose load balancer(s). When both TSDB and Pyroscope LBs are configured, it must be "+
				"repeated to provide two addresses: one for TSDB LB and one for Pyroscope LB. In that case TSDB LB will listen on "+
				"first address and Pyroscope LB on second address",
		).Default(":9030", ":9040").Strings()
		webConfigFile = lb.App.Flag(
			"web.config.file",
			"Path to configuration file that can enable TLS or authentication. See: https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md",
		).Envar("CEEMS_LB_WEB_CONFIG_FILE").Default("").String()
		configFile = lb.App.Flag(
			"config.file",
			"Configuration file path.",
		).Envar("CEEMS_LB_CONFIG_FILE").Default("").String()
		maxProcs = lb.App.Flag(
			"runtime.gomaxprocs", "The target number of CPUs Go will run on (GOMAXPROCS)",
		).Envar("GOMAXPROCS").Default("1").Int()

		// Hidden test flags
		dropPrivs = lb.App.Flag(
			"security.drop-privileges",
			"Drop privileges and run as nobody when LB is started as root.",
		).Default("true").Hidden().Bool()
	)

	// Socket activation only available on Linux
	systemdSocket := func() *bool { b := false; return &b }() //nolint:nlreturn
	if runtime.GOOS == "linux" {
		systemdSocket = lb.App.Flag(
			"web.systemd-socket",
			"Use systemd socket activation listeners instead of port listeners (Linux only).",
		).Hidden().Bool()
	}

	promslogConfig := &promslog.Config{}
	flag.AddFlags(&lb.App, promslogConfig)
	lb.App.Version(version.Print(lb.appName))
	lb.App.UsageWriter(os.Stdout)
	lb.App.HelpFlag.Short('h')

	_, err := lb.App.Parse(os.Args[1:])
	if err != nil {
		return fmt.Errorf("failed to parse CLI flags: %w", err)
	}

	// Get absolute path for web config file if provided
	var webConfigFilePath string
	if *webConfigFile != "" {
		webConfigFilePath, err = filepath.Abs(*webConfigFile)
		if err != nil {
			return fmt.Errorf("failed to get absolute path of the web config file: %w", err)
		}
	}

	// Get absolute config file path global variable that will be used in resource manager
	// and updater packages
	configFilePath, err := filepath.Abs(*configFile)
	if err != nil {
		return fmt.Errorf("failed to get absolute path of the config file: %w", err)
	}

	// Make LB config
	config, err := common.MakeConfig[CEEMSLBAppConfig](configFilePath)
	if err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set logger here after properly configuring promlog
	logger := promslog.New(promslogConfig)

	logger.Info("Starting "+lb.appName, "version", version.Info())
	logger.Info(
		"Operational information", "build_context", version.BuildContext(),
		"host_details", internal_runtime.Uname(), "fd_limits", internal_runtime.FdLimits(),
	)

	runtime.GOMAXPROCS(*maxProcs)
	logger.Debug("Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	// We should STRONGLY advise in docs that CEEMS API server should not be started as root
	// as that will end up dropping the privileges and running it as nobody user which can
	// be strange as CEEMS API server writes data to DB.
	if *dropPrivs {
		securityCfg := &security.Config{
			RunAsUser: "nobody",
			Caps:      nil,
			ReadPaths: []string{webConfigFilePath, configFilePath},
		}

		// Drop all unnecessary privileges
		if err := security.DropPrivileges(securityCfg); err != nil {
			return err
		}
	}

	// Get list of LB types based on provided config
	lbTypes := backendTypes(config)

	var lbNames []string

	for _, t := range lbTypes {
		lbNames = append(lbNames, t.String())
	}

	logger.Info("Load balancers: " + strings.Join(lbNames, ", "))

	// Ensure that enough web listen addresses are provided
	webListenAddrs := *webListenAddresses
	if len(lbTypes) > len(webListenAddrs) {
		logger.Error("Missing web listen addresses", "num_lbs", len(lbTypes), "num_addrs", len(webListenAddrs))

		return fmt.Errorf("insufficient --web.listen-address. Expected %d got %d", len(lbTypes), len(webListenAddrs))
	}

	// Create context that listens for the interrupt signal from the OS.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Make manager and LB maps
	managers := make(map[base.LBType]serverpool.Manager, 2)
	lbs := make(map[base.LBType]frontend.LoadBalancer, 2)

	for i, lbType := range lbTypes {
		// Create a pool of backend servers
		managers[lbType], err = serverpool.New(config.LB.Strategy, logger.With("backend_type", lbType))
		if err != nil {
			logger.Error("Failed to create backend server poo", "backend_type", lbType, "err", err)

			return err
		}

		// Create frontend config for load balancer
		frontendConfig := &frontend.Config{
			Logger:           logger.With("backend_type", lbType),
			LBType:           lbType,
			Address:          webListenAddrs[i],
			WebSystemdSocket: *systemdSocket,
			WebConfigFile:    webConfigFilePath,
			APIServer:        config.Server,
			Manager:          managers[lbType],
		}

		// Create frontend instance for load balancer
		lbs[lbType], err = frontend.New(frontendConfig)
		if err != nil {
			logger.Error("Failed to create load balancer frontend", "backend_type", lbType, "err", err)

			return err
		}

		// Add backend servers to serverPool
		for _, backend := range config.LB.Backends {
			for _, serverCfg := range backendURLs(lbType, backend) {
				backendServer, err := lb_backend.New(lbType, serverCfg, logger.With("backend_type", lbType))
				if err != nil {
					logger.Error("Could not set up backend server", "backend_type", lbType, "err", errors.Unwrap(err))

					continue
				}

				backendServer.ReverseProxy().ErrorHandler = errorHandler(backendServer, lbs[lbType], logger.With("backend_type", lbType))

				managers[lbType].Add(backend.ID, backendServer)
			}
		}

		// Validate configured cluster IDs against the ones in CEEMS DB
		if err := lbs[lbType].ValidateClusterIDs(ctx); err != nil {
			logger.Error("Failed to validate cluster IDs", "backend_type", lbType, "err", errors.Unwrap(err))

			return err
		}
	}

	// Declare wait group and tickers
	var wg sync.WaitGroup

	for _, lbType := range lbTypes {
		// Spawn a go routine to do health checks of backend TSDB servers
		wg.Add(1)

		go func() {
			defer wg.Done()
			monitor(ctx, managers[lbType], logger.With("backend_type", lbType))
		}()

		// Initializing the server in a goroutine so that
		// it won't block the graceful shutdown handling below
		go func() {
			if err := lbs[lbType].Start(); err != nil {
				logger.Error("Failed to start load balancer", "backend_type", lbType, "err", err)
			}
		}()
	}

	// Listen for the interrupt signal.
	<-ctx.Done()

	// Wait for all DB go routines to finish
	wg.Wait()

	// Restore default behavior on the interrupt signal and notify user of shutdown.
	stop()
	logger.Info("Shutting down gracefully, press Ctrl+C again to force")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	shutDownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, lbType := range lbTypes {
		if err := lbs[lbType].Shutdown(shutDownCtx); err != nil {
			logger.Error("Failed to gracefully shutdown LB server", "backend_type", lbType, "err", err)
		}
	}

	logger.Info("Load balancer(s) exiting")
	logger.Info("See you next time!!")

	return nil
}

// backendTypes returns LB backend types in the current config.
func backendTypes(config *CEEMSLBAppConfig) []base.LBType {
	var types []base.LBType
	for _, backend := range config.LB.Backends {
		if len(backend.TSDBs) > 0 && !slices.Contains(types, base.PromLB) {
			types = append(types, base.PromLB)
		}

		if len(backend.Pyros) > 0 && !slices.Contains(types, base.PyroLB) {
			types = append(types, base.PyroLB)
		}
	}

	return types
}

// backendURLs returns slice of backend URLs based on backend type `t`.
func backendURLs(t base.LBType, backend base.Backend) []base.ServerConfig {
	switch t {
	case base.PromLB:
		return backend.TSDBs
	case base.PyroLB:
		return backend.Pyros
	}

	return nil
}

// allowRetry checks if a failed request can be retried.
func allowRetry(r *http.Request) bool {
	if _, ok := r.Context().Value(RetryContextKey{}).(bool); ok {
		return false
	}

	return true
}

// errorHandler returns a custom error handler for reverse proxy.
func errorHandler(backendServer lb_backend.Server, lb frontend.LoadBalancer, logger *slog.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(writer http.ResponseWriter, request *http.Request, err error) {
		logger.Error("Failed to handle the request", "host", backendServer.URL().Host, "err", err)
		backendServer.SetAlive(false)

		// If already retried the request, return error
		if !allowRetry(request) {
			logger.Info("Max retry attempts reached, terminating", "address", request.RemoteAddr, "path", request.URL.Path)
			http.Error(writer, "Service not available", http.StatusServiceUnavailable)

			return
		}

		// Retry request and set context value so that we dont retry for second time
		logger.Info("Attempting retry", "address", request.RemoteAddr, "path", request.URL.Path)
		lb.Serve(
			writer,
			request.WithContext(
				context.WithValue(request.Context(), RetryContextKey{}, true),
			),
		)
	}
}

// monitor checks the backend servers health.
func monitor(ctx context.Context, manager serverpool.Manager, logger *slog.Logger) {
	t := time.NewTicker(time.Second * 20)

	logger.Info("Starting health checker")

	for {
		// This will ensure that we will run the method as soon as go routine
		// starts instead of waiting for ticker to tick
		go healthCheck(ctx, manager, logger)

		select {
		case <-t.C:
			continue
		case <-ctx.Done():
			logger.Info("Received Interrupt. Stopping health checker")

			return
		}
	}
}

// healthCheck monitors the status of all backend servers.
func healthCheck(ctx context.Context, manager serverpool.Manager, logger *slog.Logger) {
	aliveChannel := make(chan bool, 1)

	for id, backends := range manager.Backends() {
		for _, backend := range backends {
			requestCtx, stop := context.WithTimeout(ctx, 10*time.Second)
			defer stop()

			status := "up"

			go isAlive(requestCtx, aliveChannel, backend.URL(), logger)

			select {
			case <-ctx.Done():
				logger.Info("Gracefully shutting down health check")

				return
			case alive := <-aliveChannel:
				backend.SetAlive(alive)

				if !alive {
					status = "down"
				}
			}
			logger.Debug("Health check", "id", id, "backend", backend.String(), "status", status)
		}
	}
}

// isAlive returns the status of backend server with a channel.
func isAlive(ctx context.Context, aliveChannel chan bool, u *url.URL, logger *slog.Logger) {
	var d net.Dialer

	conn, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		logger.Debug("Backend unreachable", "backend", u.Redacted(), "err", err)
		aliveChannel <- false

		return
	}

	_ = conn.Close()
	aliveChannel <- true
}
