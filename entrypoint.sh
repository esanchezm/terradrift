#!/bin/sh
# entrypoint.sh: GitHub Action wrapper around the terradrift binary.
#
# Inputs are passed positionally via runs.args in action.yml because
# GitHub Actions exposes hyphenated inputs (state-path, ignore-file,
# fail-on-drift, comment-on-pr, github-token) as env vars with literal
# hyphens (INPUT_STATE-PATH), which are unreadable in POSIX shell.
# Positional arguments sidestep that name-mangling entirely.
#
# The script preserves terradrift's three-state exit code:
#   0 = no drift, 1 = drift detected, 2 = error
# A failed PR-comment post is logged as a warning but does not change
# the exit code; CI gating must depend on the scan result, not on the
# availability of the GitHub API.

set -eu

# Positional arguments — order MUST match action.yml runs.args.
PROVIDER="${1:-aws}"
STATE_PATH="${2:-}"
REGION="${3:-}"
TYPE_FILTER="${4:-}"
IGNORE_FILE="${5:-}"
FAIL_ON_DRIFT="${6:-true}"
QUIET="${7:-false}"
COMMENT_ON_PR="${8:-true}"
INPUT_TOKEN="${9:-}"

if [ -z "$STATE_PATH" ]; then
    echo "::error::state-path input is required" >&2
    exit 2
fi

# Build the terradrift argv. Quoting matters: user-supplied values may
# contain spaces, so we use the shell's positional set as an array
# rather than concatenating into a single string.
set -- scan \
    --provider "$PROVIDER" \
    --state "$STATE_PATH" \
    --no-color

if [ -n "$REGION" ]; then
    set -- "$@" --region "$REGION"
fi
if [ -n "$TYPE_FILTER" ]; then
    set -- "$@" --type "$TYPE_FILTER"
fi
if [ -n "$IGNORE_FILE" ]; then
    set -- "$@" --ignore-file "$IGNORE_FILE"
fi
if [ "$QUIET" = "true" ]; then
    set -- "$@" --quiet
fi
if [ "$FAIL_ON_DRIFT" = "false" ]; then
    set -- "$@" --exit-code=false
fi

# Run terradrift, capture stdout+stderr together, remember exit code.
# `set +e` is required so the rest of the script still runs when the
# scan exits non-zero (drift = 1, error = 2).
set +e
OUTPUT="$(/usr/local/bin/terradrift "$@" 2>&1)"
EXIT_CODE=$?
set -e

# Mirror to the action log so users can read it in the Actions UI even
# without expanding the job summary.
printf '%s\n' "$OUTPUT"

case "$EXIT_CODE" in
    0)
        STATUS_HEADER="✅ **No drift detected** — your cloud matches your Terraform state."
        DRIFT_BOOL="false"
        ;;
    1)
        STATUS_HEADER="⚠️ **Drift detected** — see scan output below."
        DRIFT_BOOL="true"
        ;;
    *)
        STATUS_HEADER="❌ **Scan failed** — see scan output below."
        DRIFT_BOOL="false"
        ;;
esac

# Job summary: rendered as Markdown in the Actions UI. Always written
# when GITHUB_STEP_SUMMARY is set, regardless of event type.
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    {
        echo "## terradrift drift scan"
        echo ""
        echo "$STATUS_HEADER"
        echo ""
        echo '```'
        printf '%s\n' "$OUTPUT"
        echo '```'
    } >> "$GITHUB_STEP_SUMMARY"
fi

# Action outputs (consumed by `steps.<id>.outputs.*` in downstream steps).
if [ -n "${GITHUB_OUTPUT:-}" ]; then
    {
        printf 'exit-code=%s\n' "$EXIT_CODE"
        printf 'drift-detected=%s\n' "$DRIFT_BOOL"
    } >> "$GITHUB_OUTPUT"
fi

# Resolve the token: prefer the input (forwarded positional), fall back
# to GITHUB_TOKEN env (some users may pass it via env: in the workflow).
EFFECTIVE_TOKEN="${INPUT_TOKEN:-${GITHUB_TOKEN:-}}"

# PR comment: only when the workflow trigger was a pull_request, the
# user opted in (comment-on-pr=true, the default), a token is
# available, and we can extract the PR number from the event payload.
# Failures here are warnings, never errors: the scan result is the
# source of truth for CI status.
if [ "${GITHUB_EVENT_NAME:-}" = "pull_request" ] \
   && [ "$COMMENT_ON_PR" = "true" ] \
   && [ -n "$EFFECTIVE_TOKEN" ] \
   && [ -n "${GITHUB_EVENT_PATH:-}" ]; then
    PR_NUMBER="$(jq -r '.pull_request.number // .number // empty' "$GITHUB_EVENT_PATH")"
    if [ -n "$PR_NUMBER" ] && [ -n "${GITHUB_REPOSITORY:-}" ]; then
        # GitHub caps issue/PR comments at 65536 chars. Reserve ~1.5 KB
        # for the markdown header and code-fence wrapper so the body
        # fits even after JSON escaping inflates some characters.
        MAX_BODY=64000
        TRUNCATED_OUTPUT="$OUTPUT"
        OUTPUT_LEN=$(printf '%s' "$OUTPUT" | wc -c)
        if [ "$OUTPUT_LEN" -gt "$MAX_BODY" ]; then
            TRUNCATED_OUTPUT="$(printf '%s' "$OUTPUT" | head -c "$MAX_BODY")
... (output truncated; see job summary or step logs for the full report)"
        fi
        BODY_MD="$(printf '## terradrift drift scan\n\n%s\n\n```\n%s\n```\n' "$STATUS_HEADER" "$TRUNCATED_OUTPUT")"
        # jq -Rs reads the entire stdin as a single raw string and
        # produces a JSON-escaped string literal. This is the safe way
        # to embed arbitrary terradrift output (which can contain
        # quotes, backticks, control characters) into a JSON POST body.
        BODY_JSON="$(printf '%s' "$BODY_MD" | jq -Rs .)"
        if ! curl --silent --show-error --fail \
            -X POST \
            -H "Authorization: Bearer $EFFECTIVE_TOKEN" \
            -H "Accept: application/vnd.github+json" \
            -H "X-GitHub-Api-Version: 2022-11-28" \
            "https://api.github.com/repos/$GITHUB_REPOSITORY/issues/$PR_NUMBER/comments" \
            -d "{\"body\": $BODY_JSON}" > /dev/null
        then
            echo "::warning::Failed to post PR comment (continuing). Check the workflow has 'pull-requests: write' permission and the github-token has the right scope." >&2
        fi
    else
        echo "::warning::Could not determine PR number from $GITHUB_EVENT_PATH; skipping PR comment." >&2
    fi
fi

exit "$EXIT_CODE"
