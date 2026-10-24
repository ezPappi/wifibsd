// Package cli implements the Cisco IOS style command line: modes, a command
// tree with unique-prefix abbreviations, "?" help, tab completion, "no"
// forms and "do" from configuration modes.
package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ArgType describes a command argument.
type ArgType struct {
	Name     string // shown in help, e.g. "A.B.C.D" or "WORD"
	Help     string
	Match    func(tok string) bool
	Complete func(s *Session, prefix string) []string
	Rest     bool // consumes the remainder of the line
}

// Node is one element in the command tree: a keyword or an argument.
type Node struct {
	Word     string
	Arg      *ArgType
	Key      string // name under which an argument value is stored
	Help     string
	Children []*Node
	Run      func(s *Session, a *Args) error
	NoRun    func(s *Session, a *Args) error
	Hidden   bool
}

// Args collects the keywords and argument values of a parsed command.
type Args struct {
	words  map[string]bool
	values map[string]string
	multi  map[string][]string
}

func newArgs() *Args {
	return &Args{words: map[string]bool{}, values: map[string]string{}, multi: map[string][]string{}}
}

// Has reports whether a keyword was given.
func (a *Args) Has(word string) bool { return a.words[word] }

// Get returns an argument value.
func (a *Args) Get(key string) string { return a.values[key] }

// All returns every value of a repeated argument.
func (a *Args) All(key string) []string { return a.multi[key] }

// ---- tree builders ----------------------------------------------------------

func kw(word, help string, children ...*Node) *Node {
	return &Node{Word: word, Help: help, Children: children}
}

func arg(key string, t *ArgType, children ...*Node) *Node {
	return &Node{Arg: t, Key: key, Help: t.Help, Children: children}
}

// repeat makes an argument node that may be given several times.
func repeat(key string, t *ArgType, run func(*Session, *Args) error) *Node {
	n := &Node{Arg: t, Key: key, Help: t.Help, Run: run}
	n.Children = []*Node{n}
	return n
}

func (n *Node) run(f func(*Session, *Args) error) *Node  { n.Run = f; return n }
func (n *Node) no(f func(*Session, *Args) error) *Node   { n.NoRun = f; return n }
func (n *Node) help(h string) *Node                      { n.Help = h; return n }
func (n *Node) hidden() *Node                            { n.Hidden = true; return n }
func (n *Node) add(children ...*Node) *Node              { n.Children = append(n.Children, children...); return n }
func (n *Node) both(f func(*Session, *Args) error) *Node { n.Run = f; n.NoRun = f; return n }
func root(children ...*Node) *Node                       { return &Node{Children: children} }

// ---- tokenizer ---------------------------------------------------------------

type token struct {
	text  string
	start int // byte offset in the line
	end   int
}

// tokenize splits a line on white space. Double quotes group words and a
// backslash escapes the next character inside quotes.
func tokenize(line string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(line) {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= len(line) {
			break
		}
		start := i
		var b strings.Builder
		inQuote := false
		for i < len(line) {
			ch := line[i]
			if inQuote {
				if ch == '\\' && i+1 < len(line) {
					b.WriteByte(line[i+1])
					i += 2
					continue
				}
				if ch == '"' {
					inQuote = false
					i++
					continue
				}
				b.WriteByte(ch)
				i++
				continue
			}
			if ch == ' ' || ch == '\t' {
				break
			}
			if ch == '"' {
				inQuote = true
				i++
				continue
			}
			b.WriteByte(ch)
			i++
		}
		if inQuote {
			return toks, fmt.Errorf("unterminated quote")
		}
		toks = append(toks, token{text: b.String(), start: start, end: i})
	}
	return toks, nil
}

// quote renders a value so tokenize reads it back unchanged.
func quote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"\\") {
		return s
	}
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"")
	return "\"" + r.Replace(s) + "\""
}

// ---- matching ----------------------------------------------------------------

// ParseError carries the offset of the offending token for the "^" marker.
type ParseError struct {
	Pos int
	Msg string
}

func (e *ParseError) Error() string { return e.Msg }

var (
	errIncomplete = errors.New("% Incomplete command.")
)

// candidates returns the children matching a (possibly partial) token.
func (n *Node) candidates(tok string) (match *Node, ambiguous []*Node) {
	lt := strings.ToLower(tok)
	var prefix []*Node
	for _, c := range n.Children {
		if c.Arg != nil {
			continue
		}
		w := strings.ToLower(c.Word)
		if w == lt {
			return c, nil
		}
		if strings.HasPrefix(w, lt) && !c.Hidden {
			prefix = append(prefix, c)
		}
	}
	if len(prefix) == 1 {
		return prefix[0], nil
	}
	if len(prefix) > 1 {
		return nil, prefix
	}
	for _, c := range n.Children {
		if c.Arg != nil && (c.Arg.Match == nil || c.Arg.Match(tok)) {
			return c, nil
		}
	}
	return nil, nil
}

