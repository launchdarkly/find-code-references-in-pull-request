# LaunchDarkly Find Code References in Pull Request GitHub action

Adds a comment to a pull request (PR) whenever a feature flag reference is found in a PR diff and creates a [flag link](https://docs.launchdarkly.com/home/organize/links) in LaunchDarkly.

<img src="https://github.com/launchdarkly/find-code-references-in-pull-request/raw/main/images/example-comment.png?raw=true" alt="An example code references PR comment" width="100%">

<img src="https://github.com/launchdarkly/find-code-references-in-pull-request/raw/main/images/example-flag-link.png?raw=true" alt="An example GitHub pull request flag link" width="100%">

## Permissions

This action requires a [LaunchDarkly access token](https://docs.launchdarkly.com/home/account-security/api-access-tokens) with:

* Read access for the designated `project-key`
* (Optional) the `createFlagLink` action, when [`create-flag-links` input is `true` (default behavior)](#inputs)

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
        uses: launchdarkly/find-code-references-in-pull-request@v2
        id: find-flags
        with:
          project-key: default
          environment-key: production
          access-token: ${{ secrets.LD_ACCESS_TOKEN }}
          repo-token: ${{ secrets.GITHUB_TOKEN }}
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
        uses: launchdarkly/find-code-references-in-pull-request@v2
        id: find-flags
        with:
          project-key: default
          environment-key: production
          access-token: ${{ secrets.LD_ACCESS_TOKEN }}
          repo-token: ${{ secrets.GITHUB_TOKEN }}

      # Add or remove labels on PRs if any flags have changed.
      # Note: This example requires the "ld-flags" label to have been created in the repository first:
      # https://docs.github.com/en/issues/using-labels-and-milestones-to-track-work/managing-labels#creating-a-label
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

## Using a private or mirrored container registry

The root Action (`launchdarkly/find-code-references-in-pull-request@v2`) is a Docker container action. GitHub builds its `Dockerfile` on each run (which pulls base images from Docker Hub), and that image reference cannot be overridden with an input.

If your organization must pull images through an internal registry or Docker Hub proxy, use the optional **`docker`** entry point. It runs a **prebuilt** runtime image via `docker run` and accepts a `dockerImage` input. Mirror `launchdarkly/find-code-references-in-pull-request` into your registry first (pin the tag you mirrored).

```yaml
on: pull_request

jobs:
  find-flags:
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write
      contents: read
    steps:
      - uses: actions/checkout@v4
      - uses: docker/login-action@v3
        with:
          registry: your.registry.example
          username: ${{ secrets.REGISTRY_USER }}
          password: ${{ secrets.REGISTRY_TOKEN }}
      - name: Find flags
        id: find-flags
        # Pin to a release that includes the docker/ entry point and published image (see changelog).
        uses: launchdarkly/find-code-references-in-pull-request/docker@v2.3.0
        with:
          project-key: default
          environment-key: production
          access-token: ${{ secrets.LD_ACCESS_TOKEN }}
          repo-token: ${{ secrets.GITHUB_TOKEN }}
          dockerImage: your.registry.example/launchdarkly/find-code-references-in-pull-request:2.3.0
```

This entry point requires a Docker CLI on the runner (included on GitHub-hosted `ubuntu-*` runners). Existing workflows that use the root Action do not need to change.

### Flag aliases

This action has full support for code reference aliases. If the project has an existing [`.launchdarkly/coderefs.yaml`](https://github.com/launchdarkly/ld-find-code-refs/blob/main/docs/CONFIGURATION.md#yaml) file, it will use the aliases defined there.

You can find more information on aliases at [launchdarkly/ld-find-code-refs](https://github.com/launchdarkly/ld-find-code-refs/blob/main/docs/ALIASES.md).

### Monorepos

This action does not support monorepos or searching for flags across LaunchDarkly projects.

<!-- action-docs-inputs source="action.yml" -->
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
| `create-flag-links` | <p>Create links to flags in LaunchDarkly. To use this feature you must use an access token with the <code>createFlagLink</code> role. To learn more, read <a href="https://docs.launchdarkly.com/home/organize/links">Flag links</a>.</p> | `false` | `true` |
| `skip-comment` | <p>Skip commenting on PR. If set to true, the action will not comment on the PR, but will still set outputs.</p> | `false` | `false` |
<!-- action-docs-inputs source="action.yml" -->

<!-- action-docs-outputs source="action.yml" -->
### Outputs

| name | description |
| --- | --- |
| `any-modified` | <p>Returns true if any flags have been added or modified in PR</p> |
| `modified-flags` | <p>Space-separated list of flags added or modified in PR</p> |
| `modified-flags-count` | <p>Number of flags added or modified in PR</p> |
| `any-removed` | <p>Returns true if any flags have been removed in PR</p> |
| `removed-flags` | <p>Space-separated list of flags removed in PR</p> |
| `removed-flags-count` | <p>Number of flags removed in PR</p> |
| `any-changed` | <p>Returns true if any flags have been changed in PR</p> |
| `changed-flags` | <p>Space-separated list of flags changed in PR</p> |
| `changed-flags-count` | <p>Number of flags changed in PR</p> |
| `any-extinct` | <p>Returns true if any flags have been removed in PR and no longer exist in codebase. Only returned if <code>check-extinctions</code> is true.</p> |
| `extinct-flags` | <p>Space-separated list of flags removed in PR and no longer exist in codebase. Only returned if <code>check-extinctions</code> is true.</p> |
| `extinct-flags-count` | <p>Number of flags removed in PR and no longer exist in codebase. Only returned if <code>check-extinctions</code> is true.</p> |
<!-- action-docs-outputs source="action.yml" -->
