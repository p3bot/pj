package skill_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/p3bot/agentdex"

	"github.com/p3bot/tk/internal/skill"
)

func TestMapCatalogErrorUnavailableAndInvalid(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want string
	}{
		{"unavailable", agentdex.ErrCatalogUnavailable, "catalog unavailable"},
		{"invalid", agentdex.ErrCatalogInvalid, "catalog invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := skill.MapCatalogError(tc.in)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("msg = %q want substring %q", err.Error(), tc.want)
			}
			if !strings.Contains(err.Error(), "tk skill") {
				t.Fatalf("msg must guide manual install via tk skill: %v", err)
			}
			if !errors.Is(err, tc.in) {
				t.Fatalf("errors.Is lost sentinel: %v", err)
			}
		})
	}
	if skill.MapCatalogError(nil) != nil {
		t.Fatal("nil in → nil out")
	}
	passthrough := errors.New("other")
	got := skill.MapCatalogError(passthrough)
	if !errors.Is(got, passthrough) {
		t.Fatal("non-catalog errors pass through")
	}
	// Identity: must return the same value, not a wrap around it.
	var unwrap interface{ Unwrap() error }
	if errors.As(got, &unwrap) {
		t.Fatal("non-catalog errors must not be wrapped")
	}
}

func TestUsageSentinelsWrap(t *testing.T) {
	err := fmt.Errorf("%w %q", skill.ErrUnknownAgent, "x")
	if !errors.Is(err, skill.ErrUnknownAgent) {
		t.Fatal("ErrUnknownAgent wrap")
	}
	err = skill.NoWritablePathError("alpha")
	if !errors.Is(err, skill.ErrNoWritablePath) {
		t.Fatal("ErrNoWritablePath wrap")
	}
}
