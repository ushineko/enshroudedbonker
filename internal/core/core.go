/*
Package core holds every user-facing operation as a headless function (spec
002 R7, nmsbonker's rule).

An operation takes a request struct and returns a result struct; the cobra CLI
and the Fyne window only render. Nothing here writes to the game directory
except Deploy and Rollback.
*/
package core

import (
	"fmt"

	"github.com/ushineko/enshroudedbonker/internal/build"
	"github.com/ushineko/enshroudedbonker/internal/compat"
	"github.com/ushineko/enshroudedbonker/internal/config"
	"github.com/ushineko/enshroudedbonker/internal/emm"
	"github.com/ushineko/enshroudedbonker/internal/steam"
)

// Level ranks a log line.
type Level int

// Log levels.
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String names a level for the CLI.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	}
	return "?"
}

// Progress reports how far through a long operation we are.
type Progress struct {
	Step  int
	Total int
	What  string
}

// Events is how a long operation talks back to its front end. Both fields may
// be nil.
type Events struct {
	Log      func(level Level, msg string)
	Progress func(p Progress)
}

func (e Events) logf(level Level, format string, args ...any) {
	if e.Log != nil {
		e.Log(level, fmt.Sprintf(format, args...))
	}
}

func (e Events) progress(step, total int, what string) {
	if e.Progress != nil {
		e.Progress(Progress{Step: step, Total: total, What: what})
	}
}

// Request is what every operation carries.
type Request struct {
	// ConfigPath overrides the settings file; "" uses the default.
	ConfigPath string
	// GameDir overrides the setting and the environment.
	GameDir string
	Events  Events
	// Running overrides the game-process check (tests).
	Running func() (bool, bool)
}

// session is the resolved state an operation works from.
type session struct {
	req        Request
	cfg        *config.Config
	paths      config.Paths
	install    *steam.Install
	installErr error
	gameSource string
	fp         *compat.Fingerprint
	fpErr      error
}

func open(req Request) (*session, error) {
	var (
		cfg *config.Config
		err error
	)
	if req.ConfigPath != "" {
		cfg, err = config.LoadFrom(req.ConfigPath)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		return nil, err
	}
	s := &session{req: req, cfg: cfg, paths: cfg.Paths()}
	dir, source := cfg.ResolvedGameDir()
	if req.GameDir != "" {
		dir, source = config.ExpandPath(req.GameDir), "--game-dir"
	}
	s.gameSource = source
	s.install, s.installErr = steam.Locate(dir)
	return s, nil
}

func (s *session) requireInstall() error { return s.installErr }

// fingerprint reads the installed build once per operation.
func (s *session) fingerprint() (compat.Fingerprint, error) {
	if s.fp != nil || s.fpErr != nil {
		if s.fp == nil {
			return compat.Fingerprint{}, s.fpErr
		}
		return *s.fp, nil
	}
	if err := s.requireInstall(); err != nil {
		s.fpErr = err
		return compat.Fingerprint{}, err
	}
	fp, err := compat.Read(s.install.Exe(), s.install.Kfc(), s.install.BuildID)
	if err != nil {
		s.fpErr = err
		return fp, err
	}
	s.fp = &fp
	return fp, nil
}

// gate decides whether this install may be built for.
func (s *session) gate() (compat.Decision, error) {
	fp, err := s.fingerprint()
	if err != nil {
		return compat.Decision{}, err
	}
	dry, err := compat.LoadDryRun(s.paths.Compat, fp)
	if err != nil {
		return compat.Decision{}, err
	}
	return compat.Decide(fp, dry), nil
}

func (s *session) pristine() (*build.Pristine, error) {
	fp, err := s.fingerprint()
	if err != nil {
		return nil, err
	}
	return build.LoadPristine(s.paths.Pristine, fp)
}

func (s *session) emm() (path, source string, err error) {
	return emm.Locate(s.paths.Tools, s.cfg.EmmPath)
}

func (s *session) running() func() (bool, bool) {
	if s.req.Running != nil {
		return s.req.Running
	}
	return steam.GameRunning
}

// emmLog adapts a core event stream to emm's line callback.
func (s *session) emmLog() func(string) {
	return func(line string) { s.req.Events.logf(LevelDebug, "%s", line) }
}

func (s *session) infoLog() func(string) {
	return func(line string) { s.req.Events.logf(LevelInfo, "%s", line) }
}
