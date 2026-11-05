package prefs

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentMail(t *testing.T) {
	var nilp *Prefs
	if nilp.AgentMail() != MailTeam {
		t.Errorf("nil prefs: %q", nilp.AgentMail())
	}
	t.Setenv("FLEET_CONFIG", filepath.Join(t.TempDir(), "nope.toml"))
	if p, _ := Load(); p.AgentMail() != MailTeam {
		t.Errorf("missing file: %q", p.AgentMail())
	}
	write(t, "[defaults]\nagent_mail = \"open\"\n")
	if p, err := Load(); err != nil || p.AgentMail() != MailOpen {
		t.Errorf("open: %v %v", p, err)
	}
	write(t, "[defaults]\nagent_mail = \"everyone\"\n")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), `agent_mail "everyone"`) {
		t.Errorf("bad value: %v", err)
	}
}
