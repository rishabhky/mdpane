package spawn

import "testing"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Terminal
	}{
		{"tmux", map[string]string{"TMUX": "/tmp/tmux-1/default,123,0"}, Tmux},
		{"tmux wins inside wezterm", map[string]string{"TMUX": "x", "WEZTERM_PANE": "3"}, Tmux},
		{"zellij", map[string]string{"ZELLIJ": "0"}, Zellij},
		{"kitty", map[string]string{"KITTY_WINDOW_ID": "1"}, Kitty},
		{"wezterm", map[string]string{"WEZTERM_PANE": "0"}, WezTerm},
		{"warp", map[string]string{"TERM_PROGRAM": "WarpTerminal"}, Warp},
		{"iterm2", map[string]string{"TERM_PROGRAM": "iTerm.app"}, ITerm2},
		{"ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, Ghostty},
		{"bare", map[string]string{}, Unknown},
	}
	for _, c := range cases {
		if got := Detect(env(c.env)); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestShellJoinQuotesSpaces(t *testing.T) {
	got := shellJoin([]string{"mdpane", "attach", "--open", "/tmp/my plan.md"})
	want := `mdpane attach --open '/tmp/my plan.md'`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
