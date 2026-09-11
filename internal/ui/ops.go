package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/spik3s/pyragit/internal/git"
	"github.com/spik3s/pyragit/internal/state"
)

// opFunc is a long-running git operation that streams lines.
type opFunc func(ctx context.Context, onLine func(string)) error

// opEvent is one line or the completion of an operation.
type opEvent struct {
	id   int
	line string
	done bool
	err  error
	took time.Duration
}

type opMsg opEvent

// followUp says what to do once an operation finishes successfully.
type followUp struct {
	refresh    []*state.Worktree
	rediscover bool
	selectPath string
}

// op is the currently running (or last finished) operation.
type op struct {
	id      int
	name    string
	dir     string
	quiet   bool // do not auto-open the output pane
	running bool
	err     error
	started time.Time
	took    time.Duration
	cancel  context.CancelFunc
	ch      chan opEvent
	follow  followUp
	lines   []string
}

var opSeq int

// start launches fn in a goroutine and returns the command that waits for
// its first event.
func startOp(name, dir string, quiet bool, follow followUp, fn opFunc) (*op, tea.Cmd) {
	opSeq++
	ctx, cancel := context.WithCancel(context.Background())
	o := &op{
		id: opSeq, name: name, dir: dir, quiet: quiet, running: true,
		started: time.Now(), cancel: cancel, ch: make(chan opEvent, 1024), follow: follow,
	}
	go func() {
		err := fn(ctx, func(line string) { o.ch <- opEvent{id: o.id, line: line} })
		if ctx.Err() != nil && err != nil {
			err = fmt.Errorf("cancelled")
		}
		o.ch <- opEvent{id: o.id, done: true, err: err, took: time.Since(o.started)}
	}()
	return o, waitOpCmd(o)
}

func waitOpCmd(o *op) tea.Cmd {
	return func() tea.Msg { return opMsg(<-o.ch) }
}

// fetchAllOp fetches every project sequentially, prefixing output lines.
func fetchAllOp(projects []*state.Project) opFunc {
	return func(ctx context.Context, onLine func(string)) error {
		var failed []string
		for _, p := range projects {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			name := p.Name
			onLine("→ " + name)
			err := git.Fetch(ctx, p.Path, func(l string) { onLine("  [" + name + "] " + l) })
			if err != nil {
				failed = append(failed, name)
				onLine("  [" + name + "] " + err.Error())
			}
		}
		if len(failed) > 0 {
			return fmt.Errorf("fetch failed for %s", strings.Join(failed, ", "))
		}
		return nil
	}
}

// outputPane shows streamed operation output at the bottom of the screen.
type outputPane struct {
	vp      viewport.Model
	visible bool
	width   int
	height  int
	lines   []string
	op      *op
}

func newOutputPane() outputPane {
	return outputPane{vp: viewport.New()}
}

func (o *outputPane) resize(w, h int) {
	o.width, o.height = w, h
	o.vp.SetWidth(max(w-2, 1))
	o.vp.SetHeight(max(h-2, 1))
	o.render()
}

func (o *outputPane) reset(op *op) {
	o.op = op
	o.lines = o.lines[:0]
	o.render()
}

func (o *outputPane) append(line string) {
	o.lines = append(o.lines, line)
	if len(o.lines) > 2000 {
		o.lines = o.lines[len(o.lines)-2000:]
	}
	o.render()
}

func (o *outputPane) render() {
	atBottom := o.vp.YOffset() >= max(len(o.lines)-o.vp.Height(), 0)
	if len(o.lines) == 0 {
		o.vp.SetContent("")
	} else {
		o.vp.SetContent(strings.Join(o.lines, "\n"))
	}
	if atBottom {
		o.vp.GotoBottom()
	}
}

func (o *outputPane) title(t Theme) string {
	if o.op == nil {
		return "Output"
	}
	title := o.op.name
	switch {
	case o.op.running:
		title += " " + t.Dim.Render(fmt.Sprintf("running %s", time.Since(o.op.started).Round(time.Second)))
	case o.op.err != nil:
		title += " " + t.BadgeWarn.Render("✗ "+o.op.err.Error())
	default:
		title += " " + t.BadgeAhead.Render(fmt.Sprintf("✓ %s", o.op.took.Round(time.Millisecond)))
	}
	return title
}

func (o *outputPane) view(t Theme, focused bool) string {
	return frame(t, o.title(t), o.vp.View(), o.width, o.height, focused)
}
