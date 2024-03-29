# Development

## Getting started

1. Install and configure [pre-commit](https://pre-commit.com/) for the repository
2. Install [nektos/act](https://github.com/nektos/act) for testing
<!-- TODO add secrets info -->

## Testing locally

Use [nektos/act](https://github.com/nektos/act) to run actions locally.

```
act
```

_Read more: [Example commands](https://github.com/nektos/act#example-commands)_

## Publishing a release

Make sure [CHANGELOG.md](CHANGELOG.md) and [version](internal/version/version.go) are updated

Follow instructions to [publish a release to the GitHub Marketplace](https://docs.github.com/en/actions/creating-actions/publishing-actions-in-github-marketplace#publishing-an-action).

**Publishing** is a manual step even if automation is used to create a release.

### Versioning

We use [semantic versioning](https://semver.org/)_ AND a major version release tag for users of the action

Example: latest release of v1.3.0 will also be available at tag v1.
