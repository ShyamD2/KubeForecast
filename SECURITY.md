# Security Policy

## Supported Versions

KubeForecast follows semantic versioning and actively maintains the following release branches:

| Version | Supported | Security Maintenance Status |
| :--- | :--- | :--- |
| `v1.x` | :white_check_mark: | Active production releases & CVE patching |
| `< v1.0` | :x: | End of Life (EOL) |

---

## Reporting a Vulnerability

We take the security of our Kubernetes scheduling plugins, mutating admission webhooks, and cloud infrastructure seriously. If you discover a security vulnerability, please report it responsibly:

### How to Report
- **Email**: Send vulnerability reports directly to `dshyamkumar021@gmail.com`.
- **Subject**: `[SECURITY VULNERABILITY] KubeForecast - <Component Name>`
- **Content**: Please include:
  - Component affected (`cmd/webhook`, `internal/scheduler`, `internal/eviction`, `terraform/`, `charts/`).
  - Step-by-step reproduction instructions or proof-of-concept (PoC).
  - Severity assessment (CVSS or impact on cluster availability/confidentiality).
  - Potential mitigation if identified.

### Response SLA
- **Initial Acknowledgment**: Within **24 hours**.
- **Triage & Impact Assessment**: Within **48 hours**.
- **Fix & Disclosure Schedule**: Within **7 business days** for high/critical vulnerabilities.

Please **do not** report vulnerabilities through public GitHub issues or discussions.

---

## Security Architecture & Invariants

KubeForecast is engineered with enterprise DevSecOps principles:
1. **Least-Privilege RBAC**: Service accounts only bind to required verbs (`get`, `list`, `watch`, `update` on nodes and pods). Cluster-admin is strictly forbidden.
2. **IAM Roles for Service Accounts (IRSA)**: Zero static AWS access keys or secrets in manifests. Pods authenticate via OIDC federation.
3. **Non-Root Container Execution**: All container images are compiled as static Go binaries and run as unprivileged non-root users (`USER 65534:65534` or distroless `nobody`).
4. **Admission Webhook Fail-Safe**: Configured with `failurePolicy: Ignore` and a strict 3-second timeout, ensuring cluster API availability is never compromised even if the webhook pod restarts.
5. **Pod Disruption Budget (PDB) Protection**: The gentle eviction reconciler validates `DisruptionsAllowed > 0` before emitting any eviction request.
