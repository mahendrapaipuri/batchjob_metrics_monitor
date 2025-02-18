// Package tsdb provides the TSDB based updater for CEEMS
package tsdb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/mahendrapaipuri/ceems/pkg/api/helper"
	"github.com/mahendrapaipuri/ceems/pkg/api/models"
	"github.com/mahendrapaipuri/ceems/pkg/api/updater"
	"github.com/mahendrapaipuri/ceems/pkg/tsdb"
	"github.com/prometheus/common/model"
)

// Name of the TSDB updater.
const (
	tsdbUpdaterID = "tsdb"
)

// Use a conservative maximum number of series to be loaded in memory for queries.
const (
	defaultQueryMaxSeries  = 50
	defaultQueryMinSamples = 0.5
)

// config is the container for the configuration of a given TSDB instance.
type tsdbConfig struct {
	QueryMaxSeries  uint64                       `yaml:"query_max_series"`
	QueryMinSamples float64                      `yaml:"query_min_samples"`
	CutoffDuration  model.Duration               `yaml:"cutoff_duration"`
	DeleteIgnore    bool                         `yaml:"delete_ignored"`
	Queries         map[string]map[string]string `yaml:"queries"`
	LabelsToDrop    []string                     `yaml:"labels_to_drop"`
}

// validate validates the config.
func (c *tsdbConfig) validate() error {
	if c.QueryMaxSeries <= 0 {
		return errors.New("query_max_series must be more than 0")
	}

	if c.QueryMinSamples <= 0 || c.QueryMinSamples > 1 {
		return errors.New("query_min_samples must be between (0, 1]")
	}

	return nil
}

// Embed TSDB struct into our TSDBUpdater struct.
type tsdbUpdater struct {
	config *tsdbConfig
	*tsdb.Client
}

// Mutex lock.
var (
	metricLock = sync.RWMutex{}
)

// Register TSDB updater
// tsdb will estimate time averaged metrics and update units struct
// It will also remove ignored units time series.
func init() {
	updater.Register(tsdbUpdaterID, New)
}

// New create a new TSDB updater.
func New(instance updater.Instance, logger *slog.Logger) (updater.Updater, error) {
	// Make TSDB config from instances extra config
	config := tsdbConfig{
		QueryMaxSeries:  defaultQueryMaxSeries,
		QueryMinSamples: defaultQueryMinSamples,
	}
	if err := instance.Extra.Decode(&config); err != nil {
		logger.Error("Failed to setup TSDB updater", "id", instance.ID, "err", err)

		return nil, err
	}

	// Validate config
	if err := config.validate(); err != nil {
		logger.Error("Failed to validate TSDB updater config", "instance_id", instance.ID, "err", err)

		return nil, err
	}

	// Create instances of TSDB
	tsdb, err := tsdb.New(
		instance.Web.URL,
		instance.Web.HTTPClientConfig,
		logger.With("id", instance.ID),
	)
	if err != nil {
		logger.Error("Failed to setup TSDB updater", "instance_id", instance.ID, "err", err)

		return nil, err
	}

	logger.Info("TSDB updater setup successful", "id", instance.ID)

	return &tsdbUpdater{
		&config,
		tsdb,
	}, nil
}

// Update fetches unit metrics from TSDB and update unit struct.
func (t *tsdbUpdater) Update(
	ctx context.Context,
	startTime time.Time,
	endTime time.Time,
	units []models.ClusterUnits,
) []models.ClusterUnits {
	for _, clusterUnit := range units {
		clusterUnit.Units = t.update(ctx, startTime, endTime, clusterUnit.Units)
	}

	return units
}

// Return query string from template.
func (t *tsdbUpdater) queryBuilder(name string, queryTemplate string, data map[string]interface{}) (string, error) {
	tmpl := template.Must(template.New(name).Parse(queryTemplate))
	builder := &strings.Builder{}

	if err := tmpl.Execute(builder, data); err != nil {
		return "", err
	}

	return builder.String(), nil
}

