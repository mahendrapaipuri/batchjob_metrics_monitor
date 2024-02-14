// Taken from prometheus-community/ipmi_exporter/blob/master/collector_ipmi.go
// DCMI spec (old) https://www.intel.com/content/dam/www/public/us/en/documents/technical-specifications/dcmi-v1-5-rev-spec.pdf

//go:build !noimpi
// +build !noimpi

package collector

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mahendrapaipuri/ceems/internal/osexec"
	"github.com/prometheus/client_golang/prometheus"
)

const ipmiCollectorSubsystem = "ipmi_dcmi"

type impiCollector struct {
	logger       log.Logger
	hostname     string
	execMode     string
	cachedMetric map[string]float64
	metricDesc   map[string]*prometheus.Desc
}

// Expected output from DCMI spec
// Ref: https://www.intel.com/content/dam/www/public/us/en/documents/technical-specifications/dcmi-v1-5-rev-spec.pdf
// Current Power                        : 164 Watts
// Minimum Power over sampling duration : 48 watts
// Maximum Power over sampling duration : 361 watts
// Average Power over sampling duration : 157 watts
// Time Stamp                           : 12/29/2023 - 08:58:00
// Statistics reporting time period     : 1473439000 milliseconds
// Power Measurement                    : Active

var (
	ipmiDcmiCmd = CEEMSExporterApp.Flag(
		"collector.ipmi.dcmi.cmd",
		"IPMI DCMI command to get system power statistics. Use full path to executables.",
	).Default("/usr/sbin/ipmi-dcmi --get-system-power-statistics").String()
	ipmiDCMIPowerMeasurementRegex = regexp.MustCompile(
		`^Power Measurement\s*:\s*(?P<value>Active|Not\sAvailable).*`,
	)
	ipmiDCMIPowerReadingRegexMap = map[string]*regexp.Regexp{
		"current": regexp.MustCompile(
			`^Current Power\s*:\s*(?P<value>[0-9.]*)\s*[w|W]atts.*`,
		),
		"min": regexp.MustCompile(
			`^Minimum Power over sampling duration\s*:\s*(?P<value>[0-9.]*)\s*[w|W]atts.*`,
		),
		"max": regexp.MustCompile(
			`^Maximum Power over sampling duration\s*:\s*(?P<value>[0-9.]*)\s*[w|W]atts.*`,
		),
	}
)

func init() {
	RegisterCollector(ipmiCollectorSubsystem, defaultEnabled, NewIPMICollector)
}

// NewIPMICollector returns a new Collector exposing IMPI DCMI power metrics.
func NewIPMICollector(logger log.Logger) (Collector, error) {
	var execMode string

	// Initialize metricDesc map
	var metricDesc = make(map[string]*prometheus.Desc, 3)
	var cachedMetric = make(map[string]float64, 3)
	metricDesc["current"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, ipmiCollectorSubsystem, "current_watts_total"),
		"Current Power consumption in watts", []string{"hostname"}, nil,
	)
	metricDesc["min"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, ipmiCollectorSubsystem, "min_watts_total"),
		"Minimum Power consumption in watts", []string{"hostname"}, nil,
	)
	metricDesc["max"] = prometheus.NewDesc(
		prometheus.BuildFQName(Namespace, ipmiCollectorSubsystem, "max_watts_total"),
		"Maximum Power consumption in watts", []string{"hostname"}, nil,
	)

	// Split command
	cmdSlice := strings.Split(*ipmiDcmiCmd, " ")

	// Verify if running ipmiDcmiCmd works
	if _, err := osexec.Execute(cmdSlice[0], cmdSlice[1:], nil, logger); err == nil {
		execMode = "native"
		goto outside
	}

	// If ipmiDcmiCmd failed to run and if sudo is not already present in command,
	// add sudo to command and execute. If current user has sudo rights it will be a success
	if cmdSlice[0] != "sudo" {
		if _, err := osexec.ExecuteWithTimeout("sudo", cmdSlice, 2, nil, logger); err == nil {
			execMode = "sudo"
			goto outside
		}
	}

	// As last attempt, run the command as root user by forking subprocess
	// as root. If there is setuid cap on the process, it will be a success
	if _, err := osexec.ExecuteAs(cmdSlice[0], cmdSlice[1:], 0, 0, nil, logger); err == nil {
		execMode = "cap"
		goto outside
	}

