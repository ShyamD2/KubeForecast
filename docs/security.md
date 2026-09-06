# Security Model & Hardening

## Defense-in-Depth Architecture
Security is designed into every layer of the Predictive Scheduling & Cost Optimization Engine.

---

## 1. Zero Hardcoded Credentials & IRSA
- The codebase contains **zero hardcoded credentials**, API keys, or long-lived AWS secrets.
- All AWS authentication (S3 snapshot storage and ECR image access) is performed using **IAM Roles for Service Accounts (IRSA)**.
- Authentication tokens are projected directly into the pod via the EKS OIDC identity provider (`sts:AssumeRoleWithWebIdentity`).

---

## 2. Least-Privilege IAM Policy
The IAM policy attached to the IRSA role is strictly scoped:
- `s3:ListBucket` is restricted to the specific bucket ARN and prefixes (`kubeforecast/*`, `snapshots/*`, `reports/*`).
- `s3:PutObject` and `s3:GetObject` are strictly scoped to the specific bucket.
- No destructive permissions (`s3:DeleteBucket`, `s3:DeleteObject`) are granted.

---

## 3. Container & Runtime Hardening
All four container images follow a strict distroless build pattern:
- **Multi-stage build**: Static compilation with stripped symbols (`-w -s`), zero shell or package manager in final image.
- **Non-Root Execution**: Runs as unprivileged user `65532:65532` (`runAsNonRoot: true`).
- **Read-Only Root Filesystem**: Filesystem is mounted read-only (`readOnlyRootFilesystem: true`).
- **Dropped Linux Capabilities**: All default Linux capabilities are explicitly dropped (`capabilities.drop: ["ALL"]`).
- **No Privilege Escalation**: `allowPrivilegeEscalation: false`.

---

## 4. Sensitive Data Sanitization
Before saving any snapshot to disk or Amazon S3, the `storage.SanitizeState()` routine runs automatically:
- Strips pod environment variables, configmap data, and secret volumes.
- Strips container command arguments and proprietary labels.
- Only retains operational metadata: namespace, pod name, requested CPU/memory, node name, and scheduling group.

---

## 5. Network Isolation
The chart deploys an explicit `NetworkPolicy`:
- Webhook ingress is restricted strictly to the Kubernetes API server.
- Metrics port ingress is restricted strictly to Prometheus scrapers.
- Egress is restricted strictly to DNS (port 53), the internal API server (port 443/6443), and outbound HTTPS to AWS S3.
