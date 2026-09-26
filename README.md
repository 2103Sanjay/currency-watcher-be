# Currency Watcher — Backend

Go API for Currency Watcher. It serves live exchange rates for currency pairs (e.g. USD → EUR, SGD → JPY), fetched from the free [Frankfurter API](https://frankfurter.dev) (v2, ~160 currencies) and cached in memory.

- Go 1.23, standard library `net/http` (only dependency: `golang.org/x/sync`)
- Infrastructure: Terraform (AWS EC2 + ECR) — [`terraform/`](terraform/)
- CI/CD: GitHub Actions — [`.github/workflows/deploy.yml`](.github/workflows/deploy.yml), deploying to EC2

---

## Running locally

Prerequisite: Go 1.23+.

```bash
go run ./cmd/server
# => listening on http://localhost:8080
```

Check it:

```bash
curl http://localhost:8080/api/health
curl "http://localhost:8080/api/rates?base=USD&targets=EUR,SGD"
```

Run the tests:

```bash
go test ./...          # add -race if you have a C toolchain
```

### Configuration

All optional, via environment variables:

| Variable           | Default                          | Purpose                                                 |
| ------------------ | -------------------------------- | ------------------------------------------------------- |
| `PORT`             | `8080`                           | HTTP listen port                                        |
| `CACHE_TTL`        | `1h`                             | How long a fetched rate table stays fresh (Go duration) |
| `UPSTREAM_TIMEOUT` | `10s`                            | Timeout for calls to Frankfurter                        |
| `RATES_API_URL`    | `https://api.frankfurter.dev/v2` | Upstream API base URL                                   |
| `ALLOWED_ORIGINS`  | `http://localhost:5173`          | Comma-separated CORS allow-list (`*` for any)           |

---

## API

### Response format

Every response, successful or not, uses the same envelope:

```json
{
  "status": "Success",
  "message": "Exchange rates fetched successfully",
  "status_code": 200,
  "data": { },
  "timestamp": "2026-09-26T08:33:35Z"
}
```

Failures have `"status": "Failed"`, a human-readable `message`, and no `data`:

```json
{
  "status": "Failed",
  "message": "unknown currency: XYZ",
  "status_code": 400,
  "timestamp": "2026-09-26T08:33:35Z"
}
```

`status_code` always matches the HTTP status. This also applies to unknown paths (`404`), unsupported methods (`405`) and unexpected server errors (`500`).

### `GET /api/rates?base=USD&targets=EUR,SGD`

Returns the rate from `base` to each target. `targets` is optional; if omitted, every available rate for `base` is returned. Codes are case-insensitive. `data`:

```json
{
  "base": "USD",
  "date": "2026-09-26",
  "rates": { "EUR": 0.87735, "SGD": 1.2791 },
  "fetchedAt": "2026-09-26T08:33:35Z",
  "expiresAt": "2026-09-26T09:33:35Z",
  "cached": true,
  "stale": false
}
```

- `cached` is `true` when the response was served without calling Frankfurter.
- `stale` is `true` when Frankfurter was unreachable and an expired cache entry was served instead.

| Status | When |
| --- | --- |
| `400` | Missing/invalid `base`, invalid target code, or a currency Frankfurter doesn't support |
| `502` | Frankfurter is unavailable and nothing is cached for that base |

### `GET /api/health`

`data` is `{"version":"<git sha>"}`.

### `GET /api/currencies`

`data` is the list of currencies currently supported, `[{"code":"EUR","name":"Euro"}, ...]`. The dashboard uses this list to suggest and validate codes.

---

## Infrastructure (Terraform)

Everything in AWS is defined in [`terraform/`](terraform/). By default it deploys to `ap-south-1` (Mumbai).

| File                           | Contents                                                                                                 |
| ------------------------------ | -------------------------------------------------------------------------------------------------------- |
| `versions.tf`                  | Terraform and AWS provider version pins                                                                  |
| `variables.tf`                 | Every input: region, environment, instance type, app/host ports, CORS origins, cache TTL, GitHub repo, … |
| `main.tf`                      | Provider (with default tags on every resource), shared lookups (the latest Amazon Linux 2023 image)      |
| `network.tf`                   | Dedicated VPC, public subnet, internet gateway, security group (API port only; no SSH)                   |
| `ecr.tf`                       | Private image registry, with scan-on-push and a lifecycle rule keeping the last 10 images                |
| `ec2.tf`                       | Instance role (SSM + ECR pull), EC2 instance (IMDSv2, encrypted disk), Elastic IP                        |
| `github_oidc.tf`               | GitHub OIDC provider and the least-privilege deploy role used by CI/CD                                   |
| `outputs.tf`                   | API URL, instance ID, role ARN, and the exact GitHub variables to set                                    |
| `templates/user_data.sh.tftpl` | First-boot script: installs Docker, writes app config, starts the latest image if any                    |

### Recreate the infrastructure

Prerequisites: [Terraform](https://developer.hashicorp.com/terraform/install) ≥ 1.6, and the [AWS CLI](https://aws.amazon.com/cli/) configured with credentials that can create IAM, EC2 and ECR resources (`aws configure`, then check with `aws sts get-caller-identity`). No credentials are stored in this repository.

```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars   # optional; every variable has a default
terraform init      # download the AWS provider
terraform plan      # review: 19 resources to add
terraform apply     # create them (about 2–3 minutes)
terraform output github_actions_variables   # values for GitHub → Settings → Variables
```

The instance has no application running until the first pipeline run deploys it. Tear everything down with `terraform destroy`.

State is stored locally in `terraform/terraform.tfstate`, which is git-ignored. For team use, move it to an S3 backend.

---

## CI/CD

The workflow is [`.github/workflows/deploy.yml`](.github/workflows/deploy.yml):

```
 push to main ─► Test ─► Build image ─► ⏸ Approval ─► Deploy to EC2
 pull request ─► Test ─► Build image
```

| Job               | What it does                                                                                                                                                                                                                                   |
| ----------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Test**          | `gofmt` check, `go vet`, `go test -race` with coverage (shown in the run summary)                                                                                                                                                              |
| **Build image**   | Builds the Docker image with the commit SHA baked in, runs it, and checks that `/api/health` reports that SHA. Saves the image as an artifact.                                                                                                 |
| **Deploy to EC2** | Waits for approval (the `production` environment). Then it pushes _the same image_ to ECR and runs [`scripts/deploy.sh`](scripts/deploy.sh) on the instance through SSM Run Command. Finally it checks that the public URL serves the new SHA. |

Security notes:

- **No AWS keys in GitHub.** The deploy job gets short-lived credentials from GitHub OIDC by assuming an IAM role that trusts only this repository's `production` environment.
- **No SSH.** Deployment goes through SSM, so port 22 stays closed and no key pairs are needed.
- **Automatic rollback.** `deploy.sh` restores the previous container if the new one fails its health check, and the pipeline goes red.

### One-time GitHub setup

1. **Approval gate.** Go to Settings → Environments → **New environment** → `production`. Tick **Required reviewers** and add yourself (leave "Prevent self-review" unticked if you're the only reviewer). Optionally, under _Deployment branches_, allow only `main`.
2. **Variables.** Go to Settings → Secrets and variables → Actions → **Variables** and add the following (all printed by `terraform output`):

   | Variable          | Example                                                         |
   | ----------------- | --------------------------------------------------------------- |
   | `AWS_REGION`      | `ap-southeast-1`                                                |
   | `AWS_ROLE_ARN`    | `arn:aws:iam::123456789012:role/currency-watcher-github-deploy` |
   | `ECR_REPOSITORY`  | `currency-watcher`                                              |
   | `EC2_INSTANCE_ID` | `i-0123456789abcdef0`                                           |
   | `API_URL`         | `http://<elastic-ip>`                                           |

   None of these are secrets, so they are stored as variables and can be read in the logs.

To approve a run, open it in the **Actions** tab and click **Review deployments** → **Approve and deploy**.

---

## Project structure

Requests flow through the layers top to bottom; each layer only calls the one below it.

```
cmd/server/            main: signal handling, calls app.Run
internal/
  app/                 wires the layers together and runs the HTTP server
  config/              environment-based configuration
  route/               URL → handler mapping + CORS, logging, panic-recovery middleware
  handler/             HTTP handlers: parse/validate requests, write JSON responses
  service/             business logic: in-memory rate cache (TTL, singleflight, stale fallback)
  repository/          data access: Frankfurter API client
  model/               domain types shared across layers
test/                  all tests, one package per layer
  testutil/            shared fakes (repository, service, clock)
  app/ config/ route/ handler/ service/ repository/
```

Because the tests live outside the packages they test, they use only exported APIs. Measure coverage of the application code with:

```bash
go test -coverpkg=./internal/... ./test/...
```
