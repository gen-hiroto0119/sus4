package highlight

import (
	"strings"
	"testing"
)

func TestHighlightEmitsAnsiForRecognizedSource(t *testing.T) {
	r := Highlight("main.go", []byte("package main\n"), false, true)
	if r.Plain {
		t.Fatalf("plain=true unexpectedly: %+v", r)
	}
	if !strings.Contains(r.Text, "\x1b[") {
		t.Errorf("expected ANSI escapes, got %q", r.Text)
	}
}

func TestHighlightDetectsBinary(t *testing.T) {
	content := []byte{'h', 'i', 0, 'x'}
	r := Highlight("blob", content, false, true)
	if !r.Binary || !r.Plain || r.Text != "Binary file" {
		t.Errorf("binary detection failed: %+v", r)
	}
}

func TestHighlightSkipsLargeFile(t *testing.T) {
	big := make([]byte, MaxBytes+1)
	for i := range big {
		big[i] = 'x'
	}
	r := Highlight("big.txt", big, false, true)
	if !r.Plain {
		t.Errorf("expected plain=true for large file: %+v", r)
	}
	if r.Reason == "" {
		t.Errorf("expected a Reason explaining skip")
	}
}

func TestHighlightFallbackForUnknownExt(t *testing.T) {
	// Should not panic and should still produce text.
	r := Highlight("file.unknown-ext-xyz", []byte("hello world\n"), false, true)
	if r.Text == "" {
		t.Errorf("expected non-empty output")
	}
}
