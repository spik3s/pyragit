package uistate

import (
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s", "state.json")
	st, err := Load(p)
	if err != nil || st.Selected != "" {
		t.Fatalf("empty load: %+v %v", st, err)
	}
	want := State{Collapsed: []string{"/a/.git"}, Selected: "/a", OutputOpen: true, LastFilesTab: 2}
	if err := Save(p, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || got.Selected != want.Selected || len(got.Collapsed) != 1 || !got.OutputOpen || got.LastFilesTab != 2 {
		t.Errorf("got %+v %v", got, err)
	}
}
