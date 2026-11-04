package conf

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	d, err := Parse(`
top = "x" # comment
[claude-work]
command = "claude"
args = ["--model", "claude-sonnet-5"]
env = { CLAUDE_CONFIG_DIR = "/h/.claude-work", A = 'b' }
waiting = ['\[y/n\]', "Do you # want"]
`)
	if err != nil {
		t.Fatal(err)
	}
	s := d["claude-work"]
	if Str(d[""], "top") != "x" || Str(s, "command") != "claude" {
		t.Fatalf("%v", d)
	}
	if !reflect.DeepEqual(List(s, "args"), []string{"--model", "claude-sonnet-5"}) {
		t.Fatalf("args %v", List(s, "args"))
	}
	if Map(s, "env")["CLAUDE_CONFIG_DIR"] != "/h/.claude-work" || Map(s, "env")["A"] != "b" {
		t.Fatalf("env %v", Map(s, "env"))
	}
	if !reflect.DeepEqual(List(s, "waiting"), []string{`\[y/n\]`, "Do you # want"}) {
		t.Fatalf("waiting %v", List(s, "waiting"))
	}
	if _, err := Parse("bad line"); err == nil {
		t.Fatal("want error")
	}
}
