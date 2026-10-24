package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"
)

// keyFilter adapts terminal input to IOS conventions before x/term sees it:
// Ctrl-C clears the line instead of ending the session, and Ctrl-Z runs
// "end".
type keyFilter struct {
	r   io.Reader
	buf []byte
}

func (k *keyFilter) Read(p []byte) (int, error) {
	if len(k.buf) == 0 {
		raw := make([]byte, len(p))
		n, err := k.r.Read(raw)
		if n == 0 {
			return 0, err
		}
		for _, b := range raw[:n] {
			switch b {
			case 0x03: // Ctrl-C
				k.buf = append(k.buf, 0x15) // Ctrl-U: erase line
			case 0x1a: // Ctrl-Z
				k.buf = append(k.buf, 0x15, 'e', 'n', 'd', '\r')
			default:
				k.buf = append(k.buf, b)
			}
		}
	}
	n := copy(p, k.buf)
	k.buf = k.buf[n:]
	return n, nil
}

type rw struct {
	io.Reader
	io.Writer
}

// Interactive runs a session on a terminal. If in is not a terminal, lines
// are read and executed as a script.
func Interactive(s *Session, in *os.File, out *os.File) error {
	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		return Script(s, in)
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer term.Restore(fd, oldState)

	t := term.NewTerminal(rw{&keyFilter{r: in}, out}, "")
	if w, h, err := term.GetSize(int(out.Fd())); err == nil && w > 0 && h > 0 {
		t.SetSize(w, h)
	}
	s.Out = t
	s.ReadLine = func(prompt string, echo bool) (string, error) {
		if !echo {
			return t.ReadPassword(prompt)
		}
		t.SetPrompt(prompt)
		defer t.SetPrompt(s.Prompt())
		return t.ReadLine()
	}
	prevRun := RunCommand
	RunCommand = func(s *Session, name string, args ...string) error {
		term.Restore(fd, oldState)
		defer term.MakeRaw(fd)
		cmd := exec.Command(name, args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		err := cmd.Run()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil // the program has already reported the problem
		}
		return err
	}
	defer func() { RunCommand = prevRun }()

	t.AutoCompleteCallback = func(line string, pos int, key rune) (string, int, bool) {
		if pos != len(line) {
			return "", 0, false // only act at the end of the line
		}
		switch key {
		case '?':
			if strings.Count(line, "\"")%2 == 1 {
				return "", 0, false // literal ? inside quotes
			}
			fmt.Fprintf(t, "%s%s?\n%s", s.Prompt(), line, s.Help(line))
			return line, pos, true
		case '\t':
			opts := s.Complete(line)
			start := len(line)
			if !strings.HasSuffix(line, " ") && line != "" {
				start = strings.LastIndexAny(line, " \t") + 1
			}
			partial := line[start:]
			switch len(opts) {
			case 0:
				return line, pos, true
			case 1:
				nl := line[:start] + opts[0] + " "
				return nl, len(nl), true
			default:
				cp := commonPrefix(opts)
				if len(cp) > len(partial) {
					nl := line[:start] + cp
					return nl, len(nl), true
				}
				fmt.Fprintf(t, "%s%s\n%s\n", s.Prompt(), line, strings.Join(opts, "  "))
				return line, pos, true
			}
		}
		return "", 0, false
	}

	if b := s.running().Banner; b != "" {
		fmt.Fprintf(t, "\n%s\n\n", b)
	}
	for !s.Quit() {
		prompt := s.Prompt()
		t.SetPrompt(prompt)
		line, err := t.ReadLine()
		if err == io.EOF {
			if s.isConfig() {
				s.endConfig()
				continue
			}
			return nil
		}
		if err != nil && !errors.Is(err, term.ErrPasteIndicator) {
			return err
		}
		s.Execute(line, len(prompt))
	}
	return nil
}

// Script executes commands read from r, one per line. Errors are printed
// with the offending line, and the run continues.
func Script(s *Session, r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() && !s.Quit() {
		s.Execute(sc.Text(), -1)
	}
	return sc.Err()
}
