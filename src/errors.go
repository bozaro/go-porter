package src

import "github.com/joomcode/errorx"

var (
	Errors = errorx.NewNamespace("docker-build-lte")

	ErrLocalFilesystem = Errors.NewType("invalid_response")
)
