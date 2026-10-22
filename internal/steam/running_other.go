//go:build !linux

package steam

// GameRunning cannot tell on this platform yet (Windows support is a later
// spec). known=false makes Deploy ask rather than assume.
func GameRunning() (running bool, known bool) { return false, false }
