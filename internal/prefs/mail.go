package prefs

import (
	"fmt"
	"strings"

	"github.com/sturlabragason/fleet/internal/conf"
)

// Agent mail policies for [defaults] agent_mail (DESIGN §6.6, ADR-34).
const (
	// MailTeam: same team, or the same repo when neither agent has a team,
	// message freely; other pairs ask. The default.
	MailTeam = "team"
	// MailOpen: every agent may message every other.
	MailOpen = "open"
	// MailAsk: every pair asks first (the behaviour before ADR-34).
	MailAsk = "ask"
)

func parseAgentMail(sec map[string]conf.Value) (string, error) {
	v := strings.TrimSpace(conf.Str(sec, "agent_mail"))
	switch v {
	case "":
		return "", nil
	case MailTeam, MailOpen, MailAsk:
		return v, nil
	}
	return "", fmt.Errorf("agent_mail %q: want %q, %q or %q", v, MailTeam, MailOpen, MailAsk)
}

// AgentMail is [defaults] agent_mail: who may message whom without asking.
// Fleet-wide; default MailTeam.
func (p *Prefs) AgentMail() string {
	if p == nil || p.agentMail == "" {
		return MailTeam
	}
	return p.agentMail
}
