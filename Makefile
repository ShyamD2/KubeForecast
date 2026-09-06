.PHONY: all build test lint docker-build local-up local-down benchmark clean

IMAGE_REGISTRY ?= localhost:5000
TAG ?= latest

all: build test

build:
	@echo "Building Go binaries..."
	go build -v ./cmd/...

test:
	@echo "Running unit and integration tests..."
	go test -v -race ./...

lint:
	@echo "Formatting and vetting Go code..."
	go vet ./...
	gofmt -s -w .

benchmark:
	@echo "Running benchmark simulation across all workload profiles..."
	go run ./cmd/simulator/main.go --scenario all --output-dir ./experiments/results

docker-build:
	@echo "Building multi-stage Docker images..."
	docker build -f Dockerfile.webhook -t $(IMAGE_REGISTRY)/predictive-webhook:$(TAG) .
	docker build -f Dockerfile.simulator -t $(IMAGE_REGISTRY)/predictive-simulator:$(TAG) .
	docker build -f Dockerfile.controller -t $(IMAGE_REGISTRY)/predictive-controller:$(TAG) .
	docker build -f Dockerfile.scheduler -t $(IMAGE_REGISTRY)/predictive-scheduler:$(TAG) .

local-up:
	@echo "Bootstrapping local kind cluster..."
	kind create cluster --name predictive-scheduler --config - <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
- role: worker
EOF
	@echo "Installing Helm chart into kind..."
	helm upgrade --install predictive-scheduler ./charts/predictive-scheduler --namespace predictive-scheduler --create-namespace

local-down:
	@echo "Destroying local kind cluster..."
	kind delete cluster --name predictive-scheduler

clean:
	@echo "Cleaning artifacts..."
	rm -rf ./bin ./experiments/results/snapshots
