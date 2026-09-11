package git

import (
	"testing"
	"time"
)

func TestParseStatus(t *testing.T) {
	in := "# branch.oid a5fd48ed25f6c3818e5266b7e19bab53fdfafe07\n" +
		"# branch.head main\n" +
		"# branch.upstream origin/main\n" +
		"# branch.ab +2 -1\n" +
		"1 AM N... 000000 100644 100644 0000000000000000000000000000000000000000 78981922613b2afb6025042ff6bd878ac1994e85 a.txt\n" +
		"1 .M N... 100644 100644 100644 0000000000000000000000000000000000000000 78981922613b2afb6025042ff6bd878ac1994e85 dir/x y.go\n" +
		"2 R. N... 100644 100644 100644 0000000000000000000000000000000000000000 78981922613b2afb6025042ff6bd878ac1994e85 R100 new.txt\told.txt\n" +
		"u UU N... 100644 100644 100644 100644 0000000000000000000000000000000000000000 0000000000000000000000000000000000000000 0000000000000000000000000000000000000000 conflict.txt\n" +
		"? b.txt\n" +
		"! ignored.txt\n"
	st, err := ParseStatus(in)
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "main" || st.Upstream != "origin/main" || st.Ahead != 2 || st.Behind != 1 || st.Detached {
		t.Errorf("branch header wrong: %+v", st)
	}
	if st.OID != "a5fd48ed25f6c3818e5266b7e19bab53fdfafe07" {
		t.Errorf("oid wrong: %q", st.OID)
	}
	want := []FileStatus{
		{Path: "a.txt", Staged: 'A', Unstaged: 'M'},
		{Path: "dir/x y.go", Staged: '.', Unstaged: 'M'},
		{Path: "new.txt", OrigPath: "old.txt", Staged: 'R', Unstaged: '.'},
		{Path: "conflict.txt", Staged: 'U', Unstaged: 'U', Conflict: true},
		{Path: "b.txt", Untracked: true},
	}
	if len(st.Files) != len(want) {
		t.Fatalf("got %d files, want %d: %+v", len(st.Files), len(want), st.Files)
	}
	for i := range want {
		if st.Files[i] != want[i] {
			t.Errorf("file %d: got %+v want %+v", i, st.Files[i], want[i])
		}
	}
	if st.StagedCount() != 2 || st.UnstagedCount() != 2 || st.UntrackedCount() != 1 || st.ConflictCount() != 1 {
		t.Errorf("counts wrong: staged=%d unstaged=%d untracked=%d conflict=%d",
			st.StagedCount(), st.UnstagedCount(), st.UntrackedCount(), st.ConflictCount())
	}
}

func TestParseStatusDetachedNoUpstream(t *testing.T) {
	in := "# branch.oid abc\n# branch.head (detached)\n"
	st, err := ParseStatus(in)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Detached || st.Upstream != "" || st.HasUpstream() {
		t.Errorf("got %+v", st)
	}
	if st.Dirty() {
		t.Error("clean tree reported dirty")
	}
}

func TestParseWorktrees(t *testing.T) {
	in := "worktree /private/tmp/pg-fixture\nHEAD a5fd48ed25f6c3818e5266b7e19bab53fdfafe07\nbranch refs/heads/main\n\n" +
		"worktree /private/tmp/pg-fixture-feat\nHEAD a5fd48ed25f6c3818e5266b7e19bab53fdfafe07\nbranch refs/heads/feat\nlocked reason here\n\n" +
		"worktree /private/tmp/gone\nHEAD deadbeef\ndetached\nprunable gitdir file points to non-existent location\n\n" +
		"worktree /private/tmp/bare.git\nbare\n\n"
	wts, err := ParseWorktrees(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 4 {
		t.Fatalf("got %d worktrees: %+v", len(wts), wts)
	}
	if wts[0].Path != "/private/tmp/pg-fixture" || wts[0].Branch != "main" || wts[0].Head != "a5fd48ed25f6c3818e5266b7e19bab53fdfafe07" {
		t.Errorf("wt0: %+v", wts[0])
	}
	if wts[1].Branch != "feat" || wts[1].Locked != "reason here" {
		t.Errorf("wt1: %+v", wts[1])
	}
	if !wts[2].Detached || wts[2].Prunable == "" || wts[2].Branch != "" {
		t.Errorf("wt2: %+v", wts[2])
	}
	if !wts[3].Bare {
		t.Errorf("wt3: %+v", wts[3])
	}
}

func TestParseAheadBehind(t *testing.T) {
	a, b, err := ParseAheadBehind("3\t5\n")
	if err != nil || a != 3 || b != 5 {
		t.Errorf("got %d %d %v", a, b, err)
	}
	if _, _, err := ParseAheadBehind("garbage"); err == nil {
		t.Error("expected error")
	}
}

func TestParseLog(t *testing.T) {
	in := "a5fd48ed25f6c3818e5266b7e19bab53fdfafe07\x00a5fd48e\x00Jacek 'Spikes' Wozniak\x001789102197\x00init\x00\n" +
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\x00bbbbbbb\x00Someone\x001789102000\x00second: with, punctuation\x00\n"
	cs, err := ParseLog(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("got %d commits", len(cs))
	}
	if cs[0].Short != "a5fd48e" || cs[0].Author != "Jacek 'Spikes' Wozniak" || cs[0].Subject != "init" {
		t.Errorf("c0: %+v", cs[0])
	}
	if !cs[0].When.Equal(time.Unix(1789102197, 0)) {
		t.Errorf("time: %v", cs[0].When)
	}
	if cs[1].Subject != "second: with, punctuation" {
		t.Errorf("c1: %+v", cs[1])
	}
	if cs, _ := ParseLog(""); len(cs) != 0 {
		t.Errorf("empty log gave %d", len(cs))
	}
}

func TestParseNameStatus(t *testing.T) {
	in := "M\ta.txt\nA\tnew dir/b.txt\nD\tgone.txt\nR095\told.txt\tnew.txt\nC050\tsrc.txt\tcopy.txt\n"
	fs, err := ParseNameStatus(in)
	if err != nil {
		t.Fatal(err)
	}
	want := []DiffFile{
		{Status: 'M', Path: "a.txt"},
		{Status: 'A', Path: "new dir/b.txt"},
		{Status: 'D', Path: "gone.txt"},
		{Status: 'R', Path: "new.txt", OrigPath: "old.txt"},
		{Status: 'C', Path: "copy.txt", OrigPath: "src.txt"},
	}
	if len(fs) != len(want) {
		t.Fatalf("got %+v", fs)
	}
	for i := range want {
		if fs[i] != want[i] {
			t.Errorf("%d: got %+v want %+v", i, fs[i], want[i])
		}
	}
}

func TestParseBranches(t *testing.T) {
	in := "feat\t\t \nmain\torigin/main\t*\n"
	bs, err := ParseBranches(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 2 || bs[0].Name != "feat" || bs[0].Current || bs[1].Name != "main" || bs[1].Upstream != "origin/main" || !bs[1].Current {
		t.Errorf("got %+v", bs)
	}
}
