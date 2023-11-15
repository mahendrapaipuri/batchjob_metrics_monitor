# Nicked from node_exporter repo and modified to current exporter

# Ensure that 'all' is the default target otherwise it will be the first target from Makefile.common.
all::

# Needs to be defined before including Makefile.common to auto-generate targets
DOCKER_ARCHS ?= amd64 armv7 arm64 ppc64le s390x

include Makefile.common

PROMTOOL_VERSION ?= 2.30.0
PROMTOOL_URL     ?= https://github.com/prometheus/prometheus/releases/download/v$(PROMTOOL_VERSION)/prometheus-$(PROMTOOL_VERSION).$(GO_BUILD_PLATFORM).tar.gz
PROMTOOL         ?= $(FIRST_GOPATH)/bin/promtool

DOCKER_IMAGE_NAME       ?= batchjob-exporter
MACH                    ?= $(shell uname -m)

STATICCHECK_IGNORE =

ifeq ($(GOHOSTOS), linux)
	test-e2e := test-e2e
else
	test-e2e := skip-test-e2e
endif

# Use CGO for non-Linux builds.
ifeq ($(GOOS), linux)
	PROMU_CONF ?= .promu.yml
else
	ifndef GOOS
		ifeq ($(GOHOSTOS), linux)
			PROMU_CONF ?= .promu.yml
		else
			PROMU_CONF ?= .promu-cgo.yml
		endif
	else
		# Do not use CGO for openbsd/amd64 builds
		ifeq ($(GOOS), openbsd)
			ifeq ($(GOARCH), amd64)
				PROMU_CONF ?= .promu.yml
			else
				PROMU_CONF ?= .promu-cgo.yml
			endif
		else
			PROMU_CONF ?= .promu-cgo.yml
		endif
	endif
endif

PROMU := $(FIRST_GOPATH)/bin/promu --config $(PROMU_CONF)

e2e-out = collector/fixtures/e2e-test-output.txt

# 64bit -> 32bit mapping for cross-checking. At least for amd64/386, the 64bit CPU can execute 32bit code but not the other way around, so we don't support cross-testing upwards.
cross-test = skip-test-32bit
define goarch_pair
	ifeq ($$(GOHOSTOS),linux)
		ifeq ($$(GOHOSTARCH),$1)
			GOARCH_CROSS = $2
			cross-test = test-32bit
		endif
	endif
endef

# By default, "cross" test with ourselves to cover unknown pairings.
$(eval $(call goarch_pair,amd64,386))
$(eval $(call goarch_pair,mips64,mips))
$(eval $(call goarch_pair,mips64el,mipsel))

all:: vet checkmetrics checkrules common-all $(cross-test) $(test-e2e)

.PHONY: test
test: collector/fixtures/sys/.unpacked 
	@echo ">> running tests"
	$(GO) test -short $(test-flags) $(pkgs)

.PHONY: test-32bit
test-32bit: collector/fixtures/sys/.unpacked 
	@echo ">> running tests in 32-bit mode"
	@env GOARCH=$(GOARCH_CROSS) $(GO) test $(pkgs)

.PHONY: skip-test-32bit
skip-test-32bit:
	@echo ">> SKIP running tests in 32-bit mode: not supported on $(GOHOSTOS)/$(GOHOSTARCH)"

%/.unpacked: %.ttar
	@echo ">> extracting fixtures"
	if [ -d $(dir $@) ] ; then rm -rf $(dir $@) ; fi
	./ttar -C $(dir $*) -x -f $*.ttar
	touch $@

update_fixtures:
	rm -vf collector/fixtures/sys/.unpacked
	./ttar -C collector/fixtures -c -f collector/fixtures/sys.ttar sys


.PHONY: test-e2e
test-e2e: build collector/fixtures/sys/.unpacked 
	@echo ">> running end-to-end tests"
	./e2e-test.sh

.PHONY: skip-test-e2e
skip-test-e2e:
	@echo ">> SKIP running end-to-end tests on $(GOHOSTOS)"

.PHONY: checkmetrics
checkmetrics: $(PROMTOOL)
	@echo ">> checking metrics for correctness"
	./checkmetrics.sh $(PROMTOOL) $(e2e-out)

.PHONY: checkrules
checkrules: $(PROMTOOL)
	@echo ">> checking rules for correctness"
	find . -name "*rules*.yml" | xargs -I {} $(PROMTOOL) check rules {}

# .PHONY: test-docker
# test-docker:
# 	@echo ">> testing docker image"
# 	./test_image.sh "$(DOCKER_REPO)/$(DOCKER_IMAGE_NAME)-linux-amd64:$(DOCKER_IMAGE_TAG)" 9100

.PHONY: promtool
promtool: $(PROMTOOL)

$(PROMTOOL):
	mkdir -p $(FIRST_GOPATH)/bin
	curl -fsS -L $(PROMTOOL_URL) | tar -xvzf - -C $(FIRST_GOPATH)/bin --strip 1 "prometheus-$(PROMTOOL_VERSION).$(GO_BUILD_PLATFORM)/promtool" 
