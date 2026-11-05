package conf

import (
	"reflect"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	d, err := Parse(`
top = "x" # comment
[claude-work]
command = "claude"
args = ["--model", "claude-sonnet-5"]
env = { CLAUDE_CONFIG_DIR = "/h/.claude-work", A = 'b' }
waiting = ['\[y/n\]', "Do you # want", '\[Paste #\d+'] # comment
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
	if !reflect.DeepEqual(List(s, "waiting"), []string{`\[y/n\]`, "Do you # want", `\[Paste #\d+`}) {
		t.Fatalf("waiting %v", List(s, "waiting"))
	}
	if _, err := Parse("bad line"); err == nil {
		t.Fatal("want error")
	}
}

func TestBoolAndDur(t *testing.T) {
	d, err := Parse("yes = true\nno = False\nnum = 3\nt = \"5s\"\nbad = \"fortnight\"\n")
	if err != nil {
		t.Fatal(err)
	}
	m := d[""]
	for _, c := range []struct {
		key     string
		val, ok bool
	}{
		{"yes", true, true}, {"no", false, true}, {"num", false, false}, {"missing", false, false},
	} {
		v, ok := Bool(m, c.key)
		if v != c.val || ok != c.ok {
			t.Errorf("Bool(%s) = %v,%v want %v,%v", c.key, v, ok, c.val, c.ok)
		}
	}
	if v, ok := Dur(m, "t"); !ok || v != 5*time.Second {
		t.Errorf("Dur(t) = %v,%v", v, ok)
	}
	for _, k := range []string{"bad", "missing"} {
		if _, ok := Dur(m, k); ok {
			t.Errorf("Dur(%s) should fail", k)
		}
	}
}
