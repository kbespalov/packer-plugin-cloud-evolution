NAME=cloud-evolution
BINARY=packer-plugin-${NAME}
PLUGIN_FQN=$(shell grep -E '^module' <go.mod | sed -E 's/module[[:space:]]+//')
COUNT?=1
TEST?=./...
export GOTOOLCHAIN ?= local

.PHONY: build dev test testacc generate plugin-check fmt vet

build:
	go build -o ${BINARY}

dev:
	go build -o ${BINARY}
	packer plugins install --path ${BINARY} "$(shell echo "${PLUGIN_FQN}" | sed 's/packer-plugin-//')"

fmt:
	gofmt -s -w .

vet:
	go vet ./...

test: fmt vet
	go test -race -count=$(COUNT) $(TEST) -timeout=3m

testacc: dev
	PACKER_ACC=1 go test -count=$(COUNT) -v ./builder/evolution -timeout=120m

generate:
	go generate ./...

plugin-check: build
	go run github.com/hashicorp/packer-plugin-sdk/cmd/packer-sdc@v0.5.4 plugin-check ${BINARY}
