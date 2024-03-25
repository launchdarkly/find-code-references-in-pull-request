## [Unreleased]

### Added

### Changed

### Fixed

## 1.3.0

### Added

- Add an info warning for changes flags that have been [deprecated](https://docs.launchdarkly.com/home/code/flag-archive#deprecating-flags)

### Changed

- Update info message for removed, but not extinct flags

### Fixed

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
