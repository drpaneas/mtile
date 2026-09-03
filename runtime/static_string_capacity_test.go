//go:build mtile_static_strings_1

package runtime

import "testing"

func TestStaticStringTierFailsClosedAtSelectedCapacity(t *testing.T) {
	if staticStringSlots != 1 || len((Context{}).staticTab) != 1 {
		t.Fatalf("static string capacity = %d, want 1", staticStringSlots)
	}
	var c Context
	if got := c.InternStatic("one"); got.Kind != KindStaticString {
		t.Fatalf("first static string kind = %d", got.Kind)
	}
	if got := c.InternStatic("two"); got.Kind != KindNil {
		t.Fatalf("over-capacity static string kind = %d, want nil", got.Kind)
	}
}
