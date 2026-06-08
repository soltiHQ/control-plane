package base

import "errors"

// ErrAlreadyStarted indicates Start was called more than once.
var ErrAlreadyStarted = errors.New("base: already started")
