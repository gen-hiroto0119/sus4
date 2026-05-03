package diffview

import "testing"

func TestParseClassifiesUnifiedDiff(t *testing.T) {
	in := `diff --git a/foo.txt b/foo.txt
index 0000001..0000002 100644
--- a/foo.txt
+++ b/foo.txt
@@ -1,2 +1,3 @@
 keep
-old
+new
+added
\ No newline at end of file
`
	got := Parse(in)
	want := []LineKind{
		LineFileHeader, // diff --git
		LineMeta,       // index
		LineMeta,       // ---
		LineMeta,       // +++
		LineHunk,       // @@
		LineContext,    // " keep"
		LineDel,        // -old
		LineAdd,        // +new
		LineAdd,        // +added
		LineMeta,       // \ No newline
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Kind != w {
			t.Errorf("line %d kind=%v, want %v (text=%q)", i, got[i].Kind, w, got[i].Text)
		}
	}

	// Spot-check the line-number plumbing on the body rows.
	type lnCheck struct {
		idx          int
		old, new int
	}
	for _, c := range []lnCheck{
		{5, 1, 1}, // " keep"
		{6, 2, 0}, // "-old"
		{7, 0, 2}, // "+new"
		{8, 0, 3}, // "+added"
	} {
		if got[c.idx].OldLine != c.old || got[c.idx].NewLine != c.new {
			t.Errorf("line %d (kind=%v): old=%d new=%d, want old=%d new=%d",
				c.idx, got[c.idx].Kind, got[c.idx].OldLine, got[c.idx].NewLine, c.old, c.new)
		}
	}
}

func TestParseHandlesBinaryDiff(t *testing.T) {
	in := "diff --git a/x.bin b/x.bin\nBinary files a/x.bin and b/x.bin differ\n"
	got := Parse(in)
	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2: %+v", len(got), got)
	}
	if got[1].Kind != LineBinary {
		t.Errorf("expected LineBinary, got %v", got[1].Kind)
	}
}

func TestParseEmptyInput(t *testing.T) {
	if got := Parse(""); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}