outside:
	collector := impiCollector{
		logger:       logger,
		hostname:     hostname,
		execMode:     execMode,
		metricDesc:   metricDesc,
		cachedMetric: cachedMetric,
	}
	return &collector, nil
}

// Get value based on regex from IPMI output
func getValue(ipmiOutput []byte, regex *regexp.Regexp) (string, error) {
	for _, line := range strings.Split(string(ipmiOutput), "\n") {
		match := regex.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		for i, name := range regex.SubexpNames() {
			if name != "value" {
				continue
			}
			return match[i], nil
		}
	}
	return "", fmt.Errorf("could not find value in output: %s", string(ipmiOutput))
}

// Update implements Collector and exposes IPMI DCMI power related metrics.
func (c *impiCollector) Update(ch chan<- prometheus.Metric) error {
	// Get power consumption from IPMI
	// IPMI commands tend to fail frequently. If that happens we use last cached metric
	powerReadings, err := c.getPowerReadings()
	if err != nil {
		level.Error(c.logger).Log(
			"msg", "Failed to get power statistics from IPMI. Using last cached values",
			"err", err, "cached_metrics", fmt.Sprintf("%#v", c.cachedMetric),
		)
		powerReadings = c.cachedMetric
	}

	// Returned value 0 means Power Measurement is not avail
	for rType, rValue := range powerReadings {
		if rValue > 0 {
			ch <- prometheus.MustNewConstMetric(c.metricDesc[rType], prometheus.CounterValue, float64(rValue), c.hostname)
			c.cachedMetric[rType] = rValue
		}
	}
	return nil
}

// Get current, min and max power readings
func (c *impiCollector) getPowerReadings() (map[string]float64, error) {
	// Execute IPMI command
	ipmiOutput, err := c.executeIPMICmd()
	if err != nil {
		return nil, err
	}

	// Parse IPMI output
	values, err := c.parseIPMIOutput(ipmiOutput)
	if err != nil {
		return nil, err
	}
	return values, nil
}

// Parse current, min and max power readings
func (c *impiCollector) parseIPMIOutput(stdOut []byte) (map[string]float64, error) {
	// Check for Power Measurement are avail
	value, err := getValue(stdOut, ipmiDCMIPowerMeasurementRegex)
	if err != nil {
		return nil, err
	}

	// When Power Measurement in 'Active' state - we can get watts
	var powerReadings = make(map[string]float64, 3)
	if value == "Active" {
		// Get power readings
		for rType, regex := range ipmiDCMIPowerReadingRegexMap {
			if reading, err := getValue(stdOut, regex); err == nil {
				if readingValue, err := strconv.ParseFloat(reading, 64); err == nil {
					powerReadings[rType] = readingValue
				}
			}
		}
		return powerReadings, nil
	}
	return nil, fmt.Errorf("IPMI Power readings not Active")
}

// Execute IPMI command based
func (c *impiCollector) executeIPMICmd() ([]byte, error) {
	var stdOut []byte
	var err error

	// Execute ipmi-dcmi command
	cmdSlice := strings.Split(*ipmiDcmiCmd, " ")
	if c.execMode == "cap" {
		stdOut, err = osexec.ExecuteAs(cmdSlice[0], cmdSlice[1:], 0, 0, nil, c.logger)
	} else if c.execMode == "sudo" {
		stdOut, err = osexec.ExecuteWithTimeout("sudo", cmdSlice, 1, nil, c.logger)
	} else if c.execMode == "native" {
		stdOut, err = osexec.Execute(cmdSlice[0], cmdSlice[1:], nil, c.logger)
	} else {
		err = fmt.Errorf("Current process do not have permissions to execute %s", *ipmiDcmiCmd)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to execute IPMI command: %s", err)
	}
	return stdOut, nil
}
