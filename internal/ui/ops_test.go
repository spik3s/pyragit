package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestOutputPaneFollowsNewLines(t *testing.T) {
	o := newOutputPane()
	o.resize(40, 5) // 3 content rows inside the border
	o.reset(&op{name: "fetch"})
	for i := 1; i <= 20; i++ {
		o.append(fmt.Sprintf("line %d", i))
	}
	view := ansi.Strip(o.view(NewTheme(true), false))
	if !strings.Contains(view, "line 20") || strings.Contains(view, "line 1\n") {
		t.Errorf("pane did not follow output:\n%s", view)
	}
	if o.vp.YOffset() != 17 {
		t.Errorf("YOffset = %d, want 17", o.vp.YOffset())
	}
}
