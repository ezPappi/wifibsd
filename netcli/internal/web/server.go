// Package web is the netcli web service: a JSON API, a server-sent events
// stream of live statistics and an embedded single page UI. All changes go
// through the same manager as the CLI, so both always agree.
package web

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/netip"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/netcli/netcli/internal/cli"
	"github.com/netcli/netcli/internal/config"
	"github.com/netcli/netcli/internal/manager"
	"github.com/netcli/netcli/internal/stats"
	"github.com/netcli/netcli/internal/system"
)

//go:embed static
var staticFS embed.FS

// Server is the web service.
type Server struct {
	Mgr       *manager.Manager
	Collector *stats.Collector
	Version   string
	// NoAuth disables authentication (development only).
	NoAuth bool
	Secure bool // set Secure on cookies (TLS)

	mu       sync.Mutex
	sessions map[string]*websession
	attempts map[string][]time.Time
}

type websession struct {
	user    string
	expires time.Time
	cli     *cli.Session
	out     bytes.Buffer
	mu      sync.Mutex
}

const (
	cookieName  = "netcli_session"
	sessionTTL  = 12 * time.Hour
	csrfHeader  = "X-Netcli"
	maxBodySize = 1 << 20
)

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	s.sessions = map[string]*websession{}
	s.attempts = map[string][]time.Time{}
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /", http.FileServer(http.FS(static)))

	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.auth(s.logout))
	mux.HandleFunc("GET /api/session", s.auth(s.session))
	mux.HandleFunc("GET /api/status", s.auth(s.status))
	mux.HandleFunc("GET /api/stats/history", s.auth(s.history))
	mux.HandleFunc("GET /api/stats/stream", s.auth(s.stream))
	mux.HandleFunc("GET /api/config", s.auth(s.getConfig))
	mux.HandleFunc("PUT /api/config", s.auth(s.putConfig))
	mux.HandleFunc("POST /api/config/validate", s.auth(s.validateConfig))
	mux.HandleFunc("POST /api/config/save", s.auth(s.saveConfig))
	mux.HandleFunc("POST /api/cli", s.auth(s.cliExec))
	mux.HandleFunc("POST /api/cli/help", s.auth(s.cliHelp))
	mux.HandleFunc("POST /api/cli/complete", s.auth(s.cliComplete))
	return securityHeaders(mux)
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, maxBodySize)).Decode(v)
}

// ---- authentication -----------------------------------------------------------

func (s *Server) lookup(r *http.Request) *websession {
	if s.NoAuth {
		s.mu.Lock()
		defer s.mu.Unlock()
		ws, ok := s.sessions["noauth"]
		if !ok {
			ws = s.newSession("admin")
			s.sessions["noauth"] = ws
		}
		return ws
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ws, ok := s.sessions[c.Value]
	if !ok || time.Now().After(ws.expires) {
		delete(s.sessions, c.Value)
		return nil
	}
	return ws
}

func (s *Server) newSession(user string) *websession {
	ws := &websession{user: user, expires: time.Now().Add(sessionTTL)}
	ws.cli = &cli.Session{Mgr: s.Mgr, Out: &ws.out, Mode: cli.ModeExec, Version: s.Version}
	ws.cli.Sample = func() (*stats.Sample, error) { return s.Collector.Latest() }
	return ws
}

// auth wraps handlers that need a logged in user. State changing requests
// must also carry the X-Netcli header: browsers cannot add custom headers
// to cross-site form posts, which blocks CSRF.
func (s *Server) auth(h func(http.ResponseWriter, *http.Request, *websession)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws := s.lookup(r)
		if ws == nil {
			writeErr(w, http.StatusUnauthorized, "login required")
			return
		}
		if r.Method != http.MethodGet && r.Header.Get(csrfHeader) != "1" {
			writeErr(w, http.StatusForbidden, "missing "+csrfHeader+" header")
			return
		}
		h(w, r, ws)
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// throttled limits failed logins to 5 per minute per address.
func (s *Server) throttled(ip string, failed bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var recent []time.Time
	for _, t := range s.attempts[ip] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	if failed {
		recent = append(recent, now)
	}
	s.attempts[ip] = recent
	return len(recent) >= 5
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(csrfHeader) != "1" {
		writeErr(w, http.StatusForbidden, "missing "+csrfHeader+" header")
		return
	}
	ip := clientIP(r)
	if s.throttled(ip, false) {
		writeErr(w, http.StatusTooManyRequests, "too many failed attempts, wait a minute")
		return
	}
	var req struct{ Username, Password string }
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	c, err := s.Mgr.Running()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(c.Users) == 0 {
		writeErr(w, http.StatusForbidden, "no users configured; add one in the CLI: username admin secret <password>")
		return
	}
	hash, ok := c.Users[req.Username]
	if !ok {
		hash = "pbkdf2-sha256$120000$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" // equal work for unknown users
	}
	if !config.CheckSecret(hash, req.Password) || !ok {
		s.throttled(ip, true)
		log.Printf("web: failed login for %q from %s", req.Username, ip)
		writeErr(w, http.StatusUnauthorized, "invalid user name or password")
		return
	}
	tok := make([]byte, 32)
	rand.Read(tok)
	id := hex.EncodeToString(tok)
	ws := s.newSession(req.Username)
	s.mu.Lock()
	s.sessions[id] = ws
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: id, Path: "/", HttpOnly: true,
		Secure: s.Secure, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionTTL.Seconds())})
	log.Printf("web: %s logged in from %s", req.Username, ip)
	writeJSON(w, http.StatusOK, map[string]string{"user": req.Username})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, ws *websession) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) session(w http.ResponseWriter, r *http.Request, ws *websession) {
	writeJSON(w, http.StatusOK, map[string]any{"user": ws.user, "noauth": s.NoAuth})
}

