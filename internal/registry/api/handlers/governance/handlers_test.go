package governance

import "testing"

func TestSafeCSV(t *testing.T) {
	for _, v := range []string{"=HYPERLINK(\"https://example.com\")", " +1", "-cmd", "@SUM(A1)", "\tformula", "\rformula"} {
		if safeCSV(v) != "'"+v {
			t.Fatalf("unsafe CSV %q", v)
		}
	}
	if safeCSV("normal") != "normal" {
		t.Fatal("normal CSV changed")
	}
}
