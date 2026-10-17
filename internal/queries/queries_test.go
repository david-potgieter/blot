package queries

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func tempStore(t *testing.T) Store {
	return Store{Path: filepath.Join(t.TempDir(), "blot", "queries.json")}
}

func TestListMissingFileIsEmpty(t *testing.T) {
	labels, err := tempStore(t).List()
	if err != nil || len(labels) != 0 {
		t.Fatalf("List() = %v, %v; want empty, nil", labels, err)
	}
}

func TestSaveThenLoad(t *testing.T) {
	s := tempStore(t)
	q := Query{Roots: "~/Work", Min: "0", Show: "dirs", Name: "node_modules"}
	if err := s.Save("node_modules", q); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("node_modules")
	want := Query{Label: "node_modules", Roots: "~/Work", Min: "0", Show: "dirs", Name: "node_modules"}
	if err != nil || got != want {
		t.Fatalf("Load() = %+v, %v; want %+v", got, err, want)
	}
}

func TestListIsSorted(t *testing.T) {
	s := tempStore(t)
	for _, l := range []string{"zeta", "alpha", "mid"} {
		if err := s.Save(l, Query{}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.List()
	if err != nil || !slices.Equal(got, []string{"alpha", "mid", "zeta"}) {
		t.Fatalf("List() = %v, %v; want [alpha mid zeta]", got, err)
	}
}

func TestLoadUnknownLabelErrors(t *testing.T) {
	s := tempStore(t)
	if err := s.Save("a", Query{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("b"); err == nil {
		t.Fatal("Load(unknown) expected error")
	}
}

func TestSaveOverwritesExistingLabel(t *testing.T) {
	s := tempStore(t)
	if err := s.Save("q", Query{Min: "1GB"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("q", Query{Min: "2GB"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("q")
	if err != nil || got.Min != "2GB" {
		t.Fatalf("Load() = %+v, %v; want Min 2GB", got, err)
	}
}

func TestDeleteRemovesQuery(t *testing.T) {
	s := tempStore(t)
	for _, l := range []string{"keep", "drop"} {
		if err := s.Save(l, Query{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Delete("drop"); err != nil {
		t.Fatal(err)
	}
	got, err := s.List()
	if err != nil || !slices.Equal(got, []string{"keep"}) {
		t.Fatalf("List() = %v, %v; want [keep]", got, err)
	}
}

func writeFile(t *testing.T, s Store, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptFileErrorsAndIsNotOverwritten(t *testing.T) {
	s := tempStore(t)
	writeFile(t, s, "{not json")
	if _, err := s.List(); err == nil {
		t.Error("List() on corrupt file expected error")
	}
	if err := s.Save("q", Query{}); err == nil {
		t.Error("Save() on corrupt file expected error")
	}
	if err := s.Delete("q"); err == nil {
		t.Error("Delete() on corrupt file expected error")
	}
	if data, _ := os.ReadFile(s.Path); string(data) != "{not json" {
		t.Errorf("file overwritten: %q", data)
	}
}

func TestHandEditedFileIsRead(t *testing.T) {
	s := tempStore(t)
	writeFile(t, s, `{"node_modules": {"label": "node_modules", "roots": "~/Work", "min": "0", "show": "dirs", "name": "node_modules"}}`)
	got, err := s.Load("node_modules")
	want := Query{Label: "node_modules", Roots: "~/Work", Min: "0", Show: "dirs", Name: "node_modules"}
	if err != nil || got != want {
		t.Fatalf("Load() = %+v, %v; want %+v", got, err, want)
	}
}

func TestSavedFileIsIndented(t *testing.T) {
	s := tempStore(t)
	if err := s.Save("big", Query{Roots: "~", Min: "500MB", Show: "all", Name: "*.log"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.Path)
	want := `{
  "big": {
    "label": "big",
    "roots": "~",
    "min": "500MB",
    "show": "all",
    "name": "*.log"
  }
}
`
	if err != nil || string(data) != want {
		t.Fatalf("file = %q, %v; want %q", data, err, want)
	}
}

func TestDefaultPathIsUnderUserConfigDir(t *testing.T) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		t.Skip(err)
	}
	got, err := DefaultPath()
	if err != nil || got != filepath.Join(cfg, "blot", "queries.json") {
		t.Fatalf("DefaultPath() = %q, %v", got, err)
	}
}
