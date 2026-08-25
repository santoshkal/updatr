package tests

import (
	_ "go.uber.org/goleak"
)

// This package contains unit tests for the updatr controller.
//
// Run:
//
//	go test ./tests -run Test -count=1 -race -cover
//	go test ./... -race -cover