// ---- status & statistics ------------------------------------------------------

type ifStatus struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	Description string   `json:"description,omitempty"`
	Present     bool     `json:"present"`
	Up          bool     `json:"up"`
	Running     bool     `json:"running"`
	Shutdown    bool     `json:"shutdown"`
	MAC         string   `json:"mac,omitempty"`
	MTU         int      `json:"mtu,omitempty"`
	Addresses   []string `json:"addresses"`
	Configured  bool     `json:"configured"`
	DHCP        bool     `json:"dhcp,omitempty"`
	ACLIn       string   `json:"acl_in,omitempty"`
	ACLOut      string   `json:"acl_out,omitempty"`
	Wireless    string   `json:"wireless,omitempty"`
	RxBytes     uint64   `json:"rx_bytes"`
	TxBytes     uint64   `json:"tx_bytes"`
	RxErrs      uint64   `json:"rx_errs"`
	TxErrs      uint64   `json:"tx_errs"`
}

func (s *Server) status(w http.ResponseWriter, r *http.Request, ws *websession) {
	c, err := s.Mgr.Running()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	startup, _ := s.Mgr.Startup()
	sample, _ := s.Collector.Latest()

	byName := map[string]*ifStatus{}
	if ifs, err := net.Interfaces(); err == nil {
		for _, ni := range ifs {
			st := &ifStatus{Name: ni.Name, Kind: config.InterfaceKind(ni.Name), Present: true,
				Up: ni.Flags&net.FlagUp != 0, Running: ni.Flags&net.FlagRunning != 0,
				MTU: ni.MTU, Addresses: []string{}}
			if len(ni.HardwareAddr) > 0 {
				st.MAC = ni.HardwareAddr.String()
			}
			if addrs, err := ni.Addrs(); err == nil {
				for _, a := range addrs {
					if p, err := netip.ParsePrefix(a.String()); err == nil && p.Addr().IsLinkLocalUnicast() {
						continue
					}
					st.Addresses = append(st.Addresses, a.String())
				}
			}
			byName[ni.Name] = st
		}
	}
	for n, ic := range c.Interfaces {
		st, ok := byName[n]
		if !ok {
			st = &ifStatus{Name: n, Kind: config.InterfaceKind(n), Addresses: []string{}}
			byName[n] = st
		}
		st.Configured = true
		st.Description = ic.Description
		st.Shutdown = ic.Shutdown
		st.DHCP = ic.DHCP
		st.ACLIn, st.ACLOut = ic.AccessGroupIn, ic.AccessGroupOut
		if ic.Wireless != nil {
			st.Wireless = ic.Wireless.Mode + " " + ic.Wireless.SSID
			if !ic.Wireless.Ready(config.InterfaceKind(n)) {
				st.Wireless += " (incomplete: " + ic.Wireless.Missing(config.InterfaceKind(n)) + ")"
			}
		}
	}
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	config.SortInterfaceNames(names)
	list := make([]*ifStatus, 0, len(names))
	for _, n := range names {
		st := byName[n]
		if sample != nil {
			if cnt, ok := sample.Ifaces[n]; ok {
				st.RxBytes, st.TxBytes, st.RxErrs, st.TxErrs = cnt.RxBytes, cnt.TxBytes, cnt.RxErrs, cnt.TxErrs
			}
		}
		list = append(list, st)
	}

	resp := map[string]any{
		"hostname":   c.Hostname,
		"version":    s.Version,
		"platform":   runtime.GOOS + "/" + runtime.GOARCH,
		"system":     system.Uname(),
		"uptime":     system.Uptime(),
		"dry_run":    s.Mgr.Applier.Exec.DryRun(),
		"unsaved":    !config.Equal(c, startup),
		"interfaces": list,
		"interval":   s.Collector.Interval.Seconds(),
	}
	if d, ok := s.Mgr.RevertPending(); ok {
		resp["revert_at"] = d.UnixMilli()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request, ws *websession) {
	writeJSON(w, http.StatusOK, s.Collector.History())
}

// stream sends a server-sent event for every new statistics point.
func (s *Server) stream(w http.ResponseWriter, r *http.Request, ws *websession) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	ch, unsub := s.Collector.Subscribe()
	defer unsub()
	fmt.Fprint(w, ": connected\n\n")
	fl.Flush()
	keep := time.NewTicker(25 * time.Second)
	defer keep.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case p := <-ch:
			data, _ := json.Marshal(p)
			fmt.Fprintf(w, "data: %s\n\n", data)
			fl.Flush()
		case <-keep.C:
			fmt.Fprint(w, ": keepalive\n\n")
			fl.Flush()
		}
	}
}

