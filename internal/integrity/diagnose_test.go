package integrity

import (
	"testing"

	"github.com/p3bot/pj/internal/scopeconfig"
)

func TestFieldTypeError(t *testing.T) {
	strs := scopeconfig.Field{Type: scopeconfig.FieldStrings}
	enum := scopeconfig.Field{Type: scopeconfig.FieldStrings, Values: []string{"api", "ui"}}
	cases := []struct {
		name  string
		field scopeconfig.Field
		value any
		want  string
	}{
		{"strings all strings", strs, []any{"a", "b"}, ""},
		{"strings empty", strs, []any{}, ""},
		{"strings scalar", strs, "api", "should be a list of strings"},
		{"strings int element", strs, []any{"api", 7}, "has a non-string entry (7)"},
		{"strings bool element", strs, []any{true}, "has a non-string entry (true)"},
		{"strings nested list", strs, []any{[]any{"a"}}, "has a non-string entry ([a])"},
		{"enum int element", enum, []any{7}, "has a non-string entry (7)"},
		{"enum outside values", enum, []any{"api", "db"}, `has value "db" outside its declared values`},
		{"enum within values", enum, []any{"api", "ui"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fieldTypeError(tc.field, tc.value); got != tc.want {
				t.Errorf("fieldTypeError = %q, want %q", got, tc.want)
			}
		})
	}
}
