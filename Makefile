name=planton
name_local=planton
pkg=github.com/plantonhq/planton
build_dir=build
version?=$(shell python3 tools/ci/release/next_version.py patch 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X ${pkg}/internal/cli/version.Version=${version}"

# bump: major, minor, or patch (default)
bump ?= patch

# Detect if version was explicitly provided on command line
ifeq ($(origin version),command line)
VERSION_EXPLICIT := true
else
VERSION_EXPLICIT := false
endif

BAZEL?=./bazelw

# If PLANTON_BUILDBUDDY_API_KEY is set, enable the :bb config and inject only the header.
ifneq ($(strip $(PLANTON_BUILDBUDDY_API_KEY)),)
BAZEL_REMOTE_FLAGS=--config=bb --remote_header=x-buildbuddy-api-key=$$PLANTON_BUILDBUDDY_API_KEY
else
BAZEL_REMOTE_FLAGS=
endif

build_cmd=go build -v ${LDFLAGS}

PARALLEL?=$(shell getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu)

clean-bazel:
	rm -rf .bazelbsp bazel-bin bazel-out bazel-testlogs bazel-planton

reset-ide: clean-bazel
	rm -rf .idea

.PHONY: deps
deps:
	go mod download
	go mod tidy

.PHONY: build_darwin
build_darwin:
	GOOS=darwin ${build_cmd} -o ${build_dir}/${name}-darwin .

.PHONY: buf-generate
buf-generate: protos

# --------------------------------------------------------------------------- #
#  Pinned proto toolchain (.tools/) -- local plugin execution
#
#  buf orchestrates codegen (buf.gen.yaml, managed mode), but every generator
#  binary runs locally from the git-ignored .tools/ directory, provisioned on
#  demand by the stamp targets below. Remote BSR plugins are deliberately not
#  used: buf.build's code-generation service enforces a server-side ~120s
#  execution limit that this proto surface exceeds (the hosted Java generator
#  fails with deadline_exceeded), and remote execution ties stub generation to
#  a third-party service being reachable.
#
#  Every pin is coupled to a runtime dependency -- bump both together:
#    PROTOC_VERSION                <-> com.google.protobuf:protobuf-java (MODULE.bazel)
#    PROTOC_GEN_GRPC_JAVA_VERSION  <-> io.grpc:*                         (MODULE.bazel)
#    PROTOC_GEN_GO_VERSION         <-> google.golang.org/protobuf        (go.mod)
#    PROTOC_GEN_GO_GRPC_VERSION    <-> google.golang.org/grpc            (go.mod)
#    PROTOC_GEN_ES_VERSION         <-> @bufbuild/protobuf (platform stubs/ts)
#
#  CROSS-REPO LOCKSTEP: protoc, grpc-java, and protoc-gen-es are ONE pin set
#  shared with the platform repo (planton-platform/product/apis/Makefile).
#  The stub-sdk release artifacts (make build-stub-sdks) record these pins in
#  their manifest, and the platform's fetch step REFUSES artifacts whose pins
#  disagree with its own -- so bumping here without bumping there (or vice
#  versa) fails the next platform upgrade loudly. Bump both repos together.
# --------------------------------------------------------------------------- #
PROTO_TOOLS_DIR := .tools

PROTOC_VERSION               := 34.0
PROTOC_GEN_GRPC_JAVA_VERSION := 1.79.0
PROTOC_GEN_GO_VERSION        := v1.36.6
PROTOC_GEN_GO_GRPC_VERSION   := v1.5.1
PROTOC_GEN_ES_VERSION        := 2.11.0

# protoc and grpc-java release artifacts share this platform naming scheme:
# osx-aarch_64 / osx-x86_64 / linux-aarch_64 / linux-x86_64
PROTO_TOOLS_OS   := $(if $(filter Darwin,$(shell uname -s)),osx,linux)
PROTO_TOOLS_ARCH := $(if $(filter arm64 aarch64,$(shell uname -m)),aarch_64,x86_64)

# Each tool is guarded by a version-stamped marker file: bumping a pin above
# reinstalls that tool on the next build; up-to-date tools cost nothing.
PROTOC_STAMP             := $(PROTO_TOOLS_DIR)/.stamp.protoc.$(PROTOC_VERSION)
GRPC_JAVA_STAMP          := $(PROTO_TOOLS_DIR)/.stamp.protoc-gen-grpc-java.$(PROTOC_GEN_GRPC_JAVA_VERSION)
PROTOC_GEN_GO_STAMP      := $(PROTO_TOOLS_DIR)/.stamp.protoc-gen-go.$(PROTOC_GEN_GO_VERSION)
PROTOC_GEN_GO_GRPC_STAMP := $(PROTO_TOOLS_DIR)/.stamp.protoc-gen-go-grpc.$(PROTOC_GEN_GO_GRPC_VERSION)
PROTOC_GEN_ES_STAMP      := $(PROTO_TOOLS_DIR)/.stamp.protoc-gen-es.$(PROTOC_GEN_ES_VERSION)

$(PROTOC_STAMP):
	@echo "installing protoc $(PROTOC_VERSION) into $(PROTO_TOOLS_DIR)/protoc/"
	@rm -rf $(PROTO_TOOLS_DIR)/protoc $(PROTO_TOOLS_DIR)/.stamp.protoc.*
	@mkdir -p $(PROTO_TOOLS_DIR)/protoc
	@curl -fsSL -o $(PROTO_TOOLS_DIR)/protoc.zip \
		https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/protoc-$(PROTOC_VERSION)-$(PROTO_TOOLS_OS)-$(PROTO_TOOLS_ARCH).zip
	@unzip -q $(PROTO_TOOLS_DIR)/protoc.zip -d $(PROTO_TOOLS_DIR)/protoc
	@rm -f $(PROTO_TOOLS_DIR)/protoc.zip
	@touch $@

$(GRPC_JAVA_STAMP):
	@echo "installing protoc-gen-grpc-java $(PROTOC_GEN_GRPC_JAVA_VERSION) into $(PROTO_TOOLS_DIR)/"
	@rm -f $(PROTO_TOOLS_DIR)/protoc-gen-grpc-java $(PROTO_TOOLS_DIR)/.stamp.protoc-gen-grpc-java.*
	@mkdir -p $(PROTO_TOOLS_DIR)
	@curl -fsSL -o $(PROTO_TOOLS_DIR)/protoc-gen-grpc-java \
		https://repo1.maven.org/maven2/io/grpc/protoc-gen-grpc-java/$(PROTOC_GEN_GRPC_JAVA_VERSION)/protoc-gen-grpc-java-$(PROTOC_GEN_GRPC_JAVA_VERSION)-$(PROTO_TOOLS_OS)-$(PROTO_TOOLS_ARCH).exe
	@chmod +x $(PROTO_TOOLS_DIR)/protoc-gen-grpc-java
	@touch $@

$(PROTOC_GEN_GO_STAMP):
	@echo "installing protoc-gen-go $(PROTOC_GEN_GO_VERSION) into $(PROTO_TOOLS_DIR)/"
	@rm -f $(PROTO_TOOLS_DIR)/.stamp.protoc-gen-go.*
	@mkdir -p $(PROTO_TOOLS_DIR)
	@GOBIN=$(abspath $(PROTO_TOOLS_DIR)) go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	@touch $@

$(PROTOC_GEN_GO_GRPC_STAMP):
	@echo "installing protoc-gen-go-grpc $(PROTOC_GEN_GO_GRPC_VERSION) into $(PROTO_TOOLS_DIR)/"
	@rm -f $(PROTO_TOOLS_DIR)/.stamp.protoc-gen-go-grpc.*
	@mkdir -p $(PROTO_TOOLS_DIR)
	@GOBIN=$(abspath $(PROTO_TOOLS_DIR)) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	@touch $@

$(PROTOC_GEN_ES_STAMP):
	@echo "installing @bufbuild/protoc-gen-es $(PROTOC_GEN_ES_VERSION) into $(PROTO_TOOLS_DIR)/node_modules/"
	@rm -f $(PROTO_TOOLS_DIR)/.stamp.protoc-gen-es.*
	@mkdir -p $(PROTO_TOOLS_DIR)
	@npm install --prefix $(PROTO_TOOLS_DIR) --no-audit --no-fund --loglevel=error @bufbuild/protoc-gen-es@$(PROTOC_GEN_ES_VERSION) >/dev/null
	@touch $@

.PHONY: proto-tools
proto-tools: $(PROTOC_STAMP) $(GRPC_JAVA_STAMP) $(PROTOC_GEN_GO_STAMP) $(PROTOC_GEN_GO_GRPC_STAMP)

.PHONY: buf-fmt
buf-fmt:
	buf format -w --disable-symlinks

.PHONY: protos
protos: buf-lint buf-fmt proto-tools
	rm -rf generated/stubs
	mkdir -p generated/stubs
	# --disable-symlinks: after any bazel run, the bazel-* convenience
	# symlinks point at an execroot carrying a full copy of the proto
	# tree; buf follows symlinks by default and fails on the duplicated
	# symbols. --timeout 20m: with LOCAL plugins the full tree outruns
	# buf's 2-minute default.
	buf generate --disable-symlinks --timeout 20m
	cp -R generated/stubs/go/github.com/plantonhq/planton/. .
	rm -rf generated/stubs/go
	# Generate a BUILD.bazel for the Java stubs so Bazel can compile them.
	# This serves as a build-time gate: if any proto generates invalid Java
	# (e.g., reserved keyword conflicts), the build fails here instead of in
	# downstream consumers.
	@printf '# gazelle:ignore\njava_library(\n    name = "java",\n    srcs = glob(["**/*.java"]),\n    visibility = ["//visibility:public"],\n    deps = [\n        "@maven//:build_buf_protovalidate",\n        "@maven//:com_google_guava_guava",\n        "@maven//:com_google_protobuf_protobuf_java",\n        "@maven//:io_grpc_grpc_api",\n        "@maven//:io_grpc_grpc_protobuf",\n        "@maven//:io_grpc_grpc_stub",\n        "@maven//:javax_annotation_javax_annotation_api",\n    ],\n)\n' > generated/stubs/java/BUILD.bazel
	@echo "Verifying generated Java stubs compile..."
	${BAZEL} build ${BAZEL_REMOTE_FLAGS} //generated/stubs/java:java
	# Rule-engine conformance gate: every CEL validation rule must COMPILE on
	# protovalidate-java (the platform control plane's engine, and the
	# strictest of the consumer engines). Walks every message descriptor --
	# rules compile lazily per validated type, so anything narrower silently
	# skips nested types (a rule the Java engine could not evaluate once
	# shipped in a release and emptied every fresh local instance's chart
	# catalog). See hack/javagate/ProtovalidateConformanceGate.java.
	@echo "Verifying every CEL rule compiles on protovalidate-java..."
	${BAZEL} test ${BAZEL_REMOTE_FLAGS} //hack/javagate:protovalidate_conformance_gate
	${BAZEL} run //:gazelle

# --------------------------------------------------------------------------- #
#  Prebuilt stub SDK artifacts (release cargo for the platform repo)
#
#  The platform consumes this repo's Java and TypeScript stubs as prebuilt
#  release artifacts (downloads.planton.dev/releases/{tag}/stubs/) instead of
#  cloning this repo and regenerating ~26k files on every pin upgrade. The
#  trees must be byte-identical to what the platform's retired local
#  generation produced, which is why the SDK templates mirror the platform's
#  managed-mode config and the toolchain pins are cross-repo lockstepped (see
#  the pin section above). The manifest records the pins; the platform's
#  fetch refuses artifacts whose pins disagree with its own.
#
#  The file-count floors are the release-packaging guard's "never ship a
#  silently empty zip" discipline applied to generation output: a
#  misconfigured template or input fails HERE, not in a consumer's build.
# --------------------------------------------------------------------------- #
STUB_SDKS_DIR        := generated/stub-sdks
STUB_SDKS_JAVA_FLOOR := 10000
STUB_SDKS_TS_FLOOR   := 3000

.PHONY: build-stub-sdks
build-stub-sdks: $(PROTOC_STAMP) $(GRPC_JAVA_STAMP) $(PROTOC_GEN_ES_STAMP)
	@test -n "$(tag)" || { echo "usage: make build-stub-sdks tag=vX.Y.Z"; exit 1; }
	rm -rf $(STUB_SDKS_DIR)
	mkdir -p $(STUB_SDKS_DIR)/java $(STUB_SDKS_DIR)/ts
	buf generate --disable-symlinks --timeout 20m --template buf.gen.sdk.java.yaml
	buf generate --disable-symlinks --timeout 20m --template buf.gen.sdk.ts.yaml
	@set -e; \
	java_count=$$(find $(STUB_SDKS_DIR)/java -type f -name '*.java' | wc -l | tr -d ' '); \
	ts_count=$$(find $(STUB_SDKS_DIR)/ts -type f | wc -l | tr -d ' '); \
	echo "generated: $$java_count java files, $$ts_count ts files"; \
	test "$$java_count" -ge $(STUB_SDKS_JAVA_FLOOR) || { echo "ERROR: java stub tree suspiciously thin ($$java_count < $(STUB_SDKS_JAVA_FLOOR)) -- template or input misconfigured; never ship a silently thin artifact"; exit 1; }; \
	test "$$ts_count" -ge $(STUB_SDKS_TS_FLOOR) || { echo "ERROR: ts stub tree suspiciously thin ($$ts_count < $(STUB_SDKS_TS_FLOOR)) -- template or input misconfigured; never ship a silently thin artifact"; exit 1; }; \
	(cd $(STUB_SDKS_DIR)/java && zip -qr ../stubs-java.zip .); \
	(cd $(STUB_SDKS_DIR)/ts && zip -qr ../stubs-ts.zip .); \
	(cd $(STUB_SDKS_DIR) && shasum -a 256 stubs-java.zip > stubs-java.zip.sha256 && shasum -a 256 stubs-ts.zip > stubs-ts.zip.sha256); \
	printf '{\n  "releaseTag": "%s",\n  "bufVersion": "%s",\n  "pins": {\n    "protoc": "%s",\n    "protocGenGrpcJava": "%s",\n    "protocGenEs": "%s"\n  },\n  "fileCounts": {\n    "java": %s,\n    "ts": %s\n  }\n}\n' \
		"$(tag)" "$$(buf --version)" "$(PROTOC_VERSION)" "$(PROTOC_GEN_GRPC_JAVA_VERSION)" "$(PROTOC_GEN_ES_VERSION)" "$$java_count" "$$ts_count" \
		> $(STUB_SDKS_DIR)/stub-sdks-manifest.json; \
	echo "stub SDK artifacts ready in $(STUB_SDKS_DIR)/"

.PHONY: build-optional-linter-plugin
build-optional-linter-plugin:
	@echo "Building buf lint plugin..."
	@cd buf/lint/optional-linter && $(MAKE) build

.PHONY: buf-lint
buf-lint: build-optional-linter-plugin
	# --disable-symlinks keeps local runs immune to the bazel-* convenience
	# symlinks (bazel run //:gazelle recreates them), whose tree duplication
	# otherwise fails the module compile with duplicate-symbol errors.
	buf lint --disable-symlinks

.PHONY: buf-breaking
# Compares the module's protos against the main branch, classified by the
# maturity channel each finding's version directory declares: alpha findings
# are advisory (alpha may break in place, visibly); beta/stable/shared
# findings fail. The rule set lives in buf.yaml (breaking:); the channel
# policy lives in tools/ci/proto/buf_breaking_channel_gate.sh.
buf-breaking:
	bash tools/ci/proto/buf_breaking_channel_gate.sh

.PHONY: bazel-mod-tidy
# go mod tidy first: `bazel mod tidy` only exposes the modules go.mod marks
# DIRECT, so a new import of a module still tagged `// indirect` passes
# every earlier step and fails analysis at //:planton with "no repository
# visible as @org_golang_x_...". Tidying go.mod here makes that impossible.
bazel-mod-tidy:
	go mod tidy
	${BAZEL} mod tidy

.PHONY: gazelle
gazelle: bazel-gazelle

.PHONY: bazel-gazelle
bazel-gazelle:
	${BAZEL} run ${BAZEL_REMOTE_FLAGS} //:gazelle

.PHONY: clean-gazelle
clean-gazelle:
	@echo "Cleaning all BUILD.bazel files (excluding root)..."
	@find . -mindepth 2 -name "BUILD.bazel" -type f -delete
	@echo "✅ All BUILD.bazel files removed (root preserved)."

.PHONY: reset-gazelle
reset-gazelle: clean-gazelle gazelle
	@echo "✅ Gazelle reset complete. BUILD.bazel files regenerated."

.PHONY: bazel-build-cli
bazel-build-cli:
	${BAZEL} build ${BAZEL_REMOTE_FLAGS} //:planton

.PHONY: bazel-test
bazel-test:
	${BAZEL} test ${BAZEL_REMOTE_FLAGS} --test_output=errors //...

# Generates kind_map_gen.go containing ToMessageMap.
# The "-tags codegen" flag is REQUIRED to avoid chicken-and-egg compilation errors.
# See pkg/crkreflect/new_instance.go and pkg/crkreflect/codegen/main.go for details.
# Regenerates pkg/conversion/embedded/specs -- the byte-for-byte mirror of
# the co-located conversion specs (catalog/<provider>/<kind>/conversions/*.yaml) that
# ships inside standalone binaries. The mirror exists because Go embeds
# cannot cross Bazel package boundaries; the drift test in pkg/conversion
# fails whenever mirror and authored specs disagree.
.PHONY: generate-conversion-registry
generate-conversion-registry:
	go run -tags codegen ./pkg/conversion/codegen

# Builds the catalog bundle -- the catalog as DATA (schemas + validation
# rules + kind registry in one buf-built descriptor set, plus conversion
# specs and presets), checksummed and self-describing. See
# pkg/catalogbundle/bundle.go for the format contract.
.PHONY: build-catalog-bundle
build-catalog-bundle:
	mkdir -p build
	buf build --disable-symlinks -o build/catalog-descriptors.binpb
	go run ./pkg/catalogbundle/cli build \
		--descriptors build/catalog-descriptors.binpb \
		--catalog-dir catalog \
		--out build/catalog-bundle.zip $(if $(tag),--tag $(tag))

# Verifies the built bundle: checksum self-verification plus the registry
# conformance gate (the bundle must serve EXACTLY what the compiled-in
# registry serves, for every kind). A bundle that fails must never ship.
.PHONY: verify-catalog-bundle
verify-catalog-bundle:
	go run ./pkg/catalogbundle/cli verify --bundle build/catalog-bundle.zip

.PHONY: build-catalog-schema-plugin
build-catalog-schema-plugin:
	go build -o $(shell go env GOPATH)/bin/protoc-gen-catalog-schema ./pkg/catalogschema/protoc-gen-catalog-schema

# Builds the catalog-schemas artifact -- one JSON schema document per catalog
# .proto (the pkg/catalogschema published contract, authored source included),
# for consoles and tools that render API contracts without compiling protos.
# The tree is regenerated wholesale into build/ (never committed) and zipped
# deterministically. See pkg/catalogschema/types.go for the contract.
.PHONY: build-catalog-schemas
build-catalog-schemas: build-catalog-schema-plugin
	rm -rf build/catalog-schemas
	buf generate --template buf.gen.catalog-schema.yaml
	go run ./pkg/catalogschema/cli package --dir build/catalog-schemas --out build/catalog-schemas.zip

# Verifies the built schema artifact: every user-facing registry kind's four
# contract documents are present at its declared version, nothing from the
# _test provider ships, and every document reads back under the published
# contract. An artifact that fails must never ship.
.PHONY: verify-catalog-schemas
verify-catalog-schemas:
	go run ./pkg/catalogschema/cli verify --zip build/catalog-schemas.zip

.PHONY: generate-cloud-resource-kind-map
generate-cloud-resource-kind-map:
	rm -f pkg/crkreflect/kind_map_gen.go
	go run -tags codegen ./pkg/crkreflect/codegen

.PHONY: generate-kubernetes-types
generate-kubernetes-types:
	$(MAKE) -C pkg/kubernetes/kubernetestypes build

# Regenerates pkg/protodocs/index.json.gz -- the proto-source documentation
# `planton explain` serves offline. Generated protobuf code strips comments,
# so the prose is distilled from a descriptor image built with source info.
# Deterministic: unchanged protos regenerate a byte-identical artifact.
# If `buf build` fails with "symbol already defined at bazel-<repo>/...",
# stale Bazel output symlinks at the repo root are leaking a duplicate
# proto tree into the module walk -- delete the bazel-* symlinks and rerun.
.PHONY: generate-proto-docs
generate-proto-docs:
	mkdir -p build
	# --disable-symlinks: after any bazel run, the bazel-* convenience
	# symlinks point at an execroot carrying a full copy of the proto
	# tree; buf follows symlinks by default and fails on the duplicated
	# symbols. The module has no legitimate protos behind symlinks.
	buf build --disable-symlinks -o build/proto-docs-image.binpb
	go run ./pkg/protodocs/distiller \
		--image build/proto-docs-image.binpb \
		--out pkg/protodocs/index.json.gz
	rm -f build/proto-docs-image.binpb

# Regenerates pkg/providerparity/schemas/ -- the distilled Terraform provider
# schemas the parity accounting measures the catalog against. One artifact
# per provider at the exact release its pin resolves to; bumping a pin means
# editing the constraint below and re-running (the artifact is replaced,
# never accumulated). Requires `tofu` on PATH and network access to the
# provider registry. Deterministic: an unchanged pin regenerates a
# byte-identical artifact.
.PHONY: generate-provider-schemas
generate-provider-schemas:
	go run ./pkg/providerparity/distiller \
		--out-dir pkg/providerparity/schemas \
		--provider 'google=hashicorp/google@~> 7.43' \
		--provider 'google-beta=hashicorp/google-beta@~> 7.43' \
		--provider 'azurerm=hashicorp/azurerm@5.0.0' \
		--provider 'aws=hashicorp/aws@~> 6.58' \
		--provider 'cloudflare=cloudflare/cloudflare@5.23.0' \
		--provider 'digitalocean=digitalocean/digitalocean@~> 2.99'

# Regenerate every committed public parity page (catalog/<provider>/terraform-parity.md)
# from the accounting. Each page embeds its own generation parameters, so this
# target needs no per-provider configuration; a provider enrolls its first page
# with `planton provider-parity --provider <p> --ga-schema <s> --write-report`.
.PHONY: generate-provider-parity-report
generate-provider-parity-report:
	PLANTON_REGEN_PROVIDERPARITY_REPORT=1 go test -count=1 ./pkg/providerparity/ -run TestPublicReportDrift

# Regenerates the committed catalog reference: per-kind reference.md files
# (co-located with each kind's protos) plus the catalog-level indexes,
# foreign-key graph, and commons page, all from the compiled-in descriptors
# via the explain engine. Always whole-catalog: cross-kind sections depend
# on every other kind's schema, so there is deliberately no way to scope a
# run. Deterministic: unchanged schemas regenerate byte-identical files
# (enforced by the drift test in pkg/explain/refgen).
#
# Depends on generate-proto-docs: the pages' comment PROSE renders from the
# embedded pkg/protodocs index (the protobuf runtime strips comments), so
# regenerating references against a stale index silently reprints old field
# documentation even though the schema tables update. Chaining the two is
# cheap and idempotent — both regens are byte-deterministic.
.PHONY: generate-reference
generate-reference: generate-proto-docs
	go run ./pkg/explain/refgen

# Regenerates the committed per-component cost estimates
# (catalog/_pricing/estimates/): derived components replay every preset
# through their cost derivation (catalog/_pricing/derivations/), modeled
# components join their estimate model (catalog/_pricing/models/) with the
# provider's price book (catalog/_pricing/pricebook/), and cluster-capacity
# components replay their presets through their capacity derivation
# (catalog/_pricing/capacity/) into footprint estimates. Always whole-tree:
# the dead-price sweep
# needs every model's references. Offline and deterministic: unchanged
# inputs regenerate byte-identical files (enforced by the drift test in
# pkg/finops/estimategen).
.PHONY: generate-cost-estimates
generate-cost-estimates:
	go run ./pkg/finops/estimategen

# Refreshes the price-book entries that carry a machine selector from the
# providers' public price APIs -- the AWS Price List bulk API, the Azure
# Retail Prices API, and the GCP Cloud Billing Catalog API (which needs
# GCP_BILLING_API_KEY or gcloud application-default credentials) --
# rewriting each refreshed entry's price, refetchable source URL, and
# retrieval date. Requires network access; CI never fetches -- it validates
# the committed snapshot. After a refresh, run generate-cost-estimates to
# roll the new prices into the estimates.
.PHONY: generate-price-book
generate-price-book:
	go run ./pkg/finops/pricebook/fetcher

# Refreshes the committed action-inventory snapshots
# (pkg/iac/actioninventory/{aws,azure,gcp,cloudflare,digitalocean}.yaml)
# from each provider's own published inventory -- AWS's machine-readable
# service reference (including each action's resource-scopability, which
# the scopability gate holds statements to), ARM's provider-operations
# metadata (the Azure arm needs a signed-in Azure CLI for its bearer
# token), GCP IAM's testable-permissions inventory (the GCP arm needs
# gcloud application-default credentials, an active gcloud project, and
# the PLANTON_GCP_ORG / PLANTON_GCP_BILLING_ACCOUNT anchors -- any org and
# billing account the credential can query; they only type the queries,
# whose results union), Cloudflare's permission-group inventory (the
# Cloudflare arm needs CLOUDFLARE_API_TOKEN -- an account-owned token
# that can read the account's token permission groups; the catalog is
# global, so any account works), and DigitalOcean's published token-scope
# reference (no credential -- the docs site serves it as machine-readable
# markdown; DigitalOcean exposes no scope-inventory API) -- scoped to the
# services, groups, and scopes the committed runner permissions manifests
# reference. Requires network access; CI never fetches -- it validates
# every manifest action against the committed snapshots (an invented or
# misspelled action name cannot ship).
.PHONY: generate-action-inventory
generate-action-inventory:
	go run ./pkg/iac/actioninventory/fetcher

.PHONY: build-go
build-go: fmt deps vet
	GOOS=darwin GOARCH=amd64 ${build_cmd} -o ${build_dir}/${name}-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 ${build_cmd} -o ${build_dir}/${name}-darwin-arm64 .
	GOOS=linux GOARCH=amd64 ${build_cmd} -o ${build_dir}/${name}-linux .
	openssl dgst -sha256 ${build_dir}/${name}-darwin-arm64
	openssl dgst -sha256 ${build_dir}/${name}-linux

.PHONY: build-cli
build-cli: build-go

.PHONY: build
build: protos generate-cloud-resource-kind-map generate-proto-docs bazel-mod-tidy bazel-gazelle bazel-build-cli build-cli e2e-matrix

${build_dir}/${name}: build-go

.PHONY: test
test:
	go test -race -v -count=1 -p $(PARALLEL) ./...

.PHONY: run
run: build
	${build_dir}/${name}

.PHONY: vet
vet:
	go vet ./cmd/...
	go vet ./internal/...
	go vet ./pkg/...
	go vet ./cicd/...

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: clean
clean:
	rm -rf ${build_dir}

.PHONY: checksum_darwin
checksum_darwin:
	@openssl dgst -sha256 ${build_dir}/${name}-darwin

.PHONY: checksum_linux
checksum_linux:
	openssl dgst -sha256 ${build_dir}/${name}-linux

.PHONY: checksum
checksum: checksum_darwin checksum_linux

.PHONY: local
local: build_darwin
	rm -f ${HOME}/.local/bin/${name_local}
	cp ./${build_dir}/${name}-darwin ${HOME}/.local/bin/${name_local}
	chmod +x ${HOME}/.local/bin/${name_local}

.PHONY: show-todo
show-todo:
	grep -r "TODO:" cmd internal

.PHONY: package-content
package-content:  ## Package all content zips (presets, iac-source, catalog-pages, proto-source)
	bash tools/ci/release/package_content.sh ${version}

.PHONY: release-buf
release-buf:
	buf push && buf push --label ${version}

.PHONY: next-version
next-version:  ## show what the next version would be
	@python3 tools/ci/release/next_version.py $(bump)

.PHONY: release
release:  ## auto-bump version, tag & push (bump=major|minor|patch, default: patch). Override with version=vX.Y.Z
	@if [ "$(VERSION_EXPLICIT)" = "true" ]; then \
		rel_version="$(version)"; \
		echo "Releasing: $$rel_version (explicit version)"; \
	else \
		rel_version=$$(python3 tools/ci/release/next_version.py $(bump)); \
		echo "Releasing: $$rel_version ($(bump) bump)"; \
	fi; \
	git tag -a $$rel_version -m "$$rel_version"; \
	git push origin $$rel_version

.PHONY: test-and-release
test-and-release: test release

# ── Website (site/) ───────────────────────────────────────────────────────────
# The planton.ai website lives in site/ with its own Makefile; these targets
# just delegate. run-site starts the Next.js dev server; preview-site builds
# the full static export (what GitHub Pages serves) and serves it locally.
.PHONY: run-site
run-site:
	$(MAKE) -C site run

.PHONY: preview-site
preview-site:
	$(MAKE) -C site preview-site

# ── E2E Tests ─────────────────────────────────────────────────────────────────
# Every provider test package sets up its harness in TestMain BEFORE Go applies
# the -run filter, so sweeping ./e2e/... pays every provider's harness setup --
# and inherits its credential failures -- even when zero tests in that package
# match. Each run below is therefore scoped to the package that owns the tests;
# Kubernetes tests live in the root ./e2e/ package.
.PHONY: e2e-test-kubernetes
e2e-test-kubernetes:  ## Run all Kubernetes E2E tests -- Tier 1 + Tier 2 + Tier 3 + Tier 4 (requires kind, pulumi, kubectl, Docker)
	go test -tags=e2e -timeout=360m -v -count=1 ./e2e/

.PHONY: e2e-test-kubernetes-tier1
e2e-test-kubernetes-tier1:  ## Run Kubernetes Tier 1 (native K8s) E2E tests only
	go test -tags=e2e -timeout=60m -v -count=1 -run "Test(KubernetesNamespace|KubernetesConfigMap|KubernetesServiceAccount|KubernetesRbac|KubernetesDeployment|KubernetesStatefulSet|KubernetesSecret|KubernetesService|KubernetesIngress|KubernetesNetworkPolicy|KubernetesCronJob|KubernetesJob|KubernetesDaemonSet|KubernetesManifest|KubernetesHelmRelease|KubernetesPersistentVolumeClaim|KubernetesStorageClass|KubernetesResourceQuota|KubernetesPriorityClass|KubernetesPodDisruptionBudget|KubernetesHorizontalPodAutoscaler|KubernetesCertManager|KubernetesClusterIssuer|KubernetesIssuer|KubernetesCertificate|KubernetesExternalDns|KubernetesExternalSecretsOperator|KubernetesClusterSecretStore|KubernetesSecretStore|KubernetesExternalSecret|KubernetesIngressNginx|KubernetesMetricsServer|KubernetesCilium|KubernetesKeda|KubernetesBackendTlsPolicy|KubernetesGatewayApiCrds|KubernetesGatewayClass|KubernetesGateway|KubernetesListenerSet|KubernetesHttpRoute|KubernetesGrpcRoute|KubernetesTcpRoute|KubernetesUdpRoute|KubernetesTlsRoute|KubernetesReferenceGrant|KubernetesIstio|KubernetesIstioBaseCrds|KubernetesPeerAuthentication|KubernetesRequestAuthentication|KubernetesAuthorizationPolicy|KubernetesServiceEntry|KubernetesDestinationRule|KubernetesEnvoyFilter|KubernetesTelemetry|KubernetesKarpenter|KubernetesKarpenterNodePool|KubernetesKarpenterEc2NodeClass|KubernetesClusterAutoscaler|KubernetesVelero|KubernetesCloudNativePgOperator|KubernetesCnpgBarmanCloudPlugin|KubernetesPostgres)_" ./e2e/

.PHONY: e2e-test-kubernetes-tier2
e2e-test-kubernetes-tier2:  ## Run Kubernetes Tier 2 (Helm-based) E2E tests only
	go test -tags=e2e -timeout=120m -v -count=1 -run "Test(KubernetesValkey|KubernetesGrafana|KubernetesKubePrometheusStack|KubernetesLoki|KubernetesTempo|KubernetesOpenBao|KubernetesOpenFga|KubernetesHarbor|KubernetesArgoCD|KubernetesArgoWorkflows|KubernetesLocust|KubernetesNats|KubernetesNeo4j|KubernetesJenkins|KubernetesSolrOperator|KubernetesPerconaMongoOperator|KubernetesPerconaMysqlOperator|KubernetesTemporal|KubernetesSeaweedFs|KubernetesQdrant|KubernetesKyverno|KubernetesGatekeeper|KubernetesSparkOperator)_" ./e2e/

.PHONY: e2e-test-kubernetes-tier3
e2e-test-kubernetes-tier3:  ## Run Kubernetes Tier 3 (operator-dependent) E2E tests -- fixtures deployed automatically
	go test -tags=e2e -timeout=120m -v -count=1 -run "Test(KubernetesKafka|KubernetesKafkaTopic|KubernetesKafkaUser|KubernetesKafkaConnect|KubernetesKafkaConnector|KubernetesKafkaMirrorMaker2|KubernetesKarapace|KubernetesKafkaUi|KubernetesOpenSearch|KubernetesMongodb|KubernetesMysql|KubernetesSolr|KubernetesClickHouse|KubernetesRabbitMq|KubernetesSignoz|KubernetesTekton|KubernetesGhaRunnerScaleSet|KubernetesPlantonRunner|KubernetesPlantonPlatform|KubernetesKeycloak|KubernetesOtelCollector|KubernetesAirflow|KubernetesRayCluster|KubernetesFlinkDeployment|KubernetesJupyterHub|KubernetesMlflow|KubernetesTrino|KubernetesSuperset)_" ./e2e/

.PHONY: e2e-test-kubernetes-tier4
e2e-test-kubernetes-tier4:  ## Run Kubernetes Tier 4 (operators, addons, cluster infra) E2E tests
	go test -tags=e2e -timeout=150m -v -count=1 -run "Test(KubernetesStrimziKafkaOperator|KubernetesOpenSearchOperator|KubernetesAltinityOperator|KubernetesRabbitMqOperator|KubernetesGhaRunnerScaleSetController|KubernetesTektonOperator|KubernetesPlantonOperator|KubernetesKeycloakOperator|KubernetesOtelOperator|KubernetesKubeRayOperator|KubernetesFlinkOperator)_" ./e2e/

# ── Terraform-only E2E targets (requires kind, tofu/terraform, kubectl, Docker) ──

.PHONY: e2e-test-kubernetes-terraform-tier1
e2e-test-kubernetes-terraform-tier1:  ## Run Kubernetes Tier 1 Terraform E2E tests only
	go test -tags=e2e -timeout=60m -v -count=1 -run "Test(KubernetesNamespace|KubernetesConfigMap|KubernetesServiceAccount|KubernetesRbac|KubernetesDeployment|KubernetesStatefulSet|KubernetesSecret|KubernetesService|KubernetesIngress|KubernetesNetworkPolicy|KubernetesCronJob|KubernetesJob|KubernetesDaemonSet|KubernetesManifest|KubernetesHelmRelease|KubernetesPersistentVolumeClaim|KubernetesStorageClass|KubernetesResourceQuota|KubernetesPriorityClass|KubernetesPodDisruptionBudget|KubernetesHorizontalPodAutoscaler|KubernetesCertManager|KubernetesClusterIssuer|KubernetesIssuer|KubernetesCertificate|KubernetesExternalDns|KubernetesExternalSecretsOperator|KubernetesClusterSecretStore|KubernetesSecretStore|KubernetesExternalSecret|KubernetesIngressNginx|KubernetesMetricsServer|KubernetesCilium|KubernetesKeda|KubernetesBackendTlsPolicy|KubernetesGatewayApiCrds|KubernetesGatewayClass|KubernetesGateway|KubernetesListenerSet|KubernetesHttpRoute|KubernetesGrpcRoute|KubernetesTcpRoute|KubernetesUdpRoute|KubernetesTlsRoute|KubernetesReferenceGrant|KubernetesIstio|KubernetesIstioBaseCrds|KubernetesPeerAuthentication|KubernetesRequestAuthentication|KubernetesAuthorizationPolicy|KubernetesServiceEntry|KubernetesDestinationRule|KubernetesEnvoyFilter|KubernetesTelemetry|KubernetesKarpenter|KubernetesKarpenterNodePool|KubernetesKarpenterEc2NodeClass|KubernetesClusterAutoscaler|KubernetesVelero|KubernetesCloudNativePgOperator|KubernetesCnpgBarmanCloudPlugin|KubernetesPostgres)_Terraform" ./e2e/

.PHONY: e2e-test-kubernetes-terraform-tier2
e2e-test-kubernetes-terraform-tier2:  ## Run Kubernetes Tier 2 Terraform (Helm-based) E2E tests only
	go test -tags=e2e -timeout=120m -v -count=1 -run "Test(KubernetesValkey|KubernetesGrafana|KubernetesKubePrometheusStack|KubernetesLoki|KubernetesTempo|KubernetesOpenBao|KubernetesOpenFga|KubernetesHarbor|KubernetesArgoCD|KubernetesArgoWorkflows|KubernetesLocust|KubernetesNats|KubernetesNeo4j|KubernetesSolrOperator|KubernetesPerconaMongoOperator|KubernetesPerconaMysqlOperator|KubernetesTemporal|KubernetesSeaweedFs|KubernetesQdrant|KubernetesKyverno|KubernetesGatekeeper|KubernetesSparkOperator)_Terraform" ./e2e/

.PHONY: e2e-test-kubernetes-terraform-tier3
e2e-test-kubernetes-terraform-tier3:  ## Run Kubernetes Tier 3 Terraform (operator-dependent) E2E tests
	go test -tags=e2e -timeout=120m -v -count=1 -run "Test(KubernetesKafka|KubernetesKafkaTopic|KubernetesKafkaUser|KubernetesKafkaConnect|KubernetesKafkaConnector|KubernetesKafkaMirrorMaker2|KubernetesKarapace|KubernetesKafkaUi|KubernetesOpenSearch|KubernetesMongodb|KubernetesMysql|KubernetesSolr|KubernetesClickHouse|KubernetesRabbitMq|KubernetesSignoz|KubernetesTekton|KubernetesGhaRunnerScaleSet|KubernetesPlantonRunner|KubernetesPlantonPlatform|KubernetesKeycloak|KubernetesOtelCollector|KubernetesAirflow|KubernetesRayCluster|KubernetesFlinkDeployment|KubernetesJupyterHub|KubernetesMlflow|KubernetesTrino|KubernetesSuperset)_Terraform" ./e2e/

.PHONY: e2e-test-kubernetes-terraform-tier4
e2e-test-kubernetes-terraform-tier4:  ## Run Kubernetes Tier 4 Terraform (operators, addons) E2E tests
	go test -tags=e2e -timeout=150m -v -count=1 -run "Test(KubernetesStrimziKafkaOperator|KubernetesOpenSearchOperator|KubernetesAltinityOperator|KubernetesRabbitMqOperator|KubernetesGhaRunnerScaleSetController|KubernetesTektonOperator|KubernetesPlantonOperator|KubernetesKeycloakOperator|KubernetesOtelOperator|KubernetesKubeRayOperator|KubernetesFlinkOperator)_Terraform" ./e2e/

# ── Auth0 E2E targets ────────────────────────────────────────────────────────

.PHONY: e2e-test-auth0
e2e-test-auth0:  ## Run all Auth0 E2E tests (requires AUTH0_DOMAIN, AUTH0_CLIENT_ID, AUTH0_CLIENT_SECRET)
	go test -tags=e2e -timeout=20m -v -count=1 ./e2e/auth0/...

.PHONY: e2e-test-auth0-pulumi
e2e-test-auth0-pulumi:  ## Run Auth0 Pulumi E2E tests only
	go test -tags=e2e -timeout=20m -v -count=1 -run ".*_Pulumi" ./e2e/auth0/...

.PHONY: e2e-test-auth0-terraform
e2e-test-auth0-terraform:  ## Run Auth0 Terraform E2E tests only
	go test -tags=e2e -timeout=20m -v -count=1 -run ".*_Terraform" ./e2e/auth0/...

# ── Cloudflare E2E targets ───────────────────────────────────────────────────

.PHONY: e2e-test-cloudflare
e2e-test-cloudflare:  ## Run all Cloudflare E2E tests (requires CLOUDFLARE_API_TOKEN, PLANTON_E2E_CLOUDFLARE_ACCOUNT_ID)
	go test -tags=e2e -timeout=30m -v -count=1 ./e2e/cloudflare/...

.PHONY: e2e-test-cloudflare-pulumi
e2e-test-cloudflare-pulumi:  ## Run Cloudflare Pulumi E2E tests only
	go test -tags=e2e -timeout=30m -v -count=1 -run ".*_Pulumi" ./e2e/cloudflare/...

.PHONY: e2e-test-cloudflare-terraform
e2e-test-cloudflare-terraform:  ## Run Cloudflare Terraform E2E tests only
	go test -tags=e2e -timeout=30m -v -count=1 -run ".*_Terraform" ./e2e/cloudflare/...

# ── GCP E2E targets ──────────────────────────────────────────────────────────

.PHONY: e2e-test-gcp
e2e-test-gcp:  ## Run all GCP E2E tests (requires ADC; test project via E2E_GCP_PROJECT/GOOGLE_PROJECT)
	go test -tags=e2e -timeout=30m -v -count=1 ./e2e/gcp/...

.PHONY: e2e-test-gcp-pulumi
e2e-test-gcp-pulumi:  ## Run GCP Pulumi E2E tests only
	go test -tags=e2e -timeout=30m -v -count=1 -run ".*_Pulumi" ./e2e/gcp/...

.PHONY: e2e-test-gcp-terraform
e2e-test-gcp-terraform:  ## Run GCP Terraform E2E tests only
	go test -tags=e2e -timeout=30m -v -count=1 -run ".*_Terraform" ./e2e/gcp/...

# ── DigitalOcean E2E targets ─────────────────────────────────────────────────

.PHONY: e2e-test-digitalocean
e2e-test-digitalocean:  ## Run all DigitalOcean E2E tests (requires DIGITALOCEAN_TOKEN; bucket lanes also need SPACES_ACCESS_KEY_ID/SPACES_SECRET_ACCESS_KEY)
	go test -tags=e2e -timeout=60m -v -count=1 ./e2e/digitalocean/...

.PHONY: e2e-test-digitalocean-pulumi
e2e-test-digitalocean-pulumi:  ## Run DigitalOcean Pulumi E2E tests only
	go test -tags=e2e -timeout=60m -v -count=1 -run ".*_Pulumi" ./e2e/digitalocean/...

.PHONY: e2e-test-digitalocean-terraform
e2e-test-digitalocean-terraform:  ## Run DigitalOcean Terraform E2E tests only
	go test -tags=e2e -timeout=60m -v -count=1 -run ".*_Terraform" ./e2e/digitalocean/...

# ── Generic component E2E targets ────────────────────────────────────────────

# Resolve the component name's provider prefix to the test package that owns it
# (see the harness-setup note at the top of the E2E section for why runs must
# never sweep ./e2e/...). Unknown prefixes fall back to the full sweep.
# DigitalOcean is matched before Kubernetes: DigitalOceanKubernetesCluster
# contains "Kubernetes", so prefix order is load-bearing.
e2e_component_pkg = $(if $(findstring DigitalOcean,$(component)),./e2e/digitalocean/...,\
$(if $(findstring Kubernetes,$(component)),./e2e/,\
$(if $(findstring Aws,$(component)),./e2e/aws/...,\
$(if $(findstring Gcp,$(component)),./e2e/gcp/...,\
$(if $(findstring Azure,$(component)),./e2e/azure/...,\
$(if $(findstring Auth0,$(component)),./e2e/auth0/...,\
$(if $(findstring Cloudflare,$(component)),./e2e/cloudflare/...,./e2e/...)))))))

.PHONY: e2e-test-component
e2e-test-component:  ## Single component E2E test (usage: make e2e-test-component component=KubernetesNamespace)
	go test -tags=e2e -timeout=120m -v -count=1 -run "Test.*$(component)" $(strip $(e2e_component_pkg))

.PHONY: e2e-matrix
e2e-matrix:  ## Regenerate E2E GitHub Actions matrix JSON from profiles
	go run . e2e discover \
		--provider kubernetes --status green --output github-matrix \
		> .github/e2e-matrix-kubernetes.json

.PHONY: e2e-build
e2e-build:  ## Compile E2E tests without running them
	go build -tags=e2e ./e2e/...

.PHONY: e2e-vet
e2e-vet:  ## Run go vet on E2E packages
	go vet ./e2e/framework/...
	go vet -tags=e2e ./e2e/...

# ── Kubernetes operator (operator/) ───────────────────────────────────────────
# The operator is a standalone Go module with its own kubebuilder Makefile; the
# targets below are front doors that delegate into it. It releases on its own
# version line: a git tag `operator/vX.Y.Z` publishes the image
# ghcr.io/plantonhq/planton/operator:vX.Y.Z AND the planton-operator Helm chart
# as X.Y.Z with appVersion vX.Y.Z (release.operator.yaml, which hands the chart
# to release.helm.yaml once the image is verified). Chart and operator share
# one number; nothing in git changes because of a release.
OPERATOR_TAG_PREFIX := operator/

.PHONY: operator-test
operator-test:  ## Run the operator's unit and envtest suites
	$(MAKE) -C operator test

.PHONY: operator-e2e
operator-e2e:  ## Run the operator's Kind e2e lane (creates and deletes its own cluster)
	$(MAKE) -C operator test-e2e

.PHONY: operator-build-image
operator-build-image:  ## Build the operator image locally (IMG=<name:tag> to override)
	$(MAKE) -C operator docker-build $(if $(IMG),IMG=$(IMG),)

.PHONY: operator-manifests
operator-manifests:  ## Regenerate the operator's CRDs and RBAC into config/ and helm/planton-operator
	$(MAKE) -C operator manifests generate

.PHONY: release-operator
release-operator:  ## Release the operator image + chart: auto-bump (bump=major|minor|patch, default patch) or version=vX.Y.Z
	@if [ "$(VERSION_EXPLICIT)" = "true" ]; then \
		rel_version="$(version)"; \
		case "$$rel_version" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo "version must look like vX.Y.Z (got $$rel_version)"; exit 1;; esac; \
		echo "Releasing operator $$rel_version (explicit version)"; \
	else \
		rel_version=$$(python3 tools/ci/release/next_version.py $(bump) --prefix $(OPERATOR_TAG_PREFIX)); \
		echo "Releasing operator $$rel_version ($(bump) bump)"; \
	fi; \
	git tag -a "$(OPERATOR_TAG_PREFIX)$$rel_version" -m "operator $$rel_version"; \
	git push origin "$(OPERATOR_TAG_PREFIX)$$rel_version"

# ── Helm charts (helm/) ───────────────────────────────────────────────────────
# Every chart under helm/ releases from a git tag named by its directory and
# version: `helm/<chart>/vX.Y.Z` publishes oci://ghcr.io/plantonhq/charts/<chart>
# at X.Y.Z through release.helm.yaml, which stamps the version at package time
# (Chart.yaml in git carries a development placeholder). planton-operator is the
# exception: its chart releases with the operator (make release-operator).
.PHONY: release-helm
release-helm:  ## Release a Helm chart: chart=<name> (helm/<name>), auto-bump (bump=...) or version=vX.Y.Z
	@if [ -z "$(chart)" ]; then echo "usage: make release-helm chart=<name> [bump=patch|minor|major | version=vX.Y.Z]"; exit 1; fi
	@if [ ! -f "helm/$(chart)/Chart.yaml" ]; then echo "no chart at helm/$(chart)"; exit 1; fi
	@if [ "$(chart)" = "planton-operator" ]; then echo "planton-operator releases with the operator: make release-operator"; exit 1; fi
	@if [ "$(VERSION_EXPLICIT)" = "true" ]; then \
		rel_version="$(version)"; \
		case "$$rel_version" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo "version must look like vX.Y.Z (got $$rel_version)"; exit 1;; esac; \
		echo "Releasing chart $(chart) $$rel_version (explicit version)"; \
	else \
		rel_version=$$(python3 tools/ci/release/next_version.py $(bump) --prefix helm/$(chart)/); \
		echo "Releasing chart $(chart) $$rel_version ($(bump) bump)"; \
	fi; \
	git tag -a "helm/$(chart)/$$rel_version" -m "helm chart $(chart) $$rel_version"; \
	git push origin "helm/$(chart)/$$rel_version"
