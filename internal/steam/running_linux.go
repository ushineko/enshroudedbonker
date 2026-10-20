//go:build linux

package steam

import (
	"os"
	"strings"
)

/*
GameRunning reports whether an enshrouded.exe process exists (R7.2).

Under Proton, Wine names the process after the PE image, so /proc/<pid>/comm
reads "enshrouded.exe". comm is matched rather than the command line, which a
shell that merely mentions the file would also match. Linux comm is cut at 15
bytes; the name is 14.
*/
func GameRunning() (running bool, known bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, false
	}
	for _, e := range entries {
		name := e.Name()
		if name == "" || name[0] < '0' || name[0] > '9' {
			continue
		}
		b, err := os.ReadFile("/proc/" + name + "/comm")
		if err != nil {
			continue
		}
		comm := strings.TrimSpace(string(b))
		if strings.EqualFold(comm, ExeName) || strings.EqualFold(comm, "enshrouded_server.exe"[:15]) {
			return true, true
		}
	}
	return false, true
}