// Get time averaged value of each metric identified by label uuid.
func (t *tsdbUpdater) fetchAggMetrics(
	ctx context.Context,
	queryTime time.Time,
	duration time.Duration,
	uuids []string,
	settings *tsdb.Settings,
) map[string]map[string]tsdb.Metric {
	aggMetrics := make(map[string]map[string]tsdb.Metric, len(t.config.Queries))

	// If duration is less than rateInterval bail
	if duration < settings.RateInterval {
		return aggMetrics
	}

	// UPDATE 20250110: Not necessary anymore as we estimate the batch size dynamically
	// // If duration is more than or equal to 1 day, double scrape and rate intervals
	// // to reduce number of data points Prometheus has to process
	// if duration >= 24*time.Hour {
	// 	scrapeInterval = 2 * scrapeInterval
	// 	evaluationInterval = 2 * evaluationInterval
	// 	rateInterval = 2 * rateInterval
	// }

	// Start a wait group
	var wg sync.WaitGroup
	for _, queries := range t.config.Queries {
		wg.Add(len(queries))
	}

	// Template data
	tmplData := map[string]interface{}{
		"UUIDs":                   strings.Join(uuids, "|"),
		"ScrapeInterval":          settings.ScrapeInterval,
		"ScrapeIntervalMilli":     settings.ScrapeInterval.Milliseconds(),
		"EvaluationInterval":      settings.EvaluationInterval,
		"EvaluationIntervalMilli": settings.EvaluationInterval.Milliseconds(),
		"RateInterval":            settings.RateInterval,
		"Range":                   duration,
	}

	// Loop over t.config.queries map and make queries
	for metricName, queries := range t.config.Queries {
		for subMetricName, query := range queries {
			go func(n string, sn string, q string) {
				defer wg.Done()

				var aggMetric tsdb.Metric

				var err error

				tsdbQuery, err := t.queryBuilder(fmt.Sprintf("%s_%s", n, sn), q, tmplData)
				if err != nil {
					t.Logger.Error(
						"Failed to build query from template", "metric", n,
						"query_template", q, "err", err,
					)

					return
				}

				if aggMetric, err = t.Query(ctx, tsdbQuery, queryTime); err != nil {
					t.Logger.Error(
						"Failed to fetch metrics from TSDB", "metric", n, "duration",
						duration, "scrape_int", settings.ScrapeInterval,
						"rate_int", settings.RateInterval, "err", err,
					)
				} else {
					metricLock.Lock()
					if aggMetrics[n] == nil {
						aggMetrics[n] = make(map[string]tsdb.Metric)
					}

					aggMetrics[n][sn] = aggMetric
					metricLock.Unlock()
				}
			}(metricName, subMetricName, query)
		}
	}

	// Wait for all go routines
	wg.Wait()

	return aggMetrics
}

