package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/netcli/netcli/internal/config"
	"github.com/netcli/netcli/internal/manager"
	"github.com/netcli/netcli/internal/stats"
)

// Mode is a CLI mode, as on IOS.
type Mode int

const (
	ModeUser      Mode = iota // host>
	ModeExec                  // host#
	ModeConfig                // host(config)#
	ModeInterface             // host(config-if)#
	ModeACL                   // host(config-ext-nacl)#
	ModeDHCP                  // host(dhcp-config)#
)

// Session is one CLI session (an interactive terminal, a script or a web
// request).
type Session struct {
	Mgr     *manager.Manager
	Out     io.Writer
	Mode    Mode
	Version string

	// Sample returns current interface counters for "show interfaces".
	Sample func() (*stats.Sample, error)
	// SetTrace enables or disables printing of executed system commands.
	SetTrace func(func(string))
	// ReadLine asks the user for input (confirmations, passwords).
	ReadLine func(prompt string, echo bool) (string, error)
	// RevertArgs are global flags passed on to the revert watchdog.
	RevertArgs []string

	Iface, ACL, Pool string
	DebugApply       bool
	History          []string
	quit             bool
	cached           *config.Config
}

// Quit reports whether the session asked to end.
func (s *Session) Quit() bool { return s.quit }

func (s *Session) printf(format string, a ...any) { fmt.Fprintf(s.Out, format, a...) }
func (s *Session) println(a ...any)               { fmt.Fprintln(s.Out, a...) }

// running returns the running configuration. It is read fresh for every
// command so changes made from the web interface are visible at once.
func (s *Session) running() *config.Config {
	if s.cached != nil {
		return s.cached
	}
	c, err := s.Mgr.Running()
	if err != nil || c == nil {
		c = config.New()
	}
	s.cached = c
	return c
}

// change applies a modification to the running configuration.
func (s *Session) change(f func(c *config.Config) error) error {
	s.cached = nil
	_, err := s.Mgr.Change(f)
	s.cached = nil
	return err
}

// Prompt returns the prompt for the current mode.
func (s *Session) Prompt() string {
	s.cached = nil
	h := s.running().Hostname
	if h == "" {
		h = "router"
	}
	switch s.Mode {
	case ModeUser:
		return h + ">"
	case ModeExec:
		return h + "#"
	case ModeConfig:
		return h + "(config)#"
	case ModeInterface:
		return h + "(config-if)#"
	case ModeACL:
		return h + "(config-ext-nacl)#"
	case ModeDHCP:
		return h + "(dhcp-config)#"
	}
	return h + "?"
}

func (s *Session) isConfig() bool { return s.Mode >= ModeConfig }

// tree returns the command tree for the current mode.
func (s *Session) tree() *Node {
	switch s.Mode {
	case ModeUser:
		return userTree
	case ModeExec:
		return execTree
	case ModeConfig:
		return configTree
	case ModeInterface:
		return interfaceTree
	case ModeACL:
		return aclTree
	case ModeDHCP:
		return dhcpTree
	}
	return userTree
}

// Execute runs one command line. promptLen is the length of the prompt the
// line was typed after, used to place the "^" marker under the offending
// token. A negative promptLen means the line was not echoed (scripts), so
// the line is printed together with any error.
func (s *Session) Execute(line string, promptLen int) bool {
	s.cached = nil
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "!") || strings.HasPrefix(trimmed, "#") {
		return true
	}
	s.History = append(s.History, trimmed)
	if len(s.History) > 200 {
		s.History = s.History[1:]
	}
	prompt := s.Prompt()
	err := s.dispatch(line)
	if err == nil {
		return true
	}
	if promptLen < 0 {
		s.printf("%s%s\n", prompt, line)
		promptLen = len(prompt)
	}
	var pe *ParseError
	if errors.As(err, &pe) {
		s.printf("%s^\n", strings.Repeat(" ", promptLen+pe.Pos))
		s.println(pe.Msg)
		return false
	}
	for _, l := range strings.Split(strings.TrimRight(err.Error(), "\n"), "\n") {
		if !strings.HasPrefix(l, "%") {
			l = "% " + l
		}
		s.println(l)
	}
	return false
}

func (s *Session) dispatch(line string) error {
	toks, err := tokenize(line)
	if err != nil {
		return err
	}
	tree := s.tree()
	noForm := false
	rest := toks
	if s.isConfig() && len(toks) > 0 && strings.EqualFold(toks[0].text, "no") {
		noForm = true
		rest = toks[1:]
	}
	n, a, err := walk(tree, line, rest)
	if err != nil && s.Mode > ModeConfig {
		// As on IOS, global configuration commands are accepted in
		// sub-modes and return to global configuration mode.
		if gn, ga, gerr := walk(configTree, line, rest); gerr == nil && runnable(gn, noForm) {
			s.Mode = ModeConfig
			return call(s, gn, ga, noForm)
		}
	}
	if err != nil {
		return err
	}
	if !runnable(n, noForm) {
		if noForm && n != tree && n.Run != nil && !hasNo(n) {
			return &ParseError{Pos: toks[0].start, Msg: "% This command has no \"no\" form."}
		}
		return errIncomplete
	}
	return call(s, n, a, noForm)
}

func runnable(n *Node, noForm bool) bool {
	if noForm {
		return n.NoRun != nil
	}
	return n.Run != nil
}

func call(s *Session, n *Node, a *Args, noForm bool) error {
	if noForm {
		return n.NoRun(s, a)
	}
	return n.Run(s, a)
}

// Help returns the "?" output for a partial line.
func (s *Session) Help(line string) string {
	toks, _ := tokenize(line)
	noForm := s.isConfig() && len(toks) > 0 && strings.EqualFold(toks[0].text, "no") &&
		(len(toks) > 1 || strings.HasSuffix(line, " "))
	tree := s.tree()
	sub := line
	if noForm {
		sub = line[toks[0].end:]
	}
	lines, err := helpFor(s, tree, sub, noForm)
	if err != nil {
		var pe *ParseError
		if errors.As(err, &pe) {
			return pe.Msg + "\n"
		}
		return "% " + err.Error() + "\n"
	}
	if len(lines) == 0 {
		return "% Unrecognized command\n"
	}
	w := 0
	for _, l := range lines {
		w = max(w, len(l.name))
	}
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "  %-*s  %s\n", w, l.name, l.help)
	}
	return b.String()
}

// Complete returns completions for the last word of a partial line.
func (s *Session) Complete(line string) []string {
	toks, _ := tokenize(line)
	tree := s.tree()
	if s.isConfig() && len(toks) > 0 && strings.EqualFold(toks[0].text, "no") && (len(toks) > 1 || strings.HasSuffix(line, " ")) {
		return complete(s, tree, line[toks[0].end:])
	}
	return complete(s, tree, line)
}

// exitMode implements "exit".
func (s *Session) exitMode() {
	switch s.Mode {
	case ModeUser, ModeExec:
		s.quit = true
	case ModeConfig:
		s.Mode = ModeExec
	default:
		s.Mode = ModeConfig
	}
}

// End implements "end" and Ctrl-Z.
func (s *Session) End() {
	if s.isConfig() {
		s.Mode = ModeExec
	}
}
