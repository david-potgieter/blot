package size

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"500MB", 500_000_000, true},
		{"1GiB", 1 << 30, true},
		{"  2 gb ", 2_000_000_000, true},
		{"1024", 1024, true},
		{"", 0, false},
		{"abc", 0, false},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("Parse(%q) = %d, %v; want %d", c.in, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("Parse(%q) expected error", c.in)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := map[int64]string{0: "0 B", 1_500_000: "1.5 MB", 2_000_000_000: "2.0 GB", -5: "0 B"}
	for in, want := range cases {
		if got := Format(in); got != want {
			t.Errorf("Format(%d) = %q; want %q", in, got, want)
		}
	}
}
