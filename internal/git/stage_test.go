package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseStashList(t *testing.T) {
	in := "stash@{0}\x001789102197\x00On main: wip thing\nstash@{1}\x001789100000\x00WIP on feat: abc123 msg\n"
	ss, err := ParseStashList(in)
	if err != nil || len(ss) != 2 {
		t.Fatalf("%+v %v", ss, err)
	}
	if ss[0].Index != 0 || ss[0].Ref != "stash@{0}" || ss[0].Message != "On main: wip thing" || ss[1].Index != 1 {
		t.Errorf("%+v", ss)
	}
	if ss, _ := ParseStashList(""); len(ss) != 0 {
		t.Error("empty")
	}
}

func TestStageDiscardCommitStash(t *testing.T) {
	ctx := context.Background()
	repo := NewTestRepo(t)
	write := func(name, content string) {
		os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644)
	}
	write("README", "changed\n")
	write("new.txt", "new\n")
	os.MkdirAll(filepath.Join(repo, "dir"), 0o755)
	write("dir/x.txt", "x\n")

	if err := Stage(ctx, repo, "README"); err != nil {
		t.Fatal(err)
	}
	st, _ := GetStatus(ctx, repo)
	if st.StagedCount() != 1 || st.UntrackedCount() != 2 {
		t.Fatalf("after stage: %+v", st.Files)
	}
	if err := Unstage(ctx, repo, "README"); err != nil {
		t.Fatal(err)
	}
	st, _ = GetStatus(ctx, repo)
	if st.StagedCount() != 0 || st.UnstagedCount() != 1 {
		t.Fatalf("after unstage: %+v", st.Files)
	}
	if err := StageAll(ctx, repo); err != nil {
		t.Fatal(err)
	}
	st, _ = GetStatus(ctx, repo)
	if st.StagedCount() != 3 {
		t.Fatalf("after stage all: %+v", st.Files)
	}
	if err := UnstageAll(ctx, repo); err != nil {
		t.Fatal(err)
	}
	// Discard one tracked file and delete one untracked dir.
	if err := DiscardFile(ctx, repo, "README"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "README")); string(b) != "hi\n" {
		t.Errorf("README not restored: %q", b)
	}
	if err := DeleteUntracked(ctx, repo, "dir"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "dir")); err == nil {
		t.Error("dir not deleted")
	}
	// Commit and amend.
	Stage(ctx, repo, "new.txt")
	mustGit(t, repo, "config", "user.email", "t@t")
	mustGit(t, repo, "config", "user.name", "t")
	if _, err := CreateCommit(ctx, repo, "add new", false); err != nil {
		t.Fatal(err)
	}
	if HeadSubject(ctx, repo) != "add new" {
		t.Errorf("subject %q", HeadSubject(ctx, repo))
	}
	if _, err := CreateCommit(ctx, repo, "add new (amended)", true); err != nil {
		t.Fatal(err)
	}
	if cs, _ := Log(ctx, repo, "HEAD", 0); len(cs) != 2 || cs[0].Subject != "add new (amended)" {
		t.Errorf("amend: %+v", cs)
	}
	// Stash push/list/show/apply/pop/drop.
	write("README", "stashed\n")
	write("untracked.txt", "u\n")
	if _, err := StashPush(ctx, repo, "my stash", true); err != nil {
		t.Fatal(err)
	}
	st, _ = GetStatus(ctx, repo)
	if st.Dirty() {
		t.Errorf("worktree not clean after stash: %+v", st.Files)
	}
	ss, err := StashList(ctx, repo)
	if err != nil || len(ss) != 1 || !strings.Contains(ss[0].Message, "my stash") {
		t.Fatalf("stash list: %+v %v", ss, err)
	}
	if p, err := StashShow(ctx, repo, ss[0].Ref, false); err != nil || !strings.Contains(p, "+stashed") || !strings.Contains(p, "untracked.txt") {
		t.Errorf("stash show: %v\n%s", err, p)
	}
	if err := StashApply(ctx, repo, ss[0].Ref); err != nil {
		t.Fatal(err)
	}
	if err := DiscardAll(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if st, _ := GetStatus(ctx, repo); st.Dirty() {
		t.Errorf("discard all left changes: %+v", st.Files)
	}
	if err := StashPop(ctx, repo, ss[0].Ref); err != nil {
		t.Fatal(err)
	}
	if st, _ := GetStatus(ctx, repo); st.UnstagedCount() != 1 || st.UntrackedCount() != 1 {
		t.Errorf("after pop: %+v", st.Files)
	}
	if ss, _ := StashList(ctx, repo); len(ss) != 0 {
		t.Errorf("stash not consumed by pop: %+v", ss)
	}
	StashPush(ctx, repo, "", false)
	ss, _ = StashList(ctx, repo)
	if err := StashDrop(ctx, repo, ss[0].Ref); err != nil {
		t.Fatal(err)
	}
	if ss, _ := StashList(ctx, repo); len(ss) != 0 {
		t.Errorf("drop failed: %+v", ss)
	}
}
