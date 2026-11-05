package status

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"time"

	"github.com/sturlabragason/fleet/internal/logx"
)

// ScanSubagents applies the complete lines in data to prev and returns the new
// state (list pruned) and how many bytes it consumed (up to and including the
// last '\n'; a partial last line is left for the next call).
func ScanSubagents(data []byte, prev Subs, now time.Time) (Subs, int64) {
	st := prev.clone()
	var off int
	for {
		n := bytes.IndexByte(data[off:], '\n')
		if n < 0 {
			break
		}
		st = applyLine(st, data[off:off+n], now)
		off += n + 1
	}
	st.List = PruneSubagents(st.List, now)
	return st, int64(off)
}

// ReadSubagents reads the event log at path from offset to EOF and applies it
// to prev. A file shorter than offset was replaced: it is read from 0 with an
// empty state. A missing file leaves (prev, offset) unchanged.
func ReadSubagents(path string, offset int64, prev Subs, now time.Time) (Subs, int64) {
	log := logx.For("status")
	f, err := os.Open(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Warn("subagent log", "path", path, "err", err)
		}
		return prev, offset
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		log.Warn("subagent log", "path", path, "err", err)
		return prev, offset
	}
	if fi.Size() < offset {
		offset, prev = 0, Subs{}
	}
	st := prev.clone()
	if fi.Size() == offset {
		st.List = PruneSubagents(st.List, now)
		return st, offset
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		log.Warn("subagent log", "path", path, "err", err)
		return prev, offset
	}
	// ReadBytes grows past the buffer, so multi-MB lines are fine (a
	// bufio.Scanner would stop at its 64K default).
	r := bufio.NewReaderSize(f, 256<<10)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				log.Warn("subagent log", "path", path, "err", err)
			}
			break // a partial last line waits for the next poll
		}
		offset += int64(len(line))
		st = applyLine(st, line[:len(line)-1], now)
	}
	st.List = PruneSubagents(st.List, now)
	return st, offset
}
