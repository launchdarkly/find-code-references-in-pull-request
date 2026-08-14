## [Unreleased]

### Added

### Changed

- Docker Hub image publish workflow loads credentials via `release-secrets` / SSM (same as other public LD images) instead of repo `DOCKERHUB_*` secrets

### Fixed

## 2.3.0

### Added

- optional GitHub Action entry point `launchdarkly/find-code-references-in-pull-request/docker` with a `dockerImage` input so workflows can pull a prebuilt runtime image from a private registry or Docker Hub proxy. The root Action is unchanged.
- multi-stage `Dockerfile` and Docker Hub publish workflow for `launchdarkly/find-code-references-in-pull-request` runtime images

### Changed

### Fixed

## 2.2.0

### Added

- Support for large pull request diffs by falling back to `git diff` when the GitHub API returns a 406 response

### Changed

- Update dependencies
- Update Go version

### Fixed

- False flag removals when `max-flags` stopped scanning mid-file
- Delete-then-re-add flag churn is now reported as modified instead of removed

## 2.1.0

### Added

- Add support for enterprise hosted github instances [More info](https://github.com/launchdarkly/find-code-references-in-pull-request/issues/102)

### Changed

### Fixed

 - Potential null pointer error when creating flag links

## 2.0.2

### Changed

- Update dependencies

## 2.0.1

### Changed

- Update dependencies
- Update golang version
- Migrate from CircleCI to Github Actions

## 2.0.0

### Added

- [Breaking change] Create flag links will be on by default. Ensure your access token has the required `createFlagLink` role.

### Changed

- Enable scanning github workflow files for flag references. [More info](https://github.com/launchdarkly/ld-find-code-refs/pull/441)

## 1.3.0

### Added

- Add an info warning for changes flags that have been [deprecated](https://docs.launchdarkly.com/home/code/flag-archive#deprecating-flags)

### Changed

- Update info message for removed, but not extinct flags
- Update dependencies

## 1.2.0

### Added

- Automatically create [flag links](https://docs.launchdarkly.com/home/organize/links) for flags modified in the pull request

### Changed

- Update dependencies

## 1.1.1

### Changed

- Update dependencies

### Fixed

- Incorrect scanning for extinctions of removed flags led to false positives

## 1.1.0

### Added

- Indicate if a removed flag has all references removed
  - Output `any-extinct`, `extinct-flags-count`, `extinct-flags`

### Changed

- Update the comment design
- Update dependencies

### Fixed

- Detect aliases for removed flags
- Wrong output set for `any-removed`, `removed-flags-count`, `removed-flags`

## 1.0.1

### Changed

- Update dependencies

## 1.0.0

Initial release!

Find flags that have changed in your pull requests.

Read docs: https://github.com/launchdarkly/find-code-references-in-pull-request 
