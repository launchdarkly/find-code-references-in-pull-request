# AGENTS.md

Guidance for AI agents and contributors working in this repository.

Human-facing docs: [README.md](README.md), [DEVELOPMENT.md](DEVELOPMENT.md), [CONTRIBUTING.md](CONTRIBUTING.md). Prefer those for end-user usage; this file is for how to change the action safely.

## What this repo is

Docker-based GitHub Action that scans a PR diff for LaunchDarkly feature flag keys (and aliases), then:

1. Sets workflow outputs (`any-changed`, `modified-flags`, `extinct-flags`, etc.)
2. Optionally comments on the PR (and updates/deletes an existing LaunchDarkly comment)
3. Optionally creates LaunchDarkly [flag links](https://docs.launchdarkly.com/home/organize/links)

Published as `launchdarkly/find-code-references-in-pull-request` (current major: **v2**). Owners: `@launchdarkly/team-foundation` ([CODEOWNERS](CODEOWNERS)).

**Not supported:** monorepos / searching across multiple LaunchDarkly projects.

## Architecture (keep this mental model)

Pipeline in [`main.go`](main.go):

```
config → LD flags → PR diff → preprocess → match → summarize → outputs → comment → flag links
```

| Package / path | Role |
| --- | --- |
| `action.yml` + `Dockerfile` | Root Action contract and container build (multi-stage; final image has binary + `git`) |
| `docker/action.yml` | Opt-in composite entry point with `dockerImage` override (`docker run`) |
| `config/` | Parse/validate `INPUT_*` env vars; GitHub client (incl. GHE) |
| `diff/` | Preprocess PR diffs; ignore paths; scan hunks into the builder |
| `search/` | Build matcher via `ld-find-code-refs` + alias generation |
| `internal/references/` | Cap flags (`max-flags`); net add/delete → outputs `modified-*` (`FlagsAdded`), `removed-*`, optional `extinct-*` |
| `internal/extinctions/` | Confirm removed flags are gone from the workspace |
| `comments/` | Render PR comment HTML/Markdown from flag metadata |
| `internal/ldclient/` | LaunchDarkly API: list flags, create flag links |
| `internal/github_actions/` | `GITHUB_OUTPUT` via `SetOutput`, masks, log groups, notices |
| `internal/version/` | Semver string for releases (`Version`) |
| `ignore/` | `.ldignore` / ignore rules for scanned paths |
| `vendor/` | Vendored Go modules — **do not hand-edit** |

Upstream library: [`launchdarkly/ld-find-code-refs`](https://github.com/launchdarkly/ld-find-code-refs) (aliases, matchers, YAML options). Alias config comes from `.launchdarkly/coderefs.yaml` when present in the consumer repo (Viper also accepts `coderefs.yml`; see README / ld-find-code-refs docs).

## Commands

```bash
# Unit tests
go test ./...
# Closer to CI:
# gotestsum --packages="./..." -- -tags=launchdarkly_easyjson -p=1

# Lint (matches pre-commit)
pre-commit run --all-files
# or: golangci-lint run -D funlen -D bodyclose -D typecheck

# Regenerate README inputs/outputs tables from action.yml
make docs

# After go.mod changes — always vendor
go mod tidy && go mod vendor

# Local e2e with nektos/act (use large runner; secrets from .secrets — see .secrets.example.env)
act -e testdata/act/pull-request.json
```

Go version comes from `go.mod` (`go-version-file: go.mod` in CI). Current module requires **Go 1.25**.

## Changing behavior — required sync points

When you add/change an input or output:

1. Update `action.yml`
2. Wire parsing/defaults in `config/config.go` (and use in `main.go` / packages)
3. Run `make docs` so README tables stay in sync (`<!-- action-docs-* -->` blocks)
4. Add a `CHANGELOG.md` entry under `## [Unreleased]`
5. Add/extend unit tests near the logic (`diff/`, `comments/`, `internal/references/`)

When bumping dependencies:

1. Update `go.mod` / `go.sum`
2. Run `go mod tidy && go mod vendor`
3. Prefer splitting **huge** vendor diffs across PRs if needed — the action’s own e2e job fetches the PR diff via the GitHub API and historically failed on oversized diffs (now has `git diff` fallback on HTTP 406, but keep PRs reviewable)

When bumping `ld-find-code-refs`, note behavioral changes that affect scanning (e.g. workflow file scanning) in the CHANGELOG.

## Invariants agents must not break

- **`max-flags`**: Finish scanning each diff **file** before applying the cap. Stopping mid-file caused false removals ([#232](https://github.com/launchdarkly/find-code-references-in-pull-request/pull/232)).
- **Net classification**: Classify by net add/delete counts so delete-then-re-add churn is **modified**, not removed.
- **Large diffs**: GitHub API may return **406**; fallback path uses `git diff` between merge-base and head ([#172](https://github.com/launchdarkly/find-code-references-in-pull-request/pull/172)). Preserve that path and keep `git` in the Docker image.
- **Comment identity**: Existing comments are found by body containing `LaunchDarkly flag references`. Do not change that marker without a migration plan.
- **`skip-comment` polarity**: Always call `setOutputs` and still build the comment body for flag-link dedupe. When `skip-comment` is `true`, do **not** call `postGithubComment` (skip ⇒ no GitHub comment write). Match `action.yml` / README, not any inverted condition in `main.go`.
- **Tokens**: Mask `access-token` / `repo-token` via `gha.MaskInput`. Never log secrets. Never commit `.secrets` or real tokens (only `.secrets.example.env`).
- **GHE**: Non-`https://github.com` `GITHUB_SERVER_URL` must keep Enterprise client URLs (`config.getGithubClient`).
- **Defaults**: Treat `action.yml` defaults as the product contract (`create-flag-links` default `true` was a v2 breaking change).

## PR and commit conventions

Recent merges use conventional-ish prefixes:

- `feat:`, `fix:`, `chore:`, `docs:`, `chore(deps):`
- Optional Jira: `[REL-xxxxx]` in title/body when tracked
- Release prep: `prepare X.Y.Z release` — bump `CHANGELOG.md` + `internal/version/version.go` together

**PR checklist for agents:**

- [ ] Scope is focused; avoid mixing feature work with large unrelated vendor bumps
- [ ] Tests for classification / comment / config behavior when logic changes
- [ ] `CHANGELOG.md` `[Unreleased]` updated for user-visible changes
- [ ] `make docs` if `action.yml` changed
- [ ] `go mod vendor` committed when modules change
- [ ] No hand-edits under `vendor/` except via `go mod vendor`

Do not add reviewers; CODEOWNERS / maintainers handle that ([CONTRIBUTING.md](CONTRIBUTING.md)).

### Fork PRs and CI

- Same-repo PRs: `.github/workflows/main.yml` runs `go-test`, `e2e-tests` (needs secrets), `e2e-docker-entrypoint` (local + GHCR mirror `/docker` path), and `generate-docs`.
- Fork PRs: secret-dependent jobs are skipped on `pull_request`. Maintainers add the **`safe-to-test`** label to run `.github/workflows/test-fork-pr.yml` (`pull_request_target`). `.github/workflows/remove-safe-to-test-label.yml` strips the label on new pushes — do not “fix around” that security gate.
- e2e uses this action against itself (`uses: ./`) with sandbox project credentials.

## Releases

Follow [DEVELOPMENT.md](DEVELOPMENT.md):

1. Move `[Unreleased]` notes into a new version section in `CHANGELOG.md`
2. Set `internal/version/version.go` to match
3. Bump default `dockerImage` in `docker/action.yml` (+ README pins) to the new semver
4. Publish via GitHub Marketplace release flow (manual publish step)
5. Maintain major floating tag (`v2` for 2.x) in addition to semver tags
6. Publish the runtime image `launchdarkly/find-code-references-in-pull-request:X.Y.Z` via `.github/workflows/publish-image.yml` (tag push or `workflow_dispatch`; Hub creds via `release-secrets` + `vars.AWS_ROLE_ARN` / SSM, same as `ld-find-code-refs`)

Optional follow-up: thin the root `Dockerfile` to `FROM` that published image so default `@v2` users also skip compile-on-run.

## Security and boundaries

- **Never** commit secrets, PATs, or LaunchDarkly tokens
- **Never** weaken `safe-to-test` / `pull_request_target` controls without explicit maintainer request
- Prefer fixing vulns by upgrading modules + vendoring (see Dependabot / remediation PRs)
- Do not expand scope to monorepo multi-project scanning unless that is an explicit product decision with docs + CHANGELOG

## Good first places to look

| Task | Start here |
| --- | --- |
| New input / output | `action.yml`, `config/config.go`, `main.go`, `make docs` |
| False positive/negative flags | `diff/`, `internal/references/`, `search/` |
| Comment formatting | `comments/` |
| Flag links / LD API | `internal/ldclient/` |
| Extinct flags | `internal/extinctions/`, `check-extinctions` input |
| CI / fork testing | `.github/workflows/` |
| Version / release | `CHANGELOG.md`, `internal/version/version.go` |
