package collector

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"github.com/prometheus/procfs"
)

var (
	metricNameRegex = regexp.MustCompile(`_*[^0-9A-Za-z_]+_*`)
	reParens        = regexp.MustCompile(`\((.*)\)`)
)

// SanitizeMetricName sanitize the given metric name by replacing invalid characters by underscores.
//
// OpenMetrics and the Prometheus exposition format require the metric name
// to consist only of alphanumericals and "_", ":" and they must not start
// with digits. Since colons in MetricFamily are reserved to signal that the
// MetricFamily is the result of a calculation or aggregation of a general
// purpose monitoring system, colons will be replaced as well.
//
// Note: If not subsequently prepending a namespace and/or subsystem (e.g.,
// with prometheus.BuildFQName), the caller must ensure that the supplied
// metricName does not begin with a digit.
func SanitizeMetricName(metricName string) string {
	return metricNameRegex.ReplaceAllString(metricName, "_")
}

// cgroupProcFilterer returns a slice of filtered cgroups based on the presence of targetEnvVars
// in the processes of each cgroup.
func cgroupProcFilterer(cgroups []cgroup, targetEnvVars []string, procFilter func(string) bool) []cgroup {
	// If targetEnvVars is empty return
	if len(targetEnvVars) == 0 {
		return cgroups
	}

	var filteredCgroups []cgroup

	for _, cgrp := range cgroups {
		var filteredProcs []procfs.Proc

		for _, proc := range cgrp.procs {
			// Ignore processes where command line matches the regex
			if procFilter != nil {
				procCmdLine, err := proc.CmdLine()
				if err != nil || len(procCmdLine) == 0 {
					continue
				}

				// Ignore process if matches found
				if procFilter(strings.Join(procCmdLine, " ")) {
					continue
				}
			}

			environ, err := proc.Environ()
			if err != nil {
				continue
			}

			for _, env := range environ {
				for _, targetEnvVar := range targetEnvVars {
					if strings.HasPrefix(env, targetEnvVar) {
						goto add_proc
					}
				}
			}

			// If we didnt find any target env vars, continue to next process
			continue

		add_proc:
			filteredProcs = append(filteredProcs, proc)
		}

		// If there is atleast one process that is filtered, replace procs field
		// in cgroup to filteredProcs and append to filteredCgroups
		if len(filteredProcs) > 0 {
			cgrp.procs = filteredProcs
			filteredCgroups = append(filteredCgroups, cgrp)
		}
	}

	return filteredCgroups
}

// fileExists checks if given file exists or not.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}

	return !info.IsDir()
}

// lookPath is like exec.LookPath but looks only in /sbin, /usr/sbin,
// /usr/local/sbin which are reserved for super user.
func lookPath(f string) (string, error) {
	locations := []string{
		"/sbin",
		"/usr/sbin",
		"/usr/local/sbin",
	}

	for _, path := range locations {
		fullPath := filepath.Join(path, f)
		if fileExists(fullPath) {
			return fullPath, nil
		}
	}

	return "", errors.New("file does not exist")
}

// inode returns the inode of a given path.
func inode(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("error running stat(%s): %w", path, err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("missing syscall.Stat_t in FileInfo for %s", path)
	}

	return stat.Ino, nil
}

// unescapeString sanitizes the string by unescaping UTF-8 characters.
func unescapeString(s string) (string, error) {
	sanitized, err := strconv.Unquote("\"" + s + "\"")
	if err != nil {
		return "", err
	}

	return sanitized, nil
}

// readUintFromFile reads a file and attempts to parse a uint64 from it.
func readUintFromFile(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}

// lookupCgroupRoots walks over the cgroup `rootDir` to check if `name` exists in any
// cgroup path and returns all the found relative root directories.
func lookupCgroupRoots(rootDir string, name string) ([]string, error) {
	var foundCgroupRoots []string

	// Walk through all cgroups and get cgroup paths
	if err := filepath.WalkDir(rootDir, func(p string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Ignore paths that are not directories
		if !info.IsDir() {
			return nil
		}

		// Check if name is in path
		// Once we add the directory to foundCgroupRoots, we need to
		// skip all the sub directories of this directory.
		// We are lookin only for leaf folders
		if strings.Contains(p, name) {
			// Get relative path of cgroup
			rel, err := filepath.Rel(rootDir, p)
			if err != nil {
				return nil //nolint:nilerr
			}

			foundCgroupRoots = append(foundCgroupRoots, rel)

			return filepath.SkipDir
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return foundCgroupRoots, nil
}

// // lookupIPs returns all the IP addresses of the current host.
// // Returns botth IPv4 and IPv6.
// func lookupIPs() ([]string, error) {
// 	ifaces, err := net.Interfaces()
// 	if err != nil {
// 		return nil, err
// 	}

// 	var ipAddrs []string

// 	for _, iface := range ifaces {
// 		if iface.Flags&net.FlagUp == 0 {
// 			continue // interface down
// 		}

// 		if iface.Flags&net.FlagLoopback != 0 {
// 			continue // loopback interface
// 		}

// 		addrs, err := iface.Addrs()
// 		if err != nil {
// 			return nil, err
// 		}

// 		for _, addr := range addrs {
// 			var ip net.IP
// 			switch v := addr.(type) {
// 			case *net.IPNet:
// 				ip = v.IP
// 			case *net.IPAddr:
// 				ip = v.IP
// 			}

// 			if ip == nil || ip.IsLoopback() {
// 				continue
// 			}
// 			// ip = ip.To4()
// 			// if ip == nil {
// 			// 	continue // not an ipv4 address
// 			// }
// 			ipAddrs = append(ipAddrs, ip.String())
// 		}
// 	}

// 	if len(ipAddrs) == 0 {
// 		return nil, errors.New("no IP addresses found on the host")
// 	}

// 	return ipAddrs, nil
// }
