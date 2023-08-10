# Find Code References in Pull Request

Adds a comment to pull requests (PRs) whenever a feature flag reference is found in a PR diff.

<!-- TODO update this link when repo name changes -->
<img src="https://github.com/launchdarkly/cr-flags/raw/main/images/example-comment.png?raw=true" alt="An example code references PR comment" width="100%">

## Permissions

In order to add a comment to a PR, the `github-token` used requires `write` permission for PRs. Permissions for the workflow may also be specified with:

```yaml
permissions:
  pull-requests: write
```

## Usage

Basic:

<!-- TODO update example repo name changes -->
```yaml
on: pull_request

jobs:
  find_flags:
    runs-on: ubuntu-latest
    name: Find LaunchDarkly feature flags in PR
    steps:
      - name: Checkout
        uses: actions/checkout@v3
      - name: Find flags
        uses: launchdarkly/cr-flags@v0.6.0
        id: find_flags
        with:
          project-key: default
          environmet-key: production
          access-token: ${{ secrets.LD_ACCESS_TOKEN }}
          repo-token: ${{ secrets.GITHUB_TOKEN }}
```

Use outputs in workflow:

<!-- TODO update example repo name changes -->
```yaml
on: pull_request

jobs:
  find_flags:
    runs-on: ubuntu-latest
    name: Find LaunchDarkly feature flags in PR
    steps:
      - name: Checkout
        uses: actions/checkout@v3
      - name: Find flags
        uses: launchdarkly/cr-flags@v0.6.0
        id: find_flags
        with:
          project-key: default
          environmet-key: production
          access-token: ${{ secrets.LD_ACCESS_TOKEN }}
          repo-token: ${{ secrets.GITHUB_TOKEN }}

      # Add or remove labels on PRs if any flags have changed
      - name: Add label
        if: steps.find_flags.outputs.any-modified == 'true' || steps.find_flags.outputs.any-removed == 'true'
        run: gh pr edit $PR_NUMBER --add-label ld-flags
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
      - name: Remove label
        if: steps.find_flags.outputs.any-modified == 'false' && steps.find_flags.outputs.any-removed == 'false'
        run: gh pr edit $PR_NUMBER --remove-label ld-flags
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
```

### Flag aliases

This actions has full support for code reference aliases. If the project has an existing `.launchdarkly/coderefs.yaml` file, it will use the aliases defined there.

More information on aliases can be found at [launchdarkly/ld-find-code-refs](https://github.com/launchdarkly/ld-find-code-refs/blob/main/docs/ALIASES.md).

<!-- action-docs-inputs -->
### Inputs

| parameter | description | required | default |
| --- | --- | --- | --- |
| repo-token | Token to use to authorize comments on PR. Typically the GITHUB_TOKEN secret. | `true` |  |
| access-token | LaunchDarkly access token | `true` |  |
| project-key | LaunchDarkly Project | `false` | default |
| environment-key | LaunchDarkly environment for creating flag links | `false` | production |
| placeholder-comment | Comment when no flags are found. If flags are found in later commits, this comment will be updated. | `false` | false |
| include-archived-flags | Scan for archived flags | `false` | true |
| max-flags | Maximum number of flags to find per PR | `false` | 5 |
| base-uri | The base URI for the LaunchDarkly server. Most users should use the default value. | `false` | https://app.launchdarkly.com |
<!-- action-docs-inputs -->

<!-- action-docs-outputs -->
### Outputs

| parameter | description |
| --- | --- |
| any-modified | Returns true if any flags have been added or modified in PR |
| modified-flags | Space-separated list of flags added or modified in PR |
| modified-flags-count | Number of flags added or modified in PR |
| any-removed | Returns true if any flags have been removed in PR |
| removed-flags | Space-separated list of flags removed in PR |
| removed-flags-count | Number of flags removed in PR |
<!-- action-docs-outputs -->
