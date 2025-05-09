package common

import "errors"

var (
	NodeAlreadyExistsErr = errors.New("node already exists")
	NodeNotFoundErr      = errors.New("node not found")
)
