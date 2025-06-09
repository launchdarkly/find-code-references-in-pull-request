package errors

import "errors"

var (
	UnauthorizedError = errors.New("`repo-token` lacks required permissions")
	NoGitError        = errors.New("`git` not installed")
)
