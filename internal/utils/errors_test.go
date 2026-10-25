package utils

import (
	"errors"
	"testing"
)

func TestComposeErrors(t *testing.T) {
	err1 := errors.New("first")
	err2 := errors.New("second")

	if err := ComposeErrors(); err != nil {
		t.Errorf("no errors: got %v, want nil", err)
	}
	if err := ComposeErrors(nil, nil); err != nil {
		t.Errorf("all nil: got %v, want nil", err)
	}
	if err := ComposeErrors(nil, err1, nil); err != err1 {
		t.Errorf("single error: got %v, want %v", err, err1)
	}
	if err := ComposeErrors(err1, err2); err == nil || err.Error() != "first: second" {
		t.Errorf("two errors: got %v, want %q", err, "first: second")
	}
}

func TestPeekErr(t *testing.T) {
	errChan := make(chan error, 1)
	if err := PeekErr(errChan); err != nil {
		t.Errorf("empty channel: got %v, want nil", err)
	}

	want := errors.New("queued")
	errChan <- want
	if err := PeekErr(errChan); err != want {
		t.Errorf("got %v, want %v", err, want)
	}
	// The error is consumed.
	if err := PeekErr(errChan); err != nil {
		t.Errorf("drained channel: got %v, want nil", err)
	}
}

func TestContainsError(t *testing.T) {
	base := errors.New("couldn't connect: connection refused")
	target := errors.New("connection refused")

	if !ContainsError(base, target) {
		t.Error("expected base to contain target")
	}
	if ContainsError(target, base) {
		t.Error("expected target not to contain base")
	}
	if ContainsError(nil, target) || ContainsError(base, nil) || ContainsError(nil, nil) {
		t.Error("nil errors must not match")
	}
}

func TestAddContext(t *testing.T) {
	if err := AddContext(nil, "ctx"); err != nil {
		t.Errorf("nil error: got %v, want nil", err)
	}
	err := AddContext(errors.New("boom"), "doing the thing")
	if err == nil || err.Error() != "doing the thing: boom" {
		t.Errorf("got %v, want %q", err, "doing the thing: boom")
	}
}
