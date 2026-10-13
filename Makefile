default: generate build

BIN := ./build/terraform-provider-technitium
MODULE := github.com/ilijamt/terraform-provider-technitium

GIT_VERSION ?= $(patsubst v%,%,$(shell git describe --tags --always --dirty 2>/dev/null || echo dev))
GIT_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo SNAPSHOT)
STAMP := -X $(MODULE)/version.Version=$(GIT_VERSION) -X $(MODULE)/version.Commit=$(GIT_COMMIT)

# Renders the docs from the resource schemas and examples/.
.PHONY: generate
generate:
	go tool tfplugindocs generate --provider-name technitium --provider-dir .

.PHONY: build
build:
	@mkdir -p build
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w $(STAMP)" -o $(BIN) .

.PHONY: test
test:
	go test ./... -count=1 -cover

# Runs the Terraform tests against a technitium/dns-server container matching
# technitium.Version (needs Docker), or against TECHNITIUM_HOST when set.
.PHONY: testacc
testacc:
	@mkdir -p build
	TF_ACC=1 go test ./... -count=1 -timeout 15m -coverpkg=./internal/... -coverprofile=build/coverage.out
	go tool cover -func=build/coverage.out | tail -1

# Removes acceptance test containers left behind by a killed or timed-out run.
.PHONY: testacc-clean
testacc-clean:
	@ids=$$(docker ps --all --quiet --filter label=terraform-provider-technitium.testacc); [ -z "$$ids" ] || docker rm --force $$ids

.PHONY: fmt
fmt:
	gofmt -w .
	terraform fmt -recursive examples

.PHONY: lint
lint:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }
	terraform fmt -check -recursive examples
	go vet ./...
	golangci-lint run ./...

.PHONY: check
check: lint test

# Writes a .terraformrc with dev_overrides pointing at the local build.
.PHONY: terraformrc
terraformrc:
	@printf 'provider_installation {\n  dev_overrides {\n    "ilijamt/technitium" = "%s/build/"\n  }\n\n  direct {}\n}\n' "$(shell pwd)" > .terraformrc
	@echo "wrote $(shell pwd)/.terraformrc"
