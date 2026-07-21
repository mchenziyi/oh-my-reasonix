package promptcompose

import "testing"

func TestCanonicalizeAndCompose(t *testing.T) {
	composition := Compose("\ufeffbase\r\n\r\n", "  user\r\n", "omr\n\n")
	if composition.Content != "base\n\n  user\n\nomr\n" {
		t.Fatalf("unexpected composition: %q", composition.Content)
	}
	if len(composition.Segments) != 3 || composition.Segments[1].ID != "user" {
		t.Fatalf("unexpected segments: %#v", composition.Segments)
	}
	withoutUser := Compose("base", "\n", "omr")
	if withoutUser.Content != "base\n\nomr\n" || len(withoutUser.Segments) != 2 {
		t.Fatalf("empty User segment was not omitted: %#v", withoutUser)
	}
}
