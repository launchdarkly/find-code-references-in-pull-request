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

Versions come from [release-please](https://github.com/googleapis/release-please), which reads [Conventional Commit](https://www.conventionalcommits.org) PR titles (enforced by `lint-pr-title.yml`): `fix:` bumps the patch version, `feat:` the minor version, and `feat!:` or a `BREAKING CHANGE:` footer the major version. `chore:`, `docs:`, `ci:` and changes under `.github/` do not trigger a release.

1. Merge PRs to `main` as usual. release-please keeps a `chore(main): release X.Y.Z` PR open that bumps [internal/version/version.go](internal/version/version.go), the default `dockerImage` in [docker/action.yml](docker/action.yml) and the README pin, and adds the [CHANGELOG.md](CHANGELOG.md) entry.
2. Review the release PR. Edit `CHANGELOG.md` on its branch if the generated notes need more context.
3. Merge the release PR. [.github/workflows/release-please.yml](.github/workflows/release-please.yml) then:
   - creates a draft GitHub release and pushes the `vX.Y.Z` tag at the release commit
   - builds and pushes `launchdarkly/find-code-references-in-pull-request:X.Y.Z` and `latest` to Docker Hub (via the shared [.github/workflows/publish.yml](.github/workflows/publish.yml)). Credentials come from `release-secrets`, which assumes `vars.AWS_ROLE_ARN` and reads `/global/services/docker/public/username` + `token` from SSM, same as `ld-find-code-refs`
   - publishes the release and moves the major tag (`v2`) to it, which is when `@v2` users pick it up. If the image push fails, the release stays a draft and `v2` does not move
4. Publish the release to the Marketplace by hand if needed (see [GitHub docs](https://docs.github.com/en/actions/creating-actions/publishing-actions-in-github-marketplace#publishing-an-action)); releases created by the workflow are not listed automatically.

Every file that carries the version is annotated with `x-release-please-version` and listed under `extra-files` in [release-please-config.json](release-please-config.json). Add any new one there.

### Dry runs and recovery

**Manual Publish** ([.github/workflows/manual-publish.yml](.github/workflows/manual-publish.yml)) runs the same `publish.yml` against an existing tag. Leave `dry_run` checked to build the image without pushing anything. To recover a release whose publish job failed, uncheck `dry_run` and check `publish_release`; that pushes the image, publishes the release and moves `v2`.

### Versioning

We use [semantic versioning](https://semver.org/)_ AND a major version release tag for users of the action

Example: latest release of v1.3.0 will also be available at tag v1.
