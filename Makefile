# Nicked from node_exporter repo and modified for current repo needs

# Ensure that 'all' is the default target otherwise it will be the first target from Makefile.common.
all::

# Needs to be defined before including Makefile.common to auto-generate targets
DOCKER_ARCHS ?= amd64 arm64

PROMTOOL_VERSION ?= 2.50.0
PROMTOOL_URL     ?= https://github.com/prometheus/prometheus/releases/download/v$(PROMTOOL_VERSION)/prometheus-$(PROMTOOL_VERSION).$(GO_BUILD_PLATFORM).tar.gz
PROMTOOL         ?= $(FIRST_GOPATH)/bin/promtool

PREFIX           := $(shell pwd)/bin

TEST_DOCKER             ?= false
DOCKER_IMAGE_NAME       ?= ceems
MACH                    ?= $(shell uname -m)
CGROUPS_MODE            ?= $([ $(stat -fc %T /sys/fs/cgroup/) = "cgroup2fs" ] && echo "unified" || ( [ -e /sys/fs/cgroup/unified/ ] && echo "hybrid" || echo "legacy"))

STATICCHECK_IGNORE =

CGO_BUILD               ?= 0
RELEASE_BUILD           ?= 0

# Swagger docs
SWAGGER_DIR     ?= pkg/api/http
SWAGGER_MAIN    ?= server.go

include Makefile.common

ifeq ($(GOHOSTOS), linux)
	test-e2e := test-e2e
else
	test-e2e := skip-test-e2e
endif

ifeq ($(TEST_DOCKER), false)
	test-docker := skip-test-docker
else
	test-docker := test-docker
endif

# Use CGO for api and GO for ceems_exporter.
PROMU_TEST_CONF ?= .promu-go-test.yml
ifeq ($(CGO_BUILD), 1)
	PROMU_CONF ?= .promu-cgo.yml
	pkgs := ./pkg/sqlite3 ./pkg/api/cli \
			./pkg/api/db ./pkg/api/helper \
			./pkg/api/resource ./pkg/api/resource/slurm \
			./pkg/api/updater \
			./pkg/api/http ./cmd/ceems_api_server \
			./pkg/lb/backend ./pkg/lb/cli \
			./pkg/lb/frontend ./pkg/lb/serverpool \
			./cmd/ceems_lb
	checkmetrics := skip-checkmetrics
	checkrules := skip-checkrules

	# go test flags
	coverage-file := coverage-cgo.out
else
	PROMU_CONF ?= .promu-go.yml
	pkgs := ./pkg/collector ./pkg/emissions ./pkg/tsdb ./pkg/grafana \
			./internal/common ./internal/osexec ./internal/structset \
			./cmd/ceems_exporter
	checkmetrics := checkmetrics
	checkrules := checkrules

	# go test flags
	coverage-file := coverage-go.out
endif
test-flags := -covermode=atomic -coverprofile=$(coverage-file).tmp -race

ifeq ($(GOHOSTOS), linux)
	test-e2e := test-e2e
else
	test-e2e := skip-test-e2e
endif

PROMU := $(FIRST_GOPATH)/bin/promu --config $(PROMU_CONF)
PROMU_TEST := $(FIRST_GOPATH)/bin/promu --config $(PROMU_TEST_CONF)

e2e-out = pkg/collector/testdata/output

# 64bit -> 32bit mapping for cross-checking. At least for amd64/386, the 64bit CPU can execute 32bit code but not the other way around, so we don't support cross-testing upwards.
cross-test = skip-test-32bit
define goarch_pair
	ifeq ($$(GOHOSTOS), linux)
		ifndef CGO_BUILD
			ifeq ($$(GOHOSTARCH), $1)
				GOARCH_CROSS = $2
				cross-test = test-32bit
			endif
		endif
	endif
endef

# By default, "cross" test with ourselves to cover unknown pairings.
$(eval $(call goarch_pair,amd64,386))
$(eval $(call goarch_pair,mips64,mips))
$(eval $(call goarch_pair,mips64el,mipsel))

all:: vet common-all $(cross-test) $(test-docker) $(checkmetrics) $(checkrules) $(test-e2e)

.PHONY: coverage
coverage:
	@echo ">> getting coverage report"
	tail -n +2 coverage-cgo.out > coverage-cgo.tmp.out && mv coverage-cgo.tmp.out coverage-cgo.out
	cat coverage-go.out coverage-cgo.out > coverage.out
	$(GO) tool cover -func=coverage.out -o=coverage.out

