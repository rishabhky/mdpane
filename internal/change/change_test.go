package change

import "testing"

func TestNoChange(t *testing.T) {
	s := "a\nb\nc"
	if got := Lines(s, s); len(got) != 0 {
		t.Fatalf("identical content reported %d changed lines", len(got))
	}
}

func TestAppendMarksTail(t *testing.T) {
	old := "a\nb\n"
	niu := "a\nb\nc\nd\n"
	got := Lines(old, niu)
	if _, ok := got[2]; !ok {
		t.Fatalf("appended line 2 not marked: %v", got)
	}
	if _, ok := got[3]; !ok {
		t.Fatalf("appended line 3 not marked: %v", got)
	}
	if _, ok := got[0]; ok {
		t.Fatalf("unchanged line 0 wrongly marked")
	}
	if !IsAppend(old, niu) {
		t.Fatal("IsAppend should be true for a pure append")
	}
}

func TestMidEditMarksOnlyRegion(t *testing.T) {
	old := "one\ntwo\nthree\nfour\nfive"
	niu := "one\ntwo\nTHREE!\nfour\nfive"
	got := Lines(old, niu)
	if _, ok := got[2]; !ok {
		t.Fatalf("edited line 2 not marked: %v", got)
	}
	for _, i := range []int{0, 1, 3, 4} {
		if _, ok := got[i]; ok {
			t.Fatalf("unchanged line %d wrongly marked (%v)", i, got)
		}
	}
	if IsAppend(old, niu) {
		t.Fatal("mid edit must not count as append")
	}
}

func TestDeletionMarksSurvivor(t *testing.T) {
	old := "one\ntwo\nthree\nfour"
	niu := "one\nthree\nfour"
	got := Lines(old, niu)
	if len(got) == 0 {
		t.Fatal("deletion produced no marks at all")
	}
	// The survivor at the deletion point (line 1, "three") should be marked.
	if _, ok := got[1]; !ok {
		t.Fatalf("survivor line not marked: %v", got)
	}
}

func TestAnsiIsIgnored(t *testing.T) {
	old := "\x1b[1mplain\x1b[0m\ntext"
	niu := "plain\ntext"
	if got := Lines(old, niu); len(got) != 0 {
		t.Fatalf("styling-only difference reported as change: %v", got)
	}
}

func TestInsertionShiftsWithoutFalsePositives(t *testing.T) {
	old := "a\nb\nc\nd\ne"
	niu := "a\nNEW\nb\nc\nd\ne"
	got := Lines(old, niu)
	if _, ok := got[1]; !ok {
		t.Fatalf("inserted line not marked: %v", got)
	}
	if len(got) > 2 {
		t.Fatalf("insertion marked too many lines (shift not handled): %v", got)
	}
}
