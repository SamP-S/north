package errors_test

import (
	stderrors "errors"
	"fmt"
	"testing"

	"github.com/SamP-S/north/internal/errors"
)

func TestBoardErrorsCarryCodeAndMessage(t *testing.T) {
	cases := []struct {
		err  errors.BoardError
		code string
	}{
		{errors.NotFound("nope"), "not_found"},
		{errors.Conflict("clash"), "conflict"},
		{errors.Invalid("bad"), "invalid"},
	}
	for _, c := range cases {
		if c.err.Code() != c.code {
			t.Errorf("code = %q, want %q", c.err.Code(), c.code)
		}
		if c.err.Error() != c.err.Message() {
			t.Errorf("Error() %q != Message() %q", c.err.Error(), c.err.Message())
		}
	}
}

func TestAsDetectsBoardError(t *testing.T) {
	if be, ok := errors.As(errors.Invalid("x")); !ok || be.Code() != "invalid" {
		t.Errorf("As failed to detect BoardError")
	}
	if _, ok := errors.As(stderrors.New("plain")); ok {
		t.Errorf("As should reject a plain error")
	}
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{nil, 0},
		{stderrors.New("boom"), 1},
		{errors.Invalid("bad"), 2},
		{errors.NotFound("gone"), 3},
		{errors.Conflict("clash"), 4},
	}
	for _, c := range cases {
		if got := errors.ExitCode(c.err); got != c.want {
			t.Errorf("ExitCode(%v) = %d, want %d", c.err, got, c.want)
		}
	}
}

func TestAsUnwrapsWrappedBoardErrors(t *testing.T) {
	wrapped := fmt.Errorf("outer context: %w", errors.Conflict("clash"))
	be, ok := errors.As(wrapped)
	if !ok || be.Code() != "conflict" {
		t.Fatalf("As should see through %%w wrapping: %v %v", be, ok)
	}
	if got := errors.ExitCode(wrapped); got != 4 {
		t.Errorf("wrapped conflict should exit 4, got %d", got)
	}
}
