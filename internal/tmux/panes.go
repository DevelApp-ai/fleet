package tmux

import (
	"strconv"
	"strings"
)

// Spawn opens a detached window (creating session if needed) running cmd.
// Returns the new pane id (%N).
func (c *Client) Spawn(session, window, dir string, env []string, cmd string) (string, error) {
	var args []string
	if c.HasSession(session) {
		args = []string{"new-window", "-d", "-t", session + ":", "-n", window}
	} else {
		args = []string{"new-session", "-d", "-s", session, "-n", window, "-x", "200", "-y", "50"}
	}
	args = append(args, "-P", "-F", "#{pane_id}")
	if dir != "" {
		args = append(args, "-c", dir)
	}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, cmd)
	return c.Run(args...)
}

// SetPaneOption sets a pane-level option such as @fleet_agent.
func (c *Client) SetPaneOption(pane, key, val string) error {
	_, err := c.Run("set-option", "-p", "-t", pane, key, val)
	return err
}

// Capture returns the visible text of a pane (last n lines if n>0).
func (c *Client) Capture(pane string, n int) (string, error) {
	args := []string{"capture-pane", "-p", "-J", "-t", pane}
	if n > 0 {
		args = append(args, "-S", "-"+strconv.Itoa(n))
	}
	return c.Run(args...)
}

// Paste pastes text into a pane via a buffer, then presses Enter.
func (c *Client) Paste(pane, text string, enter bool) error {
	buf := "fleet-" + strings.TrimPrefix(pane, "%")
	if _, err := c.RunInput(text, "load-buffer", "-b", buf, "-"); err != nil {
		return err
	}
	if _, err := c.Run("paste-buffer", "-p", "-d", "-b", buf, "-t", pane); err != nil {
		return err
	}
	if enter {
		return c.SendKeys(pane, "Enter")
	}
	return nil
}

// SendKeys sends tmux key names to a pane.
func (c *Client) SendKeys(pane string, keys ...string) error {
	_, err := c.Run(append([]string{"send-keys", "-t", pane}, keys...)...)
	return err
}

// KillPane kills one pane.
func (c *Client) KillPane(pane string) error { _, err := c.Run("kill-pane", "-t", pane); return err }

// PaneInfo is one row of list-panes.
type PaneInfo struct {
	ID, Window, Agent, Cmd string
	Session                string
	WindowID               string
	Dead                   bool
	Width, Height          int
	Used                   int64  // @fleet_used
	Role                   string // @role
}

const paneFmt = "#{pane_id}\t#{session_name}\t#{window_id}\t#{window_name}\t#{@fleet_agent}\t#{pane_dead}\t#{pane_current_command}\t#{pane_width}\t#{pane_height}\t#{@fleet_used}\t#{@role}"

// ListPanes lists panes; target "" means all (-a), otherwise -t target (a window).
func (c *Client) ListPanes(target string) ([]PaneInfo, error) {
	args := []string{"list-panes", "-F", paneFmt}
	if target == "" {
		args = append(args, "-a")
	} else {
		args = append(args, "-t", target)
	}
	out, err := c.Run(args...)
	if err != nil {
		return nil, err
	}
	var ps []PaneInfo
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 11 {
			continue
		}
		w, _ := strconv.Atoi(f[7])
		h, _ := strconv.Atoi(f[8])
		u, _ := strconv.ParseInt(f[9], 10, 64)
		ps = append(ps, PaneInfo{ID: f[0], Session: f[1], WindowID: f[2], Window: f[3], Agent: f[4], Dead: f[5] == "1", Cmd: f[6], Width: w, Height: h, Used: u, Role: f[10]})
	}
	return ps, nil
}
