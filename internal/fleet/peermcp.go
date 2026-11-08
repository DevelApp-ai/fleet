package fleet

import (
	"path/filepath"

	"github.com/sturlabragason/fleet/internal/logx"
	"github.com/sturlabragason/fleet/internal/peerfile"
	"github.com/sturlabragason/fleet/internal/tools"
)

// wirePeer merges the agent's fleet-peer server into the project config
// file its profile names (ADR-64): the MCP wiring for tools with no CLI
// flag for servers. Best effort, like the trust answer: a dir that cannot
// be written (a read-only checkout under spawn --no-worktree, say) is
// logged and skipped, and the agent still has the mail CLI.
func (f *Fleet) wirePeer(name string, p tools.Profile, dir string) {
	path := filepath.Join(dir, p.PeerProject)
	changed, err := peerfile.Add(path, tools.PeerMCPVibe(f.Exe, name))
	switch {
	case err != nil:
		logx.For("fleet").Warn("peer mcp not wired; agent falls back to the mail CLI",
			"agent", name, "file", path, "err", err)
	case changed:
		logx.For("fleet").Info("wired peer mcp", "agent", name, "file", path)
	}
}

// unwirePeer takes the server back out when the launch it was wired for
// did not take.
func (f *Fleet) unwirePeer(name string, p tools.Profile, dir string) {
	if p.PeerProject == "" {
		return
	}
	path := filepath.Join(dir, p.PeerProject)
	if _, err := peerfile.Remove(path, tools.PeerMCPName(name)); err != nil {
		logx.For("fleet").Warn("peer mcp left behind by a failed launch",
			"agent", name, "file", path, "err", err)
	}
}

// forgetPeer takes the closed agent's fleet-peer server out of the
// project config file (ADR-64). The file itself goes only when fleet
// wrote it and nothing else is left in it.
func (f *Fleet) forgetPeer(name string) {
	a := f.Store.Load().Agents[name]
	if a == nil {
		return
	}
	p, ok := f.Profiles[a.Profile]
	if !ok || p.PeerProject == "" {
		return
	}
	path := filepath.Join(a.Dir, p.PeerProject)
	changed, err := peerfile.Remove(path, tools.PeerMCPName(name))
	switch {
	case err != nil:
		logx.For("fleet").Warn("peer mcp not removed on close", "agent", name, "file", path, "err", err)
	case changed:
		logx.For("fleet").Info("removed peer mcp", "agent", name, "file", path)
	}
}
