SHELL := /bin/bash
GO ?= $(shell which go 2>/dev/null || echo /home/kamran/.local/go/bin/go)
TF ?= $(shell which tofu terraform 2>/dev/null | head -n 1)
CONFTEST ?= $(shell which conftest 2>/dev/null || echo /home/kamran/Downloads/Code/guardrails-demo/.tools/conftest)

.PHONY: all test build fmt validate-tf policy-check lint-scripts demo-report clean

all: test fmt policy-check lint-scripts

test:
	cd telemetry && $(GO) vet ./... && $(GO) test -race -v ./...

build:
	cd telemetry && mkdir -p bin && $(GO) build -o bin/validator-etl ./cmd/validator-etl

demo-report:
	cd telemetry && $(GO) run ./cmd/validator-etl --demo-report

fmt:
	$(TF) fmt -check -recursive terraform/

validate-tf:
	for env in terraform/envs/testnet terraform/envs/mainnet; do \
		(cd "$$env" && $(TF) init -backend=false -input=false && $(TF) validate); \
	done

policy-check:
	$(CONFTEST) test -p policy/terraform_validator.rego --parser hcl2 \
		terraform/modules/aws-validator/main.tf \
		terraform/modules/gcp-validator/main.tf \
		terraform/envs/testnet/main.tf \
		terraform/envs/mainnet/main.tf
	! $(CONFTEST) test -p policy/terraform_validator.rego --parser hcl2 terraform/insecure-draft/main.tf
	$(CONFTEST) test -p policy/k8s_validator.rego k8s/base/*.yaml
	! $(CONFTEST) test -p policy/k8s_validator.rego k8s/insecure-draft/bad-validator-pod.yaml

lint-scripts:
	bash -n bare-metal/scripts/*.sh
	python3 -c 'import glob, json, yaml; [json.load(open(f)) for f in glob.glob("**/*.json", recursive=True)]; [list(yaml.safe_load_all(open(f))) for f in glob.glob("**/*.y*ml", recursive=True)]; print("All JSON and YAML valid")'

clean:
	rm -rf telemetry/bin /tmp/validator-etl
