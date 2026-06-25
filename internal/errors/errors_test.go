package errors_test

import (
	stderrors "errors"
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