// ---- configuration ------------------------------------------------------------------

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request, ws *websession) {
	var c *config.Config
	var err error
	present := true
	switch r.URL.Query().Get("which") {
	case "startup":
		c, present, err = s.Mgr.Store.Startup()
	default:
		c, err = s.Mgr.Running()
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	y, err := config.Marshal(c)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{"yaml": string(y), "cli": cli.ConfigText(c, ""), "present": present}
	if r.URL.Query().Get("which") == "startup" {
		if run, err := s.Mgr.Running(); err == nil {
			resp["diff"] = manager.Diff(cli.ConfigText(c, ""), cli.ConfigText(run, ""))
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type configReq struct {
	YAML string `json:"yaml"`
}

func parseConfigReq(r *http.Request) (*config.Config, error) {
	var req configReq
	if err := readJSON(r, &req); err != nil {
		return nil, fmt.Errorf("invalid request")
	}
	c, err := config.Unmarshal([]byte(req.YAML))
	if err != nil {
		return nil, fmt.Errorf("YAML: %v", err)
	}
	return c, nil
}

func (s *Server) validateConfig(w http.ResponseWriter, r *http.Request, ws *websession) {
	c, err := parseConfigReq(r)
	if err == nil {
		err = c.Validate()
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	cur, _ := s.Mgr.Running()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true,
		"diff": manager.Diff(cli.ConfigText(cur, ""), cli.ConfigText(c, ""))})
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request, ws *websession) {
	c, err := parseConfigReq(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.Mgr.Replace(c); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	log.Printf("web: %s replaced the running configuration", ws.user)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) saveConfig(w http.ResponseWriter, r *http.Request, ws *websession) {
	if err := s.Mgr.Save(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("web: %s saved the configuration", ws.user)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- web terminal ----------------------------------------------------------------------

type cliReq struct {
	Line string `json:"line"`
}

func (s *Server) cliExec(w http.ResponseWriter, r *http.Request, ws *websession) {
	var req cliReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.out.Reset()
	for _, line := range strings.Split(req.Line, "\n") {
		prompt := ws.cli.Prompt()
		ws.out.WriteString(prompt + line + "\n")
		ws.cli.Execute(line, len(prompt))
		if ws.cli.Quit() {
			ws.cli = s.newSession(ws.user).cli
			ws.cli.Out = &ws.out
			ws.out.WriteString("(session restarted)\n")
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"output": ws.out.String(), "prompt": ws.cli.Prompt()})
}

func (s *Server) cliHelp(w http.ResponseWriter, r *http.Request, ws *websession) {
	var req cliReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"output": ws.cli.Help(req.Line), "prompt": ws.cli.Prompt()})
}

func (s *Server) cliComplete(w http.ResponseWriter, r *http.Request, ws *websession) {
	var req cliReq
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"options": ws.cli.Complete(req.Line)})
}

// ErrDisabled is returned by Serve when the web service is not enabled.
var ErrDisabled = errors.New("the web service is disabled; enable it with 'ip http secure-server' or 'ip http server'")
