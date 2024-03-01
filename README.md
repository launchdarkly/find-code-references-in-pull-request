# LaunchDarkly Find Code References in Pull Request GitHub action

Adds a comment to a pull request (PR) whenever a feature flag reference is found in a PR diff.

<!-- TODO update this link when repo name changes -->
<img src="https://github.com/launchdarkly/find-code-references-in-pull-request/raw/main/images/example-comment.png?raw=true" alt="An example code references PR comment" width="100%">

## Permissions

This action requires a [LaunchDarkly access token](https://docs.launchdarkly.com/home/account-security/api-access-tokens) with:

* Read access for the designated `project-key`
* (Optional) the `createFlagLink` action, if you have set the `create-flag-links` input to `true`

Access tokens should be stored as an [encrypted secret](https://docs.github.com/en/actions/security-guides/encrypted-secrets).

To add a comment to a PR, the `repo-token` used requires `write` permission for PRs. You can also specify permissions for the workflow with:

```yaml
permissions:
  pull-requests: write
```

## Usage

Basic:

```yaml
on: pull_request

jobs:
  find-flags:
    runs-on: ubuntu-latest
    name: Find LaunchDarkly feature flags in diff
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Find flags
        uses: launchdarkly/find-code-references-in-pull-request@v1
        id: find-flags
        with:
          project-key: default
          environment-key: production
          access-token: ${{ secrets.LD_ACCESS_TOKEN }}
          repo-token: ${{ secrets.GITHUB_TOKEN }}
          create-flag-links: true
```

Use outputs in workflow:

```yaml
on: pull_request

jobs:
  find-feature-flags:
    runs-on: ubuntu-latest
    name: Find LaunchDarkly feature flags in diff
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Find flags
        uses: launchdarkly/find-code-references-in-pull-request@v1
        id: find-flags
        with:
          project-key: default
          environment-key: production
          access-token: ${{ secrets.LD_ACCESS_TOKEN }}
          repo-token: ${{ secrets.GITHUB_TOKEN }}

      # Add or remove labels on PRs if any flags have changed
      - name: Add label
        if: steps.find-flags.outputs.any-changed == 'true'
        run: gh pr edit $PR_NUMBER --add-label ld-flags
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
      - name: Remove label
        if: steps.find-flags.outputs.any-changed == 'false'
        run: gh pr edit $PR_NUMBER --remove-label ld-flags
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          PR_NUMBER: ${{ github.event.pull_request.number }}
```

### Flag aliases

This action has full support for code reference aliases. If the project has an existing [`.launchdarkly/coderefs.yaml`](https://github.com/launchdarkly/ld-find-code-refs/blob/main/docs/CONFIGURATION.md#yaml) file, it will use the aliases defined there.

You can find more information on aliases at [launchdarkly/ld-find-code-refs](https://github.com/launchdarkly/ld-find-code-refs/blob/main/docs/ALIASES.md).

### Monorepos

This action does not support monorepos or searching for flags across LaunchDarkly projects.

<!-- action-docs-inputs action="action.yml" -->
### Inputs

| name | description | required | default |
| --- | --- | --- | --- |
| `repo-token` | <p>Token to use to authorize comments on PR. Typically the <code>GITHUB_TOKEN</code> secret or equivalent <code>github.token</code>.</p> | `true` | `""` |
| `access-token` | <p>LaunchDarkly access token</p> | `true` | `""` |
| `project-key` | <p>LaunchDarkly project key</p> | `false` | `default` |
| `environment-key` | <p>LaunchDarkly environment key for creating flag links</p> | `false` | `production` |
| `placeholder-comment` | <p>Comment on PR when no flags are found. If flags are found in later commits, this comment will be updated.</p> | `false` | `false` |
| `include-archived-flags` | <p>Scan for archived flags</p> | `false` | `true` |
| `max-flags` | <p>Maximum number of flags to find per PR</p> | `false` | `5` |
| `base-uri` | <p>The base URI for the LaunchDarkly server. Most members should use the default value.</p> | `false` | `https://app.launchdarkly.com` |
| `check-extinctions` | <p>Check if removed flags still exist in codebase</p> | `false` | `true` |
| `create-flag-links` | <p>Create links to flags in LaunchDarkly. To use this feature you must use an access token with the <code>createFlagLink</code> role. To learn more, read <a href="https://docs.launchdarkly.com/home/organize/links">Flag links</a>.</p> | `false` | `false` |
<!-- action-docs-inputs action="action.yml" -->
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
| check-extinctions | Check if removed flags still exist in codebase | `false` | true |
<!-- action-docs-inputs -->

<!-- action-docs-outputs action="action.yml"-->
### Outputs

| parameter | description |
| --- | --- |
| any-modified | Returns true if any flags have been added or modified in PR |
| modified-flags | Space-separated list of flags added or modified in PR |
| modified-flags-count | Number of flags added or modified in PR |
| any-removed | Returns true if any flags have been removed in PR |
| removed-flags | Space-separated list of flags removed in PR |
| removed-flags-count | Number of flags removed in PR |
| any-changed | Returns true if any flags have been changed in PR |
| changed-flags | Space-separated list of flags changed in PR |
| changed-flags-count | Number of flags changed in PR |
| any-extinct | Returns true if any flags have been removed in PR and no longer exist in codebase. Only returned if `check-extinctions` is true. |
| extinct-flags | Space-separated list of flags removed in PR and no longer exist in codebase. Only returned if `check-extinctions` is true. |
| extinct-flags-count | Number of flags removed in PR and no longer exist in codebase. Only returned if `check-extinctions` is true. |
<!-- action-docs-outputs -->
