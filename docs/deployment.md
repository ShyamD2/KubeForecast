# Deployment & Operations Guide

## 1. Local Development (Zero AWS Dependencies)

You can build, test, and run the entire simulation and benchmark engine locally without AWS:

```bash
# 1. Compile all binaries
make build
# or directly with Go:
go build -v ./cmd/...

# 2. Run unit and integration test suite
make test
# or:
go test -v ./...

# 3. Run performance benchmarks
go test -bench="." ./tests/benchmarks

# 4. Execute full workload simulation benchmark
make benchmark
# or:
go run ./cmd/simulator/main.go --scenario all --output-dir ./experiments/results
```

---

## 2. AWS EKS Deployment

### Prerequisites
- AWS CLI v2 configured with appropriate IAM credentials (`aws configure`)
- Terraform >= 1.5.0
- Helm >= 3.12.0
- kubectl >= 1.28.0

> [!NOTE]
> If AWS credentials are not configured in your environment, Terraform and Helm commands are marked as:
> `NOT EXECUTED — REQUIRES AWS`
> The exact commands below deploy the production-ready infrastructure.

### Step 1: Provision Cloud Infrastructure with Terraform
```bash
cd terraform/environments/dev

# Initialize modules and provider plugins
terraform init

# Review execution plan
terraform plan -var="aws_region=us-east-1"

# Provision VPC, EKS Cluster, ECR repositories, S3 bucket, and IRSA role
terraform apply -auto-approve -var="aws_region=us-east-1"
```

### Step 2: Configure kubectl
```bash
CLUSTER_NAME=$(terraform output -raw eks_cluster_name)
aws eks update-kubeconfig --name "${CLUSTER_NAME}" --region us-east-1

# Verify cluster connectivity
kubectl get nodes -o wide
```

### Step 3: Build & Push Docker Images to ECR
```bash
ECR_REGISTRY=$(aws sts get-caller-identity --query Account --output text).dkr.ecr.us-east-1.amazonaws.com
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin "${ECR_REGISTRY}"

IMAGE_REGISTRY="${ECR_REGISTRY}/dev" make docker-build
docker push "${ECR_REGISTRY}/dev/predictive-webhook:latest"
docker push "${ECR_REGISTRY}/dev/predictive-simulator:latest"
docker push "${ECR_REGISTRY}/dev/predictive-controller:latest"
docker push "${ECR_REGISTRY}/dev/predictive-scheduler:latest"
```

### Step 4: Deploy Helm Chart
```bash
IRSA_ROLE_ARN=$(terraform output -raw irsa_role_arn)
S3_BUCKET=$(terraform output -raw s3_snapshot_bucket)

helm upgrade --install predictive-scheduler ./charts/predictive-scheduler \
  --namespace predictive-scheduler \
  --create-namespace \
  --set global.imageRegistry="${ECR_REGISTRY}/dev" \
  --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="${IRSA_ROLE_ARN}" \
  --set controller.s3.bucket="${S3_BUCKET}" \
  -f ./charts/predictive-scheduler/values-production.yaml
```

### Step 5: Verify Deployment Health
```bash
kubectl get pods -n predictive-scheduler
kubectl get mutatingwebhookconfigurations
kubectl logs -n predictive-scheduler -l app.kubernetes.io/name=predictive-scheduler-controller --tail=50
```

---

## 3. Teardown
```bash
cd terraform/environments/dev
terraform destroy -auto-approve -var="aws_region=us-east-1"
```
