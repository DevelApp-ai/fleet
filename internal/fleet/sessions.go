package fleet

import (
	"fmt"
	"strings"
	"time"

	"github.com/sturlabragason/fleet/internal/board"
	"github.com/sturlabragason/fleet/internal/sessions"
)

// Sessions lists sessions in the profiles' stores, marking ones on the board.
func (f *Fleet) Sessions(o sessions.Opts) ([]sessions.Session, error) {
	o.Board = map[string]bool{}
	for _, a := range f.Store.Load().Agents {
		if a.SessionID != "" {
			o.Board[a.SessionID] = true
		}
	}
	return sessions.List(f.Profiles, o)
}

// ResolveID finds the session for an exact ID or a unique prefix of at least 8
// characters. The result's Profile is the given one, else the suggested one;
// Dir is the session's working dir. ok is false when nothing matches. A
// profile without a session store takes an ID-shaped id as given.
func (f *Fleet) ResolveID(profile, id string) (s sessions.Session, ok bool, err error) {
	store := ""
	if profile != "" {
		p, known := f.Profiles[profile]
		if !known {
			return sessions.Session{ID: id, Profile: profile}, true, nil // Adopt reports unknown profiles
		}
		if p.SessionPath == "" {
			if !sessions.IDRe.MatchString(id) {
				return s, false, fmt.Errorf("profile %s has no session store to search; pass the session ID", profile)
			}
			return sessions.Session{ID: id, Profile: profile}, true, nil
		}
		store = p.SessionPath
	}
	ss := sessions.Lookup(f.Profiles, id, store)
	switch len(ss) {
	case 0:
		return s, false, nil
	case 1:
		if profile != "" {
			ss[0].Profile = profile
		}
		return ss[0], true, nil
	}
	return s, false, fmt.Errorf("%d sessions start with %q:\n%s\nbe more specific or pass the ID", len(ss), id, candidates(ss))
}

// ResolveWords finds the one session (not in use, not on the board) whose
// title, dir, branch or ID contain every word. Profile is set as in ResolveID.
func (f *Fleet) ResolveWords(profile string, words []string) (sessions.Session, error) {
	var none sessions.Session
	q := strings.Join(words, " ")
	if strings.TrimSpace(q) == "" {
		return none, fmt.Errorf("no session id or words given")
	}
	o := sessions.Opts{Words: words, SkipBusy: true}
	if profile != "" {
		p, known := f.Profiles[profile]
		if !known {
			return none, fmt.Errorf("unknown profile %q", profile)
		}
		if p.SessionPath == "" {
			return none, fmt.Errorf("profile %s has no session store to search; pass the session ID", profile)
		}
		o.Store = p.SessionPath
	}
	ss, err := f.Sessions(o)
	if err != nil {
		return none, err
	}
	switch len(ss) {
	case 0:
		return none, fmt.Errorf("no session matches %q; try fleet sessions", q)
	case 1:
		if profile != "" {
			ss[0].Profile = profile
		}
		return ss[0], nil
	}
	return none, fmt.Errorf("%d sessions match %q:\n%s\nbe more specific or pass the ID", len(ss), q, candidates(ss))
}

// AdoptResolved adopts s with o, taking s's profile and ID, and its dir when o has none.
func (f *Fleet) AdoptResolved(o AdoptOpts, s sessions.Session) (*board.Agent, error) {
	o.Profile, o.SessionID = s.Profile, s.ID
	if o.Dir == "" {
		o.Dir = s.Dir
	}
	return f.Adopt(o)
}

// AdoptArgs adopts from CLI positionals `[profile] <id | words...>`: an ID or
// unique ID prefix (optionally followed by a dir), else words.
func (f *Fleet) AdoptArgs(o AdoptOpts, args []string) (*board.Agent, error) {
	if _, ok := f.Profiles[firstOr(args)]; ok && len(args) > 1 {
		o.Profile, args = args[0], args[1:]
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("no session id or words given")
	}
	if len(args) <= 2 {
		s, ok, err := f.ResolveID(o.Profile, args[0])
		if err != nil {
			return nil, err
		}
		if ok {
			if len(args) == 2 && o.Dir == "" {
				o.Dir = args[1]
			}
			return f.AdoptResolved(o, s)
		}
	}
	s, err := f.ResolveWords(o.Profile, args)
	if err != nil {
		// --force with a profile still adopts an ID that isn't in the store.
		if o.Force && o.Profile != "" && len(args) <= 2 && sessionIDRe.MatchString(args[0]) {
			o.SessionID = args[0]
			if len(args) == 2 && o.Dir == "" {
				o.Dir = args[1]
			}
			return f.Adopt(o)
		}
		return nil, err
	}
	return f.AdoptResolved(o, s)
}

func firstOr(a []string) string {
	if len(a) == 0 {
		return ""
	}
	return a[0]
}

// candidates lists up to 5 sessions as `  <id8>  <age>  <dir>  <title>`.
func candidates(ss []sessions.Session) string {
	var b []string
	for i, s := range ss {
		if i == 5 {
			b = append(b, fmt.Sprintf("  … and %d more", len(ss)-5))
			break
		}
		b = append(b, fmt.Sprintf("  %s  %s  %s  %s", sessions.Short(s.ID), sessions.Age(time.Since(s.Updated)), s.Dir, sessions.OneLine(s.Title, 60)))
	}
	return strings.Join(b, "\n")
}
