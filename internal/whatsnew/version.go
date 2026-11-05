package whatsnew

import (
	"runtime/debug"
	"time"
)

// Version is "fleet <rev>[+dirty] · built YYYY-MM-DD HH:MM" from the build
// info, or "fleet (unknown build)".
func Version() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return versionOf(nil)
	}
	return versionOf(bi.Settings)
}

func versionOf(settings []debug.BuildSetting) string {
	var rev, at string
	dirty := false
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			at = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "fleet (unknown build)"
	}
	v := "fleet " + rev[:min(7, len(rev))]
	if dirty {
		v += "+dirty"
	}
	if t, err := time.Parse(time.RFC3339, at); err == nil {
		v += " · built " + t.Local().Format("2006-01-02 15:04")
	}
	return v
}
