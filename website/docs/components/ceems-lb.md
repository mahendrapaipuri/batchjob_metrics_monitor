---
sidebar_position: 3
---

# CEEMS Load Balancer

## Background

The motivation behind creating CEEMS load balancer component is that neither Prometheus TSDB
nor Grafana Pyroscope enforce any sort of access control over its metrics/profiles querying.
This means once a user has been given the permissions to query a Prometheus TSDB/Grafana
Pyroscope server, they can query _any_ metrics/profiles stored in the server.

Generally, it is not necessary to expose TSDB/Pyroscope server to end users directly and it is done
using Grafana as Prometheus/Pyroscope datasource. Dashboards that are exposed to the end users
need to have query access on the underlying
datasource that the dashboard uses. Although a regular user with
[`Viewer`](https://grafana.com/docs/grafana/latest/administration/roles-and-permissions/access-control/#basic-roles)
role cannot add more panels to an existing dashboard, in order to _view_ the metrics the
user has effectively `query` permissions on the datasource.

This effectively means, the user can make _any_ query to the underlying datasource, _e.g.,_
Prometheus, using the browser cookie that is set by Grafana auth. The consequence is that
the user can query the metrics of _any_ user or _any_ compute unit. Straight forward
solutions to this problem is to create a Prometheus instance for each project/namespace.
However, this is not a scalable solution when there are thousands of projects/namespaces
in a given deployment.

This can pose few issues in multi tenant systems like HPC and cloud computing platforms.
Ideally, we do not want one user to be able to access the compute unit metrics of
other users. CEEMS load balancer component has been created to address this issue.

CEEMS Load Balancer addresses this issue by acting as a gate keeper to introspect the
query before deciding whether to proxy the request to TSDB/Pyroscope or not. It means when a user
makes a TSDB/Pyroscope query for a given compute unit, CEEMS load balancer will check if the user
owns that compute unit by verifying with CEEMS API server.

## Objectives

The main objectives of the CEEMS load balancer are two-fold:

- To provide access control on the TSDB/Pyroscope so that compute units of each project/namespace
are only accessible to the members of that project/namespace
- To provide basic load balancing for replicated TSDB/Pyroscope instances.

Thus, CEEMS load balancer can be configured as Prometheus and Pyroscope data sources in Grafana and
the load balancer will take care of routing traffic to backend TSDB/Pyroscope instances and at
the same time enforcing access control.

## Access control

Besides providing access control to the metrics, CEEMS load balancer black lists most of the
management API of the backend TSDB/Pyroscope servers. This will ensure that end users will not
be able to access API resources that return configuration details, build time/runtime information,
_etc_. CEEMS load balancer only allows handful of API resources for TSDB/Pyroscope that are
strictly necessary to make queries. Currently, the allowed resources are as follows:

### TSDB

- `/api/v1/labels`
- `/api/v1/series`
- `/api/v1/<label_name>/values`
- `/api/v1/query`
- `/api/v1/query_range`

More details on each of these resources can be consulted from
[Prometheus Docs](https://prometheus.io/docs/prometheus/latest/querying/api/#http-api).

### Pyroscope

- `/querier.v1.QuerierService/SelectMergeStacktraces`
- `/querier.v1.QuerierService/LabelNames`
- `/querier.v1.QuerierService/LabelValues`

The API documentation for Pyroscope is very minimal. These are the minimum resources
needed to pull profiling data from Pyroscope server.

## Load balancing

CEEMS load balancer supports classic load balancing strategies like round-robin and least
connection methods. Besides these two, it supports resource based strategy that is
based on retention time. Let's take a look at this strategy in-detail.

:::warning[WARNING]

Resource based load balancing strategy is only supported for TSDB. For Pyroscope,
this strategy is not supported and when used, it will be defaulted to least-connection
strategy.

:::

Taking Prometheus TSDB as an example, Prometheus advises to use local file system to store
the data. This ensure performance and data integrity. However, storing data on local
disk is not fault tolerant unless data is replicated elsewhere. There are cloud native
projects like [Thanos](https://thanos.io/), [Cortex](https://cortexmetrics.io/) to
address this issue. This load balancer is meant
to provide the basic functionality proposed by Thanos, Cortex, _etc_.

The core idea is to replicate the Prometheus data using
[Prometheus' remote write](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write)
functionality onto a remote storage which
is fault tolerant and have higher storage capacity but with a degraded query performance.
In this scenario, we have two TSDBs with following characteristics:

- TSDB using local disk: faster query performance with limited storage space
- TSDB using remote storage: slower query performance with bigger storage space

TSDB using local disk ("hot" instance) will have shorter retention period and the
one using remote storage ("cold" instance)
can have longer retention. CEEMS load balancer is capable of introspecting the query and
then routing the request to either "hot" or "cold" instances of TSDB.

## Multi cluster support

A single deployment of CEEMS load balancer is capable of loading balancing traffic between
different replicated TSDB/Pyroscope instances of multiple clusters. Imagine there are two different
clusters, one for SLURM and one for Openstack, in a DC. Slurm cluster has two dedicated
TSDB/Pyroscope instances where data is replicated between them and the same for Openstack cluster.
Thus, in total, there are four TSDB/Pyroscope instances, two for SLURM cluster and two for
Openstack cluster. A single instance of CEEMS load balancer can route the traffic
between these four different TSDB/Pyroscope instances by targeting the correct cluster.

However, in the production with heavy traffic a single instance of CEEMS load balancer
might not be a optimal solution. In that case, it is however possible to deploy a dedicated
CEEMS load balancer for each cluster.

More details on how to configuration of multi-clusters can be found in [Configuration](../configuration/ceems-lb.md)
section and some example scenarios are discussed in [Advanced](../advanced/multi-cluster.md)
section.