.PHONY: test
test: pkg/collector/testdata/sys/.unpacked pkg/collector/testdata/proc/.unpacked
	@echo ">> running tests"
	$(GO) test -short $(test-flags) $(pkgs)
	cat $(coverage-file).tmp | grep -v "main.go" > $(coverage-file)

.PHONY: test-32bit
test-32bit: pkg/collector/testdata/sys/.unpacked 
	@echo ">> running tests in 32-bit mode"
	@env GOARCH=$(GOARCH_CROSS) $(GO) test $(pkgs)

.PHONY: skip-test-32bit
skip-test-32bit:
	@echo ">> SKIP running tests in 32-bit mode: not supported on $(GOHOSTOS)/$(GOHOSTARCH)"

%/.unpacked: %.ttar
	@echo ">> extracting testdata"
	if [ -d $(dir $@) ] ; then rm -rf $(dir $@) ; fi
	./scripts/ttar -C $(dir $*) -x -f $*.ttar
	touch $@

update_testdata:
	rm -vf pkg/collector/testdata/sys/.unpacked pkg/collector/testdata/proc/.unpacked
	./scripts/ttar -C pkg/collector/testdata -c -f pkg/collector/testdata/sys.ttar sys
	./scripts/ttar -C pkg/collector/testdata -c -f pkg/collector/testdata/proc.ttar proc

ifeq ($(CGO_BUILD), 0)
.PHONY: test-e2e
test-e2e: build pkg/collector/testdata/sys/.unpacked pkg/collector/testdata/proc/.unpacked
	@echo ">> running end-to-end tests"
	./scripts/e2e-test.sh -s exporter-cgroups-v1
	./scripts/e2e-test.sh -s exporter-cgroups-v2-nvidia-ipmiutil
	./scripts/e2e-test.sh -s exporter-cgroups-v2-amd-ipmitool
	./scripts/e2e-test.sh -s exporter-cgroups-v2-nogpu
	./scripts/e2e-test.sh -s exporter-cgroups-v2-procfs
	./scripts/e2e-test.sh -s exporter-cgroups-v2-all-metrics
else
.PHONY: test-e2e
test-e2e: $(PROMTOOL) build pkg/collector/testdata/sys/.unpacked pkg/collector/testdata/proc/.unpacked
	@echo ">> running end-to-end tests"
	./scripts/e2e-test.sh -s api-project-query
	./scripts/e2e-test.sh -s api-project-empty-query
	./scripts/e2e-test.sh -s api-project-admin-query
	./scripts/e2e-test.sh -s api-user-query
	./scripts/e2e-test.sh -s api-user-admin-query
	./scripts/e2e-test.sh -s api-cluster-admin-query
	./scripts/e2e-test.sh -s api-uuid-query
	./scripts/e2e-test.sh -s api-running-query
	./scripts/e2e-test.sh -s api-admin-query
	./scripts/e2e-test.sh -s api-admin-query-all
	./scripts/e2e-test.sh -s api-admin-query-all-selected-fields
	./scripts/e2e-test.sh -s api-admin-denied-query
	./scripts/e2e-test.sh -s api-current-usage-query
	./scripts/e2e-test.sh -s api-global-usage-query
	./scripts/e2e-test.sh -s api-current-usage-admin-query
	./scripts/e2e-test.sh -s api-global-usage-admin-query
	./scripts/e2e-test.sh -s api-current-usage-admin-denied-query
	./scripts/e2e-test.sh -s api-current-stats-admin-query
	./scripts/e2e-test.sh -s api-global-stats-admin-query
	./scripts/e2e-test.sh -s api-verify-pass-query
	./scripts/e2e-test.sh -s api-verify-fail-query
	./scripts/e2e-test.sh -s api-demo-units-query
	./scripts/e2e-test.sh -s api-demo-usage-query
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-basic
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-forbid-user-query-db
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-allow-user-query-db
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-forbid-user-query-api
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-allow-user-query-api
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-allow-admin-query
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-auth
endif

ifeq ($(CGO_BUILD), 0)
.PHONY: test-e2e-update
test-e2e-update: build pkg/collector/testdata/sys/.unpacked pkg/collector/testdata/proc/.unpacked
	@echo ">> updating end-to-end tests outputs"
	./scripts/e2e-test.sh -s exporter-cgroups-v1 -u || true
	./scripts/e2e-test.sh -s exporter-cgroups-v2-nvidia-ipmiutil -u || true
	./scripts/e2e-test.sh -s exporter-cgroups-v2-amd-ipmitool -u || true
	./scripts/e2e-test.sh -s exporter-cgroups-v2-nogpu -u || true
	./scripts/e2e-test.sh -s exporter-cgroups-v2-procfs -u || true
	./scripts/e2e-test.sh -s exporter-cgroups-v2-all-metrics -u || true
