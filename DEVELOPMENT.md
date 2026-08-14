# Development

## Getting started

1. Install and configure [pre-commit](https://pre-commit.com/) for the repository
2. Install [nektos/act](https://github.com/nektos/act) for testing
<!-- TODO add secrets info -->

## Testing locally

Use [nektos/act](https://github.com/nektos/act) to run actions locally.
NB: You'll want to use the `large` runner in order to have access to all commands needed for testing.

```
act -e testdata/act/pull-request.json
```

_Read more: [Example commands](https://github.com/nektos/act#example-commands)_

## Publishing a release

1. Move `[Unreleased]` notes into a new version section in [CHANGELOG.md](CHANGELOG.md)
2. Set [internal/version/version.go](internal/version/version.go) to match
3. Update the default `dockerImage` tag in [docker/action.yml](docker/action.yml) (and README examples) to the new semver
4. Create the GitHub release / Marketplace publish (manual publish step — see [GitHub docs](https://docs.github.com/en/actions/creating-actions/publishing-actions-in-github-marketplace#publishing-an-action))
5. Maintain the major floating tag (`v2` for 2.x) in addition to the semver tag
6. Publish the runtime Docker image to Docker Hub (required for the optional `/docker` entry point):
   - Prefer tagging `vX.Y.Z` on `main` (triggers [.github/workflows/publish-image.yml](.github/workflows/publish-image.yml)), **or**
   - Run **Publish Docker image** via `workflow_dispatch` with `version: X.Y.Z` (builds from existing git tag `vX.Y.Z`)
   - Credentials: same path as other public LD images — `release-secrets` assumes `vars.AWS_ROLE_ARN` and reads `/global/services/docker/public/username` + `token` from SSM (no repo `DOCKERHUB_*` secrets)
   - Image: `launchdarkly/find-code-references-in-pull-request:X.Y.Z` (and `latest` when enabled) — must match the default `dockerImage` in [docker/action.yml](docker/action.yml)

**Publishing** to the Marketplace is a manual step even if automation is used to create a release.

### Versioning

We use [semantic versioning](https://semver.org/)_ AND a major version release tag for users of the action

Example: latest release of v1.3.0 will also be available at tag v1.
