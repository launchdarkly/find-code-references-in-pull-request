# Code References PR Commenter

Add this action for Pull Requests to receive a comment whenever a LaunchDarkly Feature Flag is referenced in any of the code changes.

<img src="https://github.com/launchdarkly/cr-flags/raw/master/images/example-comment.png?raw=true" alt="Example comment" width="100%">

## Configuration
PR Commenter has full support for Code Reference Aliases. If the project has an existing `.launchdarkly/coderefs.yaml` file it will use the aliases defined there.

```
on: [pull_request]

jobs:
  find_flags:
    runs-on: ubuntu-latest
    name: Test Find Flags
    steps:
      - name: Checkout
        uses: actions/checkout@v2
      - name: Find Flags
        uses: ./ # Uses an action in the root directory
        id: find_flags
        with:
          projKey: default
          envKey: production
          accessToken: ${{ secrets.LD_ACCESS_TOKEN }}
          githubToken: ${{ secrets.GITHUB_TOKEN }}
```