else
.PHONY: test-e2e-update
test-e2e-update: $(PROMTOOL) build pkg/collector/testdata/sys/.unpacked pkg/collector/testdata/proc/.unpacked
	@echo ">> updating end-to-end tests outputs"
	./scripts/e2e-test.sh -s api-project-query -u || true
	./scripts/e2e-test.sh -s api-project-empty-query -u || true
	./scripts/e2e-test.sh -s api-project-admin-query -u || true
	./scripts/e2e-test.sh -s api-user-query -u || true
	./scripts/e2e-test.sh -s api-user-admin-query -u || true
	./scripts/e2e-test.sh -s api-cluster-admin-query -u || true
	./scripts/e2e-test.sh -s api-uuid-query -u || true
	./scripts/e2e-test.sh -s api-running-query -u || true
	./scripts/e2e-test.sh -s api-admin-query -u || true
	./scripts/e2e-test.sh -s api-admin-query-all -u || true
	./scripts/e2e-test.sh -s api-admin-query-all-selected-fields -u || true
	./scripts/e2e-test.sh -s api-admin-denied-query -u || true
	./scripts/e2e-test.sh -s api-current-usage-query -u || true
	./scripts/e2e-test.sh -s api-global-usage-query -u || true
	./scripts/e2e-test.sh -s api-current-usage-admin-query -u || true
	./scripts/e2e-test.sh -s api-global-usage-admin-query -u || true
	./scripts/e2e-test.sh -s api-current-usage-admin-denied-query -u || true
	./scripts/e2e-test.sh -s api-current-stats-admin-query -u || true
	./scripts/e2e-test.sh -s api-global-stats-admin-query -u || true
	./scripts/e2e-test.sh -s api-verify-pass-query -u || true
	./scripts/e2e-test.sh -s api-verify-fail-query -u || true
	./scripts/e2e-test.sh -s api-demo-units-query -u || true
	./scripts/e2e-test.sh -s api-demo-usage-query -u || true
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-basic -u || true
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-forbid-user-query-db -u || true
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-allow-user-query-db -u || true
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-forbid-user-query-api -u || true
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-allow-user-query-api -u || true
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-allow-admin-query -u || true
	@env GOBIN=$(FIRST_GOPATH) ./scripts/e2e-test.sh -s lb-auth -u || true
endif

.PHONY: skip-test-e2e
skip-test-e2e:
	@echo ">> SKIP running end-to-end tests on $(GOHOSTOS)"

.PHONY: checkmetrics
checkmetrics: $(PROMTOOL)
	@echo ">> checking metrics for correctness"
	./scripts/checkmetrics.sh $(PROMTOOL) $(e2e-out)

.PHONY: skip-checkmetrics
skip-checkmetrics: $(PROMTOOL)
	@echo ">> SKIP checking metrics for correctness"

.PHONY: checkrules
checkrules: $(PROMTOOL)
	@echo ">> checking rules for correctness"
	find . -name "*.rules" | xargs -I {} $(PROMTOOL) check rules {}

.PHONY: skip-checkrules
skip-checkrules: $(PROMTOOL)
	@echo ">> SKIP checking rules for correctness"

.PHONY: test-docker
test-docker:
	@echo ">> testing docker image"
	./scripts/test_image.sh "$(DOCKER_REPO)/$(DOCKER_IMAGE_NAME)-linux-amd64:$(DOCKER_IMAGE_TAG)" 9010 ceems_exporter
	./scripts/test_image.sh "$(DOCKER_REPO)/$(DOCKER_IMAGE_NAME)-linux-amd64:$(DOCKER_IMAGE_TAG)" 9020 ceems_api_server
	./scripts/test_image.sh "$(DOCKER_REPO)/$(DOCKER_IMAGE_NAME)-linux-amd64:$(DOCKER_IMAGE_TAG)" 9030 ceems_lb

.PHONY: skip-test-docker
skip-test-docker:
	@echo ">> SKIP running docker tests"

.PHONY: promtool
promtool: $(PROMTOOL)
$(PROMTOOL):
	mkdir -p $(FIRST_GOPATH)/bin
	curl -fsS -L $(PROMTOOL_URL) | tar -xvzf - -C $(FIRST_GOPATH)/bin --strip 1 "prometheus-$(PROMTOOL_VERSION).$(GO_BUILD_PLATFORM)/promtool" 
	curl -fsS -L $(PROMTOOL_URL) | tar -xvzf - -C $(FIRST_GOPATH)/bin --strip 1 "prometheus-$(PROMTOOL_VERSION).$(GO_BUILD_PLATFORM)/prometheus" 
