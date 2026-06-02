package checksum

import (
	"strings"
	"testing"
)

func TestSHA256(t *testing.T) {
	sum, size, err := SHA256(strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("sha256: %v", err)
	}
	want := "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if sum != want {
		t.Errorf("sum = %q, want %q", sum, want)
	}
	if size != 5 {
		t.Errorf("size = %d, want 5", size)
	}
}
