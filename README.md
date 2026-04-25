# terradrift

Detect drift between your Terraform state and your real cloud infrastructure — locally and in CI.

`terradrift` compares the resources recorded in a Terraform state file against the resources currently present in the cloud, then prints a `terraform plan`-style diff so you can spot:

- **Unmanaged** resources that exist in the cloud but not in your state.
- **Missing** resources that exist in your state but not in the cloud.
- **Drifted** resources whose attributes diverge between the two.

Supported providers today: **AWS** (EC2 instances, S3 buckets, security groups, IAM roles).

## CLI

```bash
terradrift scan \
  --provider aws \
  --region eu-west-1 \
  --state terraform.tfstate
```

State sources are pluggable: a local path, `s3://bucket/key`, `http(s)://...`, or `-` for stdin.

### Useful flags

| Flag | Purpose |
| --- | --- |
| `--type aws_instance` | Scope the scan to a single resource type. |
| `--ignore-file .driftignore` | Suppress resources you knowingly manage outside Terraform (gitignore-style globs). |
| `--no-color` | Disable ANSI color (forced; auto-detected for non-TTY output). |
| `--quiet` | Print only the summary line. |
| `--exit-code=false` | Always exit 0 on a successful scan; errors still exit 2. |

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Clean scan, no drift. |
| `1` | Drift detected (unmanaged, missing, or drifted resources). |
| `2` | Scan could not complete (bad arguments, unreadable state, cloud-provider failure). |

### `.driftignore`

A gitignore-style file that suppresses resources from the unmanaged/drifted sections. Patterns match `<type>.<name>` (or `<type>.<id>` when the resource has no Name).

```
# .driftignore
aws_instance.web-2
aws_s3_bucket.temp-*
aws_iam_role.*
```

`terradrift` looks for `.driftignore` in the current directory first, then walks upward to the git repository root. Override with `--ignore-file <path>`.

## GitHub Action

Run `terradrift` on every PR or on a schedule. Drift becomes a failed step that you can gate releases on.

```yaml
name: Drift Detection

on:
  schedule:
    - cron: '0 6 * * *'
  pull_request:
    paths:
      - 'terraform/**'
      - '**/*.tfstate'

permissions:
  contents: read
  pull-requests: write

jobs:
  drift:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: eu-west-1

      - uses: esanchezm/terradrift@main
        with:
          provider: aws
          state-path: terraform.tfstate
          region: eu-west-1
          fail-on-drift: true
```

A complete example, including PR comment behaviour and step outputs, lives in [`examples/drift-check.yml`](examples/drift-check.yml).

> **Versioning note:** the action is referenced as `@main` until the first stable tag is published. Pin to a commit SHA (`@<40-char-sha>`) for reproducibility, or to `@v1` once that tag exists.

### Inputs

| Input | Required | Default | Description |
| --- | --- | --- | --- |
| `provider` | no | `aws` | Cloud provider. |
| `state-path` | **yes** | — | Local path, `s3://...`, `http(s)://...`, or `-` for stdin. |
| `region` | no | `''` | Cloud region (e.g., `eu-west-1`). |
| `type` | no | `''` | Single resource type filter (e.g., `aws_instance`). |
| `ignore-file` | no | `''` | Custom `.driftignore` path; otherwise auto-discovered. |
| `fail-on-drift` | no | `true` | Exit 1 on drift. Set `false` to report without failing CI. |
| `quiet` | no | `false` | Print only the summary line. |
| `comment-on-pr` | no | `true` | On pull_request events, post the scan result as a PR comment. |

### Outputs

| Output | Description |
| --- | --- |
| `exit-code` | `0` (clean), `1` (drift), or `2` (error). |
| `drift-detected` | `"true"` or `"false"`. |

### What the action does

1. Translates inputs into `terradrift scan` flags.
2. Runs the scan inside the action container.
3. Mirrors the rendered report to the workflow log.
4. Writes a Markdown summary to `$GITHUB_STEP_SUMMARY` (visible on the Actions UI).
5. On `pull_request` events with a token, posts the same summary as a PR comment.
6. Exits with `0` / `1` / `2` so `if: failure()` and similar gates work.

AWS credentials are not an action input — use [`aws-actions/configure-aws-credentials`](https://github.com/aws-actions/configure-aws-credentials) (or any other mechanism that exports the standard AWS env vars) before this step.

## License

MIT — see [LICENSE](LICENSE).
