#!/usr/bin/env bash

set -euf -o pipefail

cd "$(dirname $0)/.."

port="$((10000 + (RANDOM % 10000)))"
tmpdir=$(mktemp -d /tmp/ceems_e2e_test.XXXXXX)

skip_re="^(go_|ceems_exporter_build_info|ceems_scrape_collector_duration_seconds|process_|ceems_textfile_mtime_seconds|ceems_time_(zone|seconds)|ceems_network_(receive|transmit)_(bytes|packets)_total)"

arch="$(uname -m)"

scenario="exporter-cgroups-v1"; keep=0; update=0; verbose=0
while getopts 'hs:kuv' opt
do
  case "$opt" in
    s)
      scenario=$OPTARG
      ;;
    k)
      keep=1
      ;;
    u)
      update=1
      ;;
    v)
      verbose=1
      set -x
      ;;
    *)
      echo "Usage: $0 [-p] [-k] [-u] [-v]"
      echo "  -s: scenario to test [options: exporter, stats]"
      echo "  -k: keep temporary files and leave ceems_{exporter,server} running"
      echo "  -u: update fixtures"
      echo "  -v: verbose output"
      exit 1
      ;;
  esac
done

if [[ "${scenario}" =~ "exporter" ]]
then
  # cgroups_mode=$([ $(stat -fc %T /sys/fs/cgroup/) = "cgroup2fs" ] && echo "unified" || ( [ -e /sys/fs/cgroup/unified/ ] && echo "hybrid" || echo "legacy"))
  # cgroups_mode="legacy"

  if [ "${scenario}" = "exporter-cgroups-v1" ]
  then
    cgroups_mode="legacy"
    desc="Cgroups V1"
    fixture='pkg/collector/fixtures/output/e2e-test-cgroupsv1-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-nvidia" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2"
    fixture='pkg/collector/fixtures/output/e2e-test-cgroupsv2-nvidia-output.txt'
   elif [ "${scenario}" = "exporter-cgroups-v2-amd" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2"
    fixture='pkg/collector/fixtures/output/e2e-test-cgroupsv2-amd-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-nogpu" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 when there are no GPUs"
    fixture='pkg/collector/fixtures/output/e2e-test-cgroupsv2-nogpu-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-procfs" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 using /proc for fetching job properties"
    fixture='pkg/collector/fixtures/output/e2e-test-cgroupsv2-procfs-output.txt'
  elif [ "${scenario}" = "exporter-cgroups-v2-all-metrics" ]
  then
    cgroups_mode="unified"
    desc="Cgroups V2 enabling all available cgroups metrics"
    fixture='pkg/collector/fixtures/output/e2e-test-cgroupsv2-all-metrics-output.txt'
  fi

  # case "${cgroups_mode}" in
  #   legacy|hybrid) fixture='pkg/collector/fixtures/output/e2e-test-cgroupsv1-output.txt' ;;
  #   *) fixture='pkg/collector/fixtures/output/e2e-test-cgroupsv2-output.txt' ;;
  # esac

  logfile="${tmpdir}/ceems_exporter.log"
  fixture_output="${tmpdir}/e2e-test-exporter-output.txt"
  pidfile="${tmpdir}/ceems_exporter.pid"
elif [[ "${scenario}" =~ "stats" ]] 
then

  if [ "${scenario}" = "stats-project-query" ]
  then
    desc="/api/projects end point test"
    fixture='pkg/stats/fixtures/output/e2e-test-stats-server-project-query.txt'
  elif [ "${scenario}" = "stats-uuid-query" ]
  then
    desc="/api/units end point test with uuid query param"
    fixture='pkg/stats/fixtures/output/e2e-test-stats-server-uuid-query.txt'
  # elif [ "${scenario}" = "stats-jobid-query" ]
  # then
  #   desc="/api/units end point test with jobid query param"
  #   fixture='pkg/stats/fixtures/output/e2e-test-stats-server-jobid-query.txt'
  # elif [ "${scenario}" = "stats-jobuuid-jobid-query" ]
  # then
  #   desc="/api/units end point test with jobuuid and jobid query param"
  #   fixture='pkg/stats/fixtures/output/e2e-test-stats-server-jobuuid-jobid-query.txt'
  elif [ "${scenario}" = "stats-admin-query" ]
  then
    desc="/api/units/admin end point test for admin query"
    fixture='pkg/stats/fixtures/output/e2e-test-stats-server-admin-query.txt'
  elif [ "${scenario}" = "stats-admin-query-all" ]
  then
    desc="/api/units/admin end point test for admin query for all jobs"
    fixture='pkg/stats/fixtures/output/e2e-test-stats-server-admin-query-all.txt'
  elif [ "${scenario}" = "stats-admin-denied-query" ]
  then
    desc="/api/units/admin end point test for denied request"
    fixture='pkg/stats/fixtures/output/e2e-test-stats-server-admin--denied-query.txt'
  elif [ "${scenario}" = "stats-current-usage-query" ]
  then
    desc="/api/usage/current end point test"
    fixture='pkg/stats/fixtures/output/e2e-test-stats-server-current-usage-query.txt'
  elif [ "${scenario}" = "stats-global-usage-query" ]
  then
    desc="/api/usage/global end point test"
    fixture='pkg/stats/fixtures/output/e2e-test-stats-server-global-usage-query.txt'
  elif [ "${scenario}" = "stats-current-usage-admin-query" ]
  then
    desc="/api/usage/current/admin end point test"
    fixture='pkg/stats/fixtures/output/e2e-test-stats-server-current-usage-admin-query.txt'
  elif [ "${scenario}" = "stats-global-usage-admin-query" ]
  then
    desc="/api/usage/global/admin end point test"
    fixture='pkg/stats/fixtures/output/e2e-test-stats-server-global-usage-admin-query.txt'
  elif [ "${scenario}" = "stats-current-usage-admin-denied-query" ]
  then
    desc="/api/usage/current/admin end point test"
    fixture='pkg/stats/fixtures/output/e2e-test-stats-server-current-usage-admin-denied-query.txt'
  fi

  logfile="${tmpdir}/ceems_server.log"
  fixture_output="${tmpdir}/e2e-test-stats-server-output.txt"
  pidfile="${tmpdir}/ceems_server.pid"
fi

echo "using scenario: ${scenario}. Description: ${desc}"

finish() {
  if [ $? -ne 0 -o ${verbose} -ne 0 ]
  then
    cat << EOF >&2
LOG =====================
$(cat "${logfile}")
=========================
EOF
  fi

  if [ ${update} -ne 0 ]
  then
    cp "${fixture_output}" "${fixture}"
  fi

  if [ ${keep} -eq 0 ]
  then
    kill -9 "$(cat ${pidfile})"
    # This silences the "Killed" message
    set +e
    wait "$(cat ${pidfile})" > /dev/null 2>&1
    rm -rf "${tmpdir}"
  fi
}

trap finish EXIT

get() {
  if command -v curl > /dev/null 2>&1
  then
    curl -s "$@"
  elif command -v wget > /dev/null 2>&1
  then
    wget -O - "$@"
  else
    echo "Neither curl nor wget found"
    exit 1
  fi
}

waitport() {
  timeout 5 bash -c "while ! curl -s -f "http://localhost:${port}" > /dev/null; do sleep 0.1; done";
  sleep 1
}

if [[ "${scenario}" =~ "exporter" ]] 
then
  if [ ! -x ./bin/ceems_exporter ]
  then
      echo './bin/ceems_exporter not found. Consider running `go build` first.' >&2
      exit 1
  fi

  if [ "${scenario}" = "exporter-cgroups-v1" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/fixtures/sys" \
        --path.cgroupfs="pkg/collector/fixtures/sys/fs/cgroup" \
        --path.procfs="pkg/collector/fixtures/proc" \
        --collector.slurm.create.unique.jobids \
        --collector.slurm.job.props.path="pkg/collector/fixtures/slurmjobprops" \
        --collector.slurm.gpu.type="nvidia" \
        --collector.slurm.nvidia.smi.path="pkg/collector/fixtures/nvidia-smi" \
        --collector.slurm.force.cgroups.version="v1" \
        --collector.slurm.gpu.job.map.path="pkg/collector/fixtures/gpujobmap" \
        --collector.ipmi.dcmi.cmd="pkg/collector/fixtures/ipmi-dcmi-wrapper.sh" \
        --collector.empty.hostname.label \
        --web.listen-address "127.0.0.1:${port}" \
        --log.level="debug" > "${logfile}" 2>&1 &

  elif [ "${scenario}" = "exporter-cgroups-v2-nvidia" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/fixtures/sys" \
        --path.cgroupfs="pkg/collector/fixtures/sys/fs/cgroup" \
        --path.procfs="pkg/collector/fixtures/proc" \
        --collector.slurm.job.props.path="pkg/collector/fixtures/slurmjobprops" \
        --collector.slurm.gpu.type="nvidia" \
        --collector.slurm.nvidia.smi.path="pkg/collector/fixtures/nvidia-smi" \
        --collector.slurm.force.cgroups.version="v2" \
        --collector.slurm.gpu.job.map.path="pkg/collector/fixtures/gpujobmap" \
        --collector.ipmi.dcmi.cmd="pkg/collector/fixtures/ipmi-dcmi-wrapper.sh" \
        --collector.empty.hostname.label \
        --web.listen-address "127.0.0.1:${port}" \
        --log.level="debug" > "${logfile}" 2>&1 &

  elif [ "${scenario}" = "exporter-cgroups-v2-amd" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/fixtures/sys" \
        --path.cgroupfs="pkg/collector/fixtures/sys/fs/cgroup" \
        --path.procfs="pkg/collector/fixtures/proc" \
        --collector.slurm.create.unique.jobids \
        --collector.slurm.job.props.path="pkg/collector/fixtures/slurmjobprops" \
        --collector.slurm.gpu.type="amd" \
        --collector.slurm.rocm.smi.path="pkg/collector/fixtures/rocm-smi" \
        --collector.slurm.force.cgroups.version="v2" \
        --collector.slurm.gpu.job.map.path="pkg/collector/fixtures/gpujobmap" \
        --collector.ipmi.dcmi.cmd="pkg/collector/fixtures/ipmi-dcmi-wrapper.sh" \
        --collector.empty.hostname.label \
        --web.listen-address "127.0.0.1:${port}" \
        --log.level="debug" > "${logfile}" 2>&1 &

  elif [ "${scenario}" = "exporter-cgroups-v2-nogpu" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/fixtures/sys" \
        --path.cgroupfs="pkg/collector/fixtures/sys/fs/cgroup" \
        --path.procfs="pkg/collector/fixtures/proc" \
        --collector.slurm.create.unique.jobids \
        --collector.slurm.job.props.path="pkg/collector/fixtures/slurmjobprops" \
        --collector.slurm.force.cgroups.version="v2" \
        --collector.ipmi.dcmi.cmd="pkg/collector/fixtures/ipmi-dcmi-wrapper.sh" \
        --collector.empty.hostname.label \
        --web.listen-address "127.0.0.1:${port}" \
        --log.level="debug" > "${logfile}" 2>&1 &

  elif [ "${scenario}" = "exporter-cgroups-v2-procfs" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/fixtures/sys" \
        --path.cgroupfs="pkg/collector/fixtures/sys/fs/cgroup" \
        --path.procfs="pkg/collector/fixtures/proc" \
        --collector.slurm.create.unique.jobids \
        --collector.slurm.gpu.type="nvidia" \
        --collector.slurm.nvidia.smi.path="pkg/collector/fixtures/nvidia-smi" \
        --collector.slurm.force.cgroups.version="v2" \
        --collector.ipmi.dcmi.cmd="pkg/collector/fixtures/ipmi-dcmi-wrapper.sh" \
        --collector.empty.hostname.label \
        --web.listen-address "127.0.0.1:${port}" \
        --log.level="debug" > "${logfile}" 2>&1 &
  
  elif [ "${scenario}" = "exporter-cgroups-v2-all-metrics" ] 
  then
      ./bin/ceems_exporter \
        --path.sysfs="pkg/collector/fixtures/sys" \
        --path.cgroupfs="pkg/collector/fixtures/sys/fs/cgroup" \
        --path.procfs="pkg/collector/fixtures/proc" \
        --collector.slurm.create.unique.jobids \
        --collector.slurm.job.props.path="pkg/collector/fixtures/slurmjobprops" \
        --collector.slurm.gpu.type="amd" \
        --collector.slurm.rocm.smi.path="pkg/collector/fixtures/rocm-smi" \
        --collector.slurm.force.cgroups.version="v2" \
        --collector.slurm.gpu.job.map.path="pkg/collector/fixtures/gpujobmap" \
        --collector.slurm.swap.memory.metrics \
        --collector.slurm.psi.metrics \
        --collector.ipmi.dcmi.cmd="pkg/collector/fixtures/ipmi-dcmi-wrapper.sh" \
        --collector.empty.hostname.label \
        --web.listen-address "127.0.0.1:${port}" \
        --log.level="debug" > "${logfile}" 2>&1 &
  fi

  echo $! > "${pidfile}"

  # sleep 1
  waitport

  get "127.0.0.1:${port}/metrics" | grep -E -v "${skip_re}" > "${fixture_output}"
elif [[ "${scenario}" =~ "stats" ]] 
then
  if [ ! -x ./bin/ceems_server ]
  then
      echo './bin/ceems_server not found. Consider running `go build` first.' >&2
      exit 1
  fi

  ./bin/ceems_server \
    --slurm.sacct.path="pkg/stats/fixtures/sacct" \
    --resource.manager.slurm \
    --storage.data.path="${tmpdir}" \
    --storage.data.backup.path="${tmpdir}" \
    --storage.data.backup.interval="2s" \
    --storage.data.skip.delete.old.units \
    --storage.data.cutoff.duration="5m" \
    --test.disable.checks \
    --web.listen-address="127.0.0.1:${port}" \
    --web.admin-users="grafana" \
    --log.level="debug" > "${logfile}" 2>&1 &

  echo $! > "${pidfile}"

  # sleep 2
  waitport

  if [ "${scenario}" = "stats-project-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/projects" > "${fixture_output}"
  elif [ "${scenario}" = "stats-uuid-query" ]
  then
    get -H "X-Grafana-User: usr2" "127.0.0.1:${port}/api/units?uuid=1481508&project=acc2" > "${fixture_output}"
  # elif [ "${scenario}" = "stats-jobid-query" ]
  # then
  #   get -H "X-Grafana-User: usr8" "127.0.0.1:${port}/api/units?jobid=1479765&project=acc1&from=1676934000&to=1677020400" > "${fixture_output}"
  # elif [ "${scenario}" = "stats-jobuuid-jobid-query" ]
  # then
  #   get -H "X-Grafana-User: usr15" "127.0.0.1:${port}/api/units?jobuuid=6311b9ce-d741-d5ba-a27a-9c22eb21254c&jobid=11508&jobid=81510&project=acc1" > "${fixture_output}"
  elif [ "${scenario}" = "stats-admin-query" ]
  then
    get -H "X-Grafana-User: grafana" -H "X-Dashboard-User: usr3" "127.0.0.1:${port}/api/units?project=acc3&from=1676934000&to=1677538800" > "${fixture_output}"
  elif [ "${scenario}" = "stats-admin-query-all" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/units/admin?from=1676934000&to=1677538800" > "${fixture_output}"
  elif [ "${scenario}" = "stats-admin-denied-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/units/admin" > "${fixture_output}"
  elif [ "${scenario}" = "stats-current-usage-query" ]
  then
    get -H "X-Grafana-User: usr3" "127.0.0.1:${port}/api/usage/current?from=1676934000&to=1677538800" > "${fixture_output}"
  elif [ "${scenario}" = "stats-global-usage-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/usage/global" > "${fixture_output}"
  elif [ "${scenario}" = "stats-current-usage-admin-query" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/usage/current/admin?user=usr3&from=1676934000&to=1677538800" > "${fixture_output}"
  elif [ "${scenario}" = "stats-global-usage-admin-query" ]
  then
    get -H "X-Grafana-User: grafana" "127.0.0.1:${port}/api/usage/global/admin" > "${fixture_output}"
  elif [ "${scenario}" = "stats-current-usage-admin-denied-query" ]
  then
    get -H "X-Grafana-User: usr1" "127.0.0.1:${port}/api/usage/global/admin?user=usr2" > "${fixture_output}"
  fi
fi

diff -u \
    "${fixture}" \
    "${fixture_output}"
