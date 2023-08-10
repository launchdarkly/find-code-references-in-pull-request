# LaunchDarkly Find Code References in Pull Request GitHub Action

Adds a comment to a pull request (PR) whenever a feature flag reference is found in a PR diff.

<!-- TODO update this link when repo name changes -->
<img src="https://github.com/launchdarkly/cr-flags/raw/main/images/example-comment.png?raw=true" alt="An example code references PR comment" width="100%">

## Permissions

This action requires a [LaunchDarkly access token](https://docs.launchdarkly.com/home/account-security/api-access-tokens) with read access for the designated `project-key`. Access tokens should be stored as an [encrypted secret](https://docs.github.com/en/actions/security-guides/encrypted-secrets).

To add a comment to a PR, the `repo-token` used requires `write` permission for PRs. You can also specify permissions for the workflow with:

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
  find-flags:
    runs-on: ubuntu-latest
    name: Find LaunchDarkly feature flags in diff
    steps:
      - name: Checkout
        uses: actions/checkout@v3
      - name: Find flags
        uses: launchdarkly/cr-flags@v0.6.0
        id: find-flags
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
  find-feature-flags:
    runs-on: ubuntu-latest
    name: Find LaunchDarkly feature flags in diff
    steps:
      - name: Checkout
        uses: actions/checkout@v3
      - name: Find flags
        uses: launchdarkly/cr-flags@v0.6.0
        id: find-flags
        with:
          project-key: default
          environmet-key: production
          access-token: ${{ secrets.LD_ACCESS_TOKEN }}
          repo-token: ${{ secrets.GITHUB_TOKEN }}

      # Add or remove labels on PRs if any flags have changed
      - name: Add label
        if: steps.find-flags.outputs.any-modified == 'true' || steps.find-flags.outputs.any-removed == 'true'
        run: gh pr edit $PR_NUMBER --add-label ld-flags
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
      - name: Remove label
        if: steps.find-flags.outputs.any-modified == 'false' && steps.find-flags.outputs.any-removed == 'false'
        run: gh pr edit $PR_NUMBER --remove-label ld-flags
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
```

### Flag aliases

This action has full support for code reference aliases. If the project has an existing [`.launchdarkly/coderefs.yaml`](https://github.com/launchdarkly/ld-find-code-refs/blob/main/docs/CONFIGURATION.md#yaml) file, it will use the aliases defined there.

More information on aliases can be found at [launchdarkly/ld-find-code-refs](https://github.com/launchdarkly/ld-find-code-refs/blob/main/docs/ALIASES.md).

### Monorepos

This action does not support monorepos or searching for flags across LaunchDarkly projects.

<!-- action-docs-inputs -->
### Inputs

| parameter | description | required | default |
| --- | --- | --- | --- |
| repo-token | Token to use to authorize comments on PR. Typically the `GITHUB_TOKEN` secret or equivalent `github.token`. | `true` |  |
| access-token | LaunchDarkly access token | `true` |  |
| project-key | LaunchDarkly project key | `false` | default |
| environment-key | LaunchDarkly environment key for creating flag links | `false` | production |
| placeholder-comment | Comment on PR when no flags are found. If flags are found in later commits, this comment will be updated. | `false` | false |
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
