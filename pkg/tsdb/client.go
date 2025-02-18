// Package tsdb implements Client client
package tsdb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v3"
)

// Custom errors.
var (
	ErrMissingData         = errors.New("missing data in Client response")
	ErrMissingConfig       = errors.New("global config not found in Client config")
	ErrFailedTypeAssertion = errors.New("failed type assertion")
)

var settingsLock = sync.RWMutex{}

// Metric defines Client metrics.
type Metric map[string]float64

// RangeMetric defines Client range metrics.
type RangeMetric map[string][]model.SamplePair

// Config is Prometheus config representation.
type Config struct {
	Global struct {
		ScrapeInterval     model.Duration `yaml:"scrape_interval"`
		EvaluationInterval model.Duration `yaml:"evaluation_interval"`
	} `yaml:"global"`
}

type Result struct {
	Metric map[string]string `json:"metric"`
	Value  interface{}       `json:"value"`
	Values []interface{}     `json:"values"`
}

type Data struct {
	ResultType string   `json:"resultType"`
	Result     []Result `json:"result"`
}

// Response is the Client response model.
type Response[T any] struct {
	Status    string   `json:"status"`
	Data      T        `json:"data,omitempty"`
	ErrorType string   `json:"errorType,omitempty"`
	Error     string   `json:"error,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

type Settings struct {
	ScrapeInterval     time.Duration
	EvaluationInterval time.Duration
	RateInterval       time.Duration
	QueryLookbackDelta time.Duration
	QueryTimeout       time.Duration
	QueryMaxSamples    uint64
	RetentionPeriod    time.Duration
}

// Client struct.
type Client struct {
	URL              *url.URL
	API              v1.API
	Logger           *slog.Logger
	settingsCache    *Settings
	settingsCacheTTL time.Duration
	lastUpdate       time.Time
	available        bool
}

const (
	// Default intervals. Return them if we cannot fetch config.
	defaultScrapeInterval     = time.Minute
	defaultEvaluationInterval = time.Minute
	defaultLookbackDelta      = 5 * time.Minute
	defaultQueryTimeout       = 2 * time.Minute
	defaultQueryMaxSamples    = 50000000
)

var defaultSettings = Settings{
	ScrapeInterval:     defaultScrapeInterval,
	EvaluationInterval: defaultEvaluationInterval,
	RateInterval:       4 * defaultScrapeInterval,
	QueryLookbackDelta: defaultLookbackDelta,
	QueryTimeout:       defaultQueryTimeout,
	QueryMaxSamples:    uint64(defaultQueryMaxSamples),
}

// New returns a new instance of Client.
func New(webURL string, config config_util.HTTPClientConfig, logger *slog.Logger) (*Client, error) {
	// If webURL is empty return empty struct with available set to false
	if webURL == "" {
		logger.Warn("Client web URL not found")

		return &Client{
			available: false,
		}, nil
	}

	// Unwrap original error to avoid leaking sensitive passwords in output
	tsdbURL, err := url.Parse(webURL)
	if err != nil {
		return nil, errors.Unwrap(err)
	}

	// Create a HTTP roundtripper
	httpRoundTripper, err := config_util.NewRoundTripperFromConfig(config, "tsdb", config_util.WithUserAgent("ceems/tsdb"))
	if err != nil {
		return nil, err
	}

	// Create a new API client
	client, err := api.NewClient(api.Config{
		Address:      tsdbURL.String(),
		RoundTripper: httpRoundTripper,
	})
	if err != nil {
		return nil, err
	}

	// Prometheus API
	api := v1.NewAPI(client)

	return &Client{
		URL:              tsdbURL,
		API:              api,
		Logger:           logger,
		settingsCache:    &defaultSettings,
		settingsCacheTTL: 6 * time.Hour, // Update Client settings for every 6 hours
		available:        true,
	}, nil
}

// String implements stringer method for Client.
func (t *Client) String() string {
	return fmt.Sprintf("Client{URL: %s, available: %t}", t.URL.Redacted(), t.available)
}

// Available returns true if Client is alive.
func (t *Client) Available() bool {
	return t.available
}

// Ping attempts to ping Client.
func (t *Client) Ping() error {
	var d net.Dialer
	// Check if Client is reachable
	conn, err := d.Dial("tcp", t.URL.Host)
	if err != nil {
		return err
	}

	defer conn.Close()

	return nil
}

// Settings returns selected Client config settings.
func (t *Client) Settings(ctx context.Context) *Settings {
	// This must be protected from concurrent access
	settingsLock.Lock()
	defer settingsLock.Unlock()

	// Check if lastUpdate time is more than 3 hrs
	if time.Since(t.lastUpdate) < t.settingsCacheTTL {
		return t.settingsCache
	}

	// Update settings and lastUpdate time
	if settings, err := t.fetchSettings(ctx); err == nil {
		t.lastUpdate = time.Now()
		t.settingsCache = settings
	}

	return t.settingsCache
}

// fetchSettings returns selected Client config parameters.
func (t *Client) fetchSettings(ctx context.Context) (*Settings, error) {
	// Get global config
	c, err := t.API.Config(ctx)
	if err != nil {
		return nil, err
	}

	// Unmarhsall config into struct
	var config Config
	if err := yaml.Unmarshal([]byte(c.YAML), &config); err != nil {
		return nil, err
	}

	// Get flags
	flags, err := t.API.Flags(ctx)
	if err != nil {
		return nil, err
	}

	// Get runtime info
	info, err := t.API.Runtimeinfo(ctx)
	if err != nil {
		return nil, err
	}

	// Make a default settings struct
	settings := defaultSettings
	settings.ScrapeInterval = time.Duration(config.Global.ScrapeInterval)
	settings.EvaluationInterval = time.Duration(config.Global.EvaluationInterval)

	// Get query timeout and max samples from flags
	if v, err := strconv.ParseUint(flags["query.max-samples"], 10, 64); err != nil {
		settings.QueryMaxSamples = v
	}

	if queryTimeout, err := model.ParseDuration(flags["query.timeout"]); err == nil {
		settings.QueryTimeout = time.Duration(queryTimeout)
	}

	if queryTimeout, err := model.ParseDuration(flags["query.lookback-delta"]); err == nil {
		settings.QueryLookbackDelta = time.Duration(queryTimeout)
	}

	var retentionPeriod model.Duration

	// If storageRetention is set to duration ONLY, we can consider it as
	// retention period
	if retentionPeriod, err = model.ParseDuration(info.StorageRetention); err != nil {
		// If storageRetention is set to size or time and size, we need to get
		// "actual" retention period
		for _, retentionString := range strings.Split(info.StorageRetention, "or") {
			retentionPeriod, err = model.ParseDuration(strings.TrimSpace(retentionString))
			if err != nil {
				continue
			}

			break
		}
	}

	settings.RetentionPeriod = time.Duration(retentionPeriod)

	// Set rate interval as 4 times scrape interval (Grafana recommendation)
	settings.RateInterval = 4 * settings.ScrapeInterval

	return &settings, nil
}

// Series makes a Client query to get series.
func (t *Client) Series(ctx context.Context, matchers []string, start time.Time, end time.Time) ([]model.LabelSet, error) {
	// Make API request to get series
	series, warnings, err := t.API.Series(ctx, matchers, start, end)
	if err != nil {
		return nil, err
	}

	if warnings != nil {
		t.Logger.Warn("Client returned warnings for /api/v1/series resource", "warnings", warnings)
	}

	return series, nil
}

// Labels makes a Client query to get list of labels.
func (t *Client) Labels(ctx context.Context, matchers []string, start time.Time, end time.Time) ([]string, error) {
	// Make API request to get labels
	labels, warnings, err := t.API.LabelNames(ctx, matchers, start, end)
	if err != nil {
		return nil, err
	}

	if warnings != nil {
		t.Logger.Warn("Client returned warnings for /api/v1/labels resource", "warnings", warnings)
	}

	return labels, nil
}

// Query makes a Client query.
func (t *Client) Query(ctx context.Context, query string, queryTime time.Time) (Metric, error) {
	// We need to recommend to do it for whole Prometheus instance
	//
	// // Get current scrape interval to use as lookback_delta
	// // This query parameter is undocumented on Prometheus. If we use
	// // default value of 5m, we tend to have metrics 5m **after** compute
	// // unit has finished which gives over estimation of energy
	// if scrapeInterval := t.Settings(ctx).ScrapeInterval; scrapeInterval > 0 {
	// 	values.Add("lookback_delta", scrapeInterval.String())
	// }
	// Make API request to execute query
	result, warnings, err := t.API.Query(ctx, query, queryTime)
	if err != nil {
		return nil, err
	}

	if warnings != nil {
		t.Logger.Warn("Client returned warnings for /api/v1/query resource", "warnings", warnings)
	}

	// Parse data
	queriedValues := make(Metric)

	var values model.Vector

	var ok bool

	// Assert result in vector
	if values, ok = result.(model.Vector); !ok {
		return nil, fmt.Errorf("%w on data: %v", ErrFailedTypeAssertion, result)
	}

	// Iterate over each value and make UUID to value map
	for _, value := range values {
		if uuid, ok := value.Metric["uuid"]; ok {
			queriedValues[string(uuid)] = float64(value.Value)
		}
	}

	return queriedValues, nil
}

// RangeQuery makes a Client range query.
func (t *Client) RangeQuery(
	ctx context.Context,
	query string,
	startTime time.Time,
	endTime time.Time,
	step time.Duration,
) (RangeMetric, error) {
	// We need to recommend to do it for whole Prometheus instance
	//
	// // Get current scrape interval to use as lookback_delta
	// // This query parameter is undocumented on Prometheus. If we use
	// // default value of 5m, we tend to have metrics 5m **after** compute
	// // unit has finished which gives over estimation of energy
	// if scrapeInterval := t.Settings(ctx).ScrapeInterval; scrapeInterval > 0 {
	// 	values.Add("lookback_delta", scrapeInterval.String())
	// }
	// Make query range
	queryRange := v1.Range{
		Start: startTime,
		End:   endTime,
	}

	if step > 0 {
		queryRange.Step = step
	}

	// Make API request to execute query
	result, warnings, err := t.API.QueryRange(ctx, query, queryRange)
	if err != nil {
		return nil, err
	}

	if warnings != nil {
		t.Logger.Warn("Client returned warnings for /api/v1/query_range resource", "warnings", warnings)
	}

	// Parse data
	queriedRangeValues := make(RangeMetric)

	var values model.Matrix

	var ok bool

	// Assert result in matrix
	if values, ok = result.(model.Matrix); !ok {
		return nil, fmt.Errorf("%w on data: %v", ErrFailedTypeAssertion, result)
	}

	// Iterate over each value and make UUID to value map
	for _, value := range values {
		if n, ok := value.Metric["__name__"]; ok {
			queriedRangeValues[string(n)] = value.Values
		}
	}

	return queriedRangeValues, nil
}

// Delete time series with given labels.
func (t *Client) Delete(ctx context.Context, startTime time.Time, endTime time.Time, matchers []string) error {
	// Make API request to execute delete query
	if err := t.API.DeleteSeries(ctx, matchers, startTime, endTime); err != nil {
		return err
	}

	return nil
}
