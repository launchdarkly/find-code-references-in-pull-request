# Code References PR Commenter

Add this action for Pull Requests to receive a comment whenever a LaunchDarkly Feature Flag is referenced in any of the code changes.

<img src="https://github.com/launchdarkly/cr-flags/raw/main/images/example-comment.png?raw=true" alt="Example comment" width="100%">

## Configuration
PR Commenter has full support for Code Reference Aliases. If the project has an existing `.launchdarkly/coderefs.yaml` file it will use the aliases defined there.

```
on: [pull_request]

jobs:
  find_flags:
    runs-on: ubuntu-latest
    name: Find LaunchDarkly Feature Flags
    steps:
      - name: Checkout
        uses: actions/checkout@v3
      - name: Find Flags
        uses: launchdarkly/cr-flags@v0.6.0
        id: find_flags
        with:
          project-key: default
          environmet-key: production
          access-token: ${{ secrets.LD_ACCESS_TOKEN }}
          repo-token: ${{ secrets.GITHUB_TOKEN }}
```

<!-- action-docs-inputs -->
### Inputs

| parameter | description | required | default |
| --- | --- | --- | --- |
| project-key | LaunchDarkly Project | `false` | default |
| environment-key | LaunchDarkly environment for setting flag links. Can be a comma-separated list of environment keys. | `false` | production |
| base-uri | The base URI for the LaunchDarkly server. Most users should use the default value. | `false` | https://app.launchdarkly.com |
| repo-token | Token to use to authorize comments on PR. Typically the GITHUB_TOKEN secret. | `true` |  |
| access-token | LaunchDarkly access token | `true` |  |
| placeholder-comment | Comment even if no flags are found. If flags are found in later commits this comment will be updated. | `false` | false |
| max-flags | Maximum number of flags to find per PR | `false` | 5 |
<!-- action-docs-inputs -->

<!-- action-docs-outputs -->
### Outputs

| parameter | description |
| --- | --- |
| any-modified | Returns true if any flags have been added or modified in pull request |
| modified-flags | Space-separated list of flags added or modified in pull request |
| modified-flags-count | Number of flags added or modified in pull request |
| any-removed | Returns true if any flags have been removed in pull request |
| removed-flags | Space-separated list of flags removed in pull request |
| removed-flags-count | Number of flags removed in pull request |
<!-- action-docs-outputs -->