// walk parses tokens from the root and returns the final node and values.
func walk(r *Node, line string, toks []token) (*Node, *Args, error) {
	n := r
	a := newArgs()
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		c, amb := n.candidates(t.text)
		if amb != nil {
			return nil, nil, &ParseError{Pos: t.start, Msg: fmt.Sprintf("%% Ambiguous command:  \"%s\"", strings.TrimSpace(line[:t.end]))}
		}
		if c == nil {
			return nil, nil, &ParseError{Pos: t.start, Msg: "% Invalid input detected at '^' marker."}
		}
		if c.Arg != nil {
			val := t.text
			if c.Arg.Rest {
				// The raw remainder keeps spacing intact; a single quoted
				// token is taken without its quotes.
				if i < len(toks)-1 || !strings.HasPrefix(line[t.start:], "\"") {
					val = strings.TrimSpace(line[t.start:])
				}
				i = len(toks)
			}
			a.values[c.Key] = val
			a.multi[c.Key] = append(a.multi[c.Key], val)
		} else {
			a.words[c.Word] = true
		}
		n = c
	}
	return n, a, nil
}

// helpLine is one row of "?" output.
type helpLine struct{ name, help string }

// helpFor lists what may follow the given (partial) line.
func helpFor(s *Session, r *Node, line string, noForm bool) ([]helpLine, error) {
	toks, err := tokenize(line)
	if err != nil {
		return nil, err
	}
	partial := ""
	if len(toks) > 0 && !strings.HasSuffix(line, " ") && !strings.HasSuffix(line, "\t") {
		partial = toks[len(toks)-1].text
		toks = toks[:len(toks)-1]
	}
	n, _, err := walk(r, line, toks)
	if err != nil {
		return nil, err
	}
	if n.Arg != nil && n.Arg.Rest {
		return []helpLine{{"<cr>", ""}}, nil
	}
	var out []helpLine
	lp := strings.ToLower(partial)
	for _, c := range n.Children {
		if c.Hidden {
			continue
		}
		if c.Arg != nil {
			if partial == "" || (c.Arg.Match != nil && c.Arg.Match(partial)) || c.Arg.Rest {
				out = append(out, helpLine{c.Arg.Name, c.Help})
			}
			continue
		}
		if noForm && c.NoRun == nil && !hasNo(c) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(c.Word), lp) {
			out = append(out, helpLine{c.Word, c.Help})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ai, aj := strings.HasPrefix(out[i].name, "<") || isUpper(out[i].name), strings.HasPrefix(out[j].name, "<") || isUpper(out[j].name)
		if ai != aj {
			return !ai
		}
		return out[i].name < out[j].name
	})
	if partial == "" && ((!noForm && n.Run != nil) || (noForm && n.NoRun != nil)) && n != r {
		out = append(out, helpLine{"<cr>", ""})
	}
	return out, nil
}

func isUpper(s string) bool { return s != "" && strings.ToUpper(s) == s && strings.ToLower(s) != s }

// hasNo reports whether a subtree contains any no-form.
func hasNo(n *Node) bool {
	if n.NoRun != nil {
		return true
	}
	for _, c := range n.Children {
		if c != n && hasNo(c) {
			return true
		}
	}
	return false
}

// complete returns completions for the last (partial) token of line.
func complete(s *Session, r *Node, line string) []string {
	toks, err := tokenize(line)
	if err != nil {
		return nil
	}
	partial := ""
	if len(toks) > 0 && !strings.HasSuffix(line, " ") {
		partial = toks[len(toks)-1].text
		toks = toks[:len(toks)-1]
	}
	n, _, err := walk(r, line, toks)
	if err != nil {
		return nil
	}
	var out []string
	lp := strings.ToLower(partial)
	for _, c := range n.Children {
		if c.Hidden {
			continue
		}
		if c.Arg != nil {
			if c.Arg.Complete != nil {
				for _, v := range c.Arg.Complete(s, partial) {
					if strings.HasPrefix(strings.ToLower(v), lp) {
						out = append(out, v)
					}
				}
			}
			continue
		}
		if strings.HasPrefix(strings.ToLower(c.Word), lp) {
			out = append(out, c.Word)
		}
	}
	sort.Strings(out)
	return dedupe(out)
}

func dedupe(s []string) []string {
	var out []string
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

// commonPrefix returns the longest common prefix of the strings.
func commonPrefix(s []string) string {
	if len(s) == 0 {
		return ""
	}
	p := s[0]
	for _, v := range s[1:] {
		for !strings.HasPrefix(strings.ToLower(v), strings.ToLower(p)) {
			p = p[:len(p)-1]
		}
	}
	return p
}
