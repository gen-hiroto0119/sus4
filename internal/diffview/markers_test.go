package diffview

import (
	"reflect"
	"testing"
)

func TestMarkersClassifiesEdits(t *testing.T) {
	// File of 6 new lines. Edits:
	//   - line 2: pure addition
	//   - line 4: modification (one old line replaced by one new)
	//   - between line 5 and line 6: pure deletion
	in := `diff --git a/foo b/foo
index 0..1 100644
--- a/foo
+++ b/foo
@@ -1,5 +1,6 @@
 keep1
+addedA
 keep2
-oldB
+modB
 keep3
-droppedC
 keep4
`
	parsed := Parse(in)
	got := Markers(parsed)
	want := map[int]ChangeKind{
		2: ChangeAdd, // +addedA
		4: ChangeMod, // -oldB / +modB
		6: ChangeDel, // surviving line below the lone -droppedC
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestMarkersTrailingAddRun(t *testing.T) {
	// Adds at end-of-file have no trailing context line — the flush has
	// to handle the boundary at end-of-input.
	in := `diff --git a/foo b/foo
index 0..1 100644
--- a/foo
+++ b/foo
@@ -1,1 +1,3 @@
 keep
+tail1
+tail2
`
	got := Markers(Parse(in))
	want := map[int]ChangeKind{
		2: ChangeAdd,
		3: ChangeAdd,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestMarkersTrailingModRun(t *testing.T) {
	// A - run followed by a + run with no closing context. The flush at
	// end-of-input should still classify it as Mod, not Add.
	in := `diff --git a/foo b/foo
index 0..1 100644
--- a/foo
+++ b/foo
@@ -1,2 +1,2 @@
 keep
-old
+new
`
	got := Markers(Parse(in))
	want := map[int]ChangeKind{2: ChangeMod}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestMarkersEmptyDiff(t *testing.T) {
	got := Markers(Parse(""))
	if len(got) != 0 {
		t.Errorf("expected empty markers, got %+v", got)
	}
}

func TestMarkersIgnoresBinary(t *testing.T) {
	in := "diff --git a/x.bin b/x.bin\nBinary files a/x.bin and b/x.bin differ\n"
	got := Markers(Parse(in))
	if len(got) != 0 {
		t.Errorf("expected no markers for binary diff, got %+v", got)
	}
}