// Fetch unit metrics from TSDB and update UnitStat struct for each unit.
func (t *tsdbUpdater) update(
	ctx context.Context,
	startTime time.Time,
	endTime time.Time,
	units []models.Unit,
) []models.Unit {
	// Bail if TSDB is unavailable or there are no units to update
	if !t.Available() || len(units) == 0 {
		return units
	}

	// We compute aggregate metrics only for this interval duration and
	// while updating DB we either sum or calculate cumulative average based
	// metric type
	duration := endTime.Sub(startTime).Truncate(time.Minute)

	// Initialize ignored units slice
	var ignoredUnits []string

	allUnitUUIDs := make([]string, len(units))

	var uuid string

	// Loop over all units and find earliest start time of a unit
	j := 0

	for i := range units {
		uuid = units[i].UUID
		// If unit is empty struct ignore
		if uuid == "" {
			continue
		}

		// Ignore units that ran for less than cutoffPeriod seconds and check if
		// unit has end time stamp. If we decide to populate DB with running units,
		// EndTS will be zero as we cannot convert unknown time into time stamp.
		// Check if we EndTS is not zero before ignoring unit. If it is zero, it means
		// it must be RUNNING unit
		//
		// We get the aggregate metrics of these "ignored" comput units as well but
		// we will remove time series of metrics from TSDB as they might be not realiable
		// for small durations
		if units[i].EndedAtTS > 0 {
			if units[i].EndedAtTS-units[i].StartedAtTS < time.Duration(t.config.CutoffDuration).Milliseconds() {
				ignoredUnits = append(ignoredUnits, uuid)
				units[i].Ignore = 1
			}
		}

		allUnitUUIDs[j] = uuid
		j++
	}

	// Get current TSDB settings
	// Get rate and scrape intervals
	settings := t.Settings(ctx)

	// Estimate a batch size based on scrape interval, duration, query max samples and total time series
	samplesPerSeries := max(uint64(duration.Seconds()/settings.ScrapeInterval.Seconds()), 1)
	maxLabels := settings.QueryMaxSamples / (t.config.QueryMaxSeries * samplesPerSeries)
	batchSize := min(max(int(t.config.QueryMinSamples*float64(maxLabels)), 10), len(allUnitUUIDs[:j]))

	// Batch UUIDs into slices of 1000 so that we make TSDB requests for each 1000 units
	// This is to safeguard against OOM errors due to a very large number of units
	// that can spread across big time interval
	uuidBatches := helper.ChunkBy(allUnitUUIDs[:j], batchSize)
	numBatches := len(uuidBatches)

	aggMetrics := make(map[string]map[string]tsdb.Metric)

	// Loop over each chunk
	for iBatch, batchUUIDs := range uuidBatches {
		select {
		case <-ctx.Done():
			t.Logger.Error("Aborting units update", "err", ctx.Err())

			return units
		default:
			// Get aggregate metrics of present chunk
			batchedAggMetrics := t.fetchAggMetrics(ctx, endTime, duration, batchUUIDs, settings)

			// Merge metrics map of each metric type. Metric map has uuid as key and hence
			// merging is safe as UUID is "unique" during the given update interval
			for metricName, metrics := range batchedAggMetrics {
				// If inner map has not been initialized yet, do it
				// These are parent metrics like avg_cpu_usage, avg_gpu_usage
				if aggMetrics[metricName] == nil {
					aggMetrics[metricName] = make(map[string]tsdb.Metric, len(metrics))
				}
				// Each parent metric has sub metrics that operator chooses and we loop
				// over them here
				for subMetricName, subMetrics := range metrics {
					if aggMetrics[metricName][subMetricName] == nil {
						aggMetrics[metricName][subMetricName] = make(tsdb.Metric, len(subMetrics))
					}

					maps.Copy(aggMetrics[metricName][subMetricName], subMetrics)
				}
			}

			t.Logger.Debug(
				"progress", "batch_id", iBatch, "total_batches", numBatches, "batch_size", batchSize,
			)
		}
	}

	// Update all units
	// NOTE: We can improve this by using reflect package by naming queries
	// after field names. That will allow us to dynamically look up struct
	// field using query name and set the properties.
	for i := range units {
		uuid := units[i].UUID

		// Update with CPU metrics
		if metrics, mExists := aggMetrics["avg_cpu_usage"]; mExists {
			units[i].AveCPUUsage = make(models.MetricMap)

			for name, metric := range metrics {
				if value, exists := metric[uuid]; exists {
					units[i].AveCPUUsage[name] = sanitizeValue(value)
				}
			}
		}

		if metrics, mExists := aggMetrics["avg_cpu_mem_usage"]; mExists {
			units[i].AveCPUMemUsage = make(models.MetricMap)

			for name, metric := range metrics {
				if value, exists := metric[uuid]; exists {
					units[i].AveCPUMemUsage[name] = sanitizeValue(value)
				}
			}
		}

		if metrics, mExists := aggMetrics["total_cpu_energy_usage_kwh"]; mExists {
			units[i].TotalCPUEnergyUsage = make(models.MetricMap)

			for name, metric := range metrics {
				if value, exists := metric[uuid]; exists {
					units[i].TotalCPUEnergyUsage[name] = sanitizeValue(value)
				}
			}
		}

		if metrics, mExists := aggMetrics["total_cpu_emissions_gms"]; mExists {
			units[i].TotalCPUEmissions = make(models.MetricMap)

			for name, metric := range metrics {
				if value, exists := metric[uuid]; exists {
					units[i].TotalCPUEmissions[name] = sanitizeValue(value)
				}
			}
		}

		// Update with GPU metrics
		if metrics, mExists := aggMetrics["avg_gpu_usage"]; mExists {
			units[i].AveGPUUsage = make(models.MetricMap)

			for name, metric := range metrics {
				if value, exists := metric[uuid]; exists {
					units[i].AveGPUUsage[name] = sanitizeValue(value)
				}
			}
		}

		if metrics, mExists := aggMetrics["avg_gpu_mem_usage"]; mExists {
			units[i].AveGPUMemUsage = make(models.MetricMap)

			for name, metric := range metrics {
				if value, exists := metric[uuid]; exists {
					units[i].AveGPUMemUsage[name] = sanitizeValue(value)
				}
			}
		}

		if metrics, mExists := aggMetrics["total_gpu_energy_usage_kwh"]; mExists {
			units[i].TotalGPUEnergyUsage = make(models.MetricMap)

			for name, metric := range metrics {
				if value, exists := metric[uuid]; exists {
					units[i].TotalGPUEnergyUsage[name] = sanitizeValue(value)
				}
			}
		}

		if metrics, mExists := aggMetrics["total_gpu_emissions_gms"]; mExists {
			units[i].TotalGPUEmissions = make(models.MetricMap)

			for name, metric := range metrics {
				if value, exists := metric[uuid]; exists {
					units[i].TotalGPUEmissions[name] = sanitizeValue(value)
				}
			}
		}

		// Update with IO metrics
		if metrics, mExists := aggMetrics["total_io_write_stats"]; mExists {
			units[i].TotalIOWriteStats = make(models.MetricMap)

			for name, metric := range metrics {
				if value, exists := metric[uuid]; exists {
					units[i].TotalIOWriteStats[name] = sanitizeValue(value)
				}
			}
		}

		if metrics, mExists := aggMetrics["total_io_read_stats"]; mExists {
			units[i].TotalIOReadStats = make(models.MetricMap)

			for name, metric := range metrics {
				if value, exists := metric[uuid]; exists {
					units[i].TotalIOReadStats[name] = sanitizeValue(value)
				}
			}
		}

		// Update with network metrics
		if metrics, mExists := aggMetrics["total_ingress_stats"]; mExists {
			units[i].TotalIngressStats = make(models.MetricMap)

			for name, metric := range metrics {
				if value, exists := metric[uuid]; exists {
					units[i].TotalIngressStats[name] = sanitizeValue(value)
				}
			}
		}

		if metrics, mExists := aggMetrics["total_outgress_stats"]; mExists {
			units[i].TotalOutgressStats = make(models.MetricMap)

			for name, metric := range metrics {
				if value, exists := metric[uuid]; exists {
					units[i].TotalOutgressStats[name] = sanitizeValue(value)
				}
			}
		}
	}

	// If delete_ignored is set to `true`, drop all the labels with uuid
	// corresponding to ones in ignoredUnits
	var uuidsToDelete []string

	if t.config.DeleteIgnore {
		uuidsToDelete = ignoredUnits
	}

	// Finally delete time series
	if err := t.deleteTimeSeries(ctx, startTime, endTime, uuidsToDelete); err != nil {
		t.Logger.Error("Failed to delete time series in TSDB", "err", err)
	}

	return units
}

