package core

import (
	"context"
	"testing"
)

// StubGetEmm replaces the download Apply makes when emm is missing.
func StubGetEmm(t *testing.T, stub func() error) {
	t.Helper()
	original := getEmm
	getEmm = func(_ context.Context, _ GetEmmRequest) (GetEmmResult, error) { return GetEmmResult{}, stub() }
	t.Cleanup(func() { getEmm = original })
}
