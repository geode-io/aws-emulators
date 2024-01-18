# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ Build

.PHONY: clean
clean: ## Delete all built binaries.
	rm -rf ./bin ./vendor ./out

.PHONY: build
build: build-emulators ## Build all binaries.

EMULATORS = \
	kinesis-subscription-emulator \
	apig-websocket-emulator

EMU = $(word 1, $(EMULATORS))

MOD = $(word 1, $(MODULES))

.PHONY: build-emulator
build-emulator: ## Build a single emulator binary. Usage: make build-emulator EMU=kinesis.
	env GOARCH=amd64 GOOS=linux go build -o bin/emulator/$(EMU) $(EMU)/run.go;

.PHONY: build-emulators
build-emulators: ## Build all emulator binaries.
	for emu in $(EMULATORS); do \
		env GOARCH=amd64 GOOS=linux go build -o bin/emulator/$$emu $$emu/run.go; \
	done

.PHONY: lint
lint: ## Lints the project, logging any warnings or errors without modifying any files.
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Reformat all code with the go fmt command.
	go fmt ./...

.PHONY: vet
vet: ## Run vet on all code with the go vet command.
	go vet ./...

##@ Tests

.PHONY: test
test: ## Unit test all modules.
	go test -v -race ./...

.PHONY: test-short
test-short: ## Unit test all modules in short-mode.
	go test -v -race -short ./...

##@ Run

.PHONY: offline-up
offline-up: ## Start the offline environment.
	docker-compose up --detach --force-recreate --build --remove-orphans

.PHONY: offline-down
offline-down: ## Stop the offline environment.
	docker-compose down --remove-orphans

.PHONY: offline-logs-v
offline-logs-v: ## View the logs of the offline environment for all containers.
	docker-compose logs --follow

.PHONY: offline-a
offline-a: ## Start the offline environment without detaching.
	docker-compose up --force-recreate --build --remove-orphans

.PHONY: offline-clean
offline-clean: ## Delete all offline environment data.
	docker-compose down --remove-orphans --volumes --rmi local

SH=/bin/bash
SVC = $(word 1, $(SERVICES))

.PHONY: offline-connect
offline-connect: ## Run and connect to the specified service. Usage make offline-connect SVC=bus. Optionally specify SH=/bin/sh.
	docker-compose exec $(SVC) $(SH)

##@ Misc.

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php
.PHONY: help
help: ## Display usage help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)