// Delete time series data of ignored units.
func (t *tsdbUpdater) deleteTimeSeries(
	ctx context.Context,
	startTime time.Time,
	endTime time.Time,
	unitUUIDs []string,
) error {
	// Check if there are any units to ignore. If there aren't return immediately
	// We shouldnt make a API request to delete with empty units slice as TSDB will
	// match all units during that period with uuid=~"" matcher
	if len(unitUUIDs) == 0 && len(t.config.LabelsToDrop) == 0 {
		return nil
	}

	t.Logger.Debug("TSDB delete time series", "units_ignored", len(unitUUIDs))

	/*
		We should give start and end query params as well. If not, TSDB has to look over
		"all" time blocks (potentially 1000s or more) and try to find the series.
		The thing is the time series data of these "ignored" units should be head block
		as they have started and finished very "recently".

		Imagine we are updating units data for every 15 min and we would like to ignore units
		that have wall time less than 10 min. If we are updating units from, say 10h-10h-15,
		the units that have been ignored cannot start earlier than 9h50 to have finished within
		10h-10h15 window. So, all these time series must be in the head block of TSDB and
		we should provide start and end query params corresponding to
		9h50 (lastupdatetime - ignored unit duration) and current time, respectively. This
		will help TSDB to narrow the search to head block and hence deletion of time series
		will be easy as they are potentially not yet persisted to disk.
	*/
	start := startTime.Add(-time.Duration(t.config.CutoffDuration))
	end := endTime

	// Matcher must be of format "{uuid=~"<regex>"}"
	// Ref: https://ganeshvernekar.com/blog/prometheus-tsdb-queries/
	//
	// Join them with | as delimiter. We will use regex match to match all series
	// with the label uuid=~"$unitids"
	allUUIDs := strings.Join(unitUUIDs, "|")
	matchers := t.config.LabelsToDrop
	matchers = append(matchers, fmt.Sprintf("{uuid=~\"%s\"}", allUUIDs))

	// Make a API request to delete data of ignored units
	return t.Delete(ctx, start, end, matchers)
}

// sanitizeValue verifies if value is either NaN/Inf/-Inf.
// If value is any of these, zero will be returned. Returns 0 if value is negative.
func sanitizeValue(val float64) models.JSONFloat {
	if math.IsNaN(val) || math.IsInf(val, 0) || val < 0 {
		return models.JSONFloat(0)
	}

	return models.JSONFloat(val)
}
