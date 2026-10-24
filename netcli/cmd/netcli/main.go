// Command netcli is a Cisco IOS style configuration shell and web service
// for NetBSD routers.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/syslog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/netcli/netcli/internal/cli"
	"github.com/netcli/netcli/internal/config"
	"github.com/netcli/netcli/internal/manager"
	"github.com/netcli/netcli/internal/stats"
	"github.com/netcli/netcli/internal/system"
	"github.com/netcli/netcli/internal/web"
)

var version = "0.1.0"

const usage = `netcli - Cisco style network configuration for NetBSD

Usage:
  netcli [global options] [command] [arguments]

Commands:
  shell                 interactive CLI (default)
  exec "cmd" ...        run CLI commands in privileged mode and exit
                        (several commands can be separated with ';')
  apply                 apply the startup configuration (at boot)
  serve                 run the web service
  hash-password         print a password hash for the YAML file
  version               print the version

Global options:
`

type traceSetter interface{ SetTrace(system.Tracer) }

type globals struct {
	paths   system.Paths
	dryRun  bool
	sandbox string
	exec    system.Executor
	mgr     *manager.Manager
	raw     []string // global flags as given, passed on to child processes
}

func main() {
	g := &globals{paths: system.DefaultPaths()}
	fs := flag.NewFlagSet("netcli", flag.ExitOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage); fs.PrintDefaults() }
	fs.StringVar(&g.paths.EtcDir, "etc", g.paths.EtcDir, "configuration directory (startup.yaml, TLS files)")
	fs.StringVar(&g.paths.RunDir, "run", g.paths.RunDir, "runtime directory (running config, generated files)")
	fs.StringVar(&g.paths.DHCPLeases, "leases", g.paths.DHCPLeases, "dhcpd lease database")
	fs.BoolVar(&g.dryRun, "dry-run", !system.IsNetBSD(), "log system commands instead of running them (default on non-NetBSD)")
	fs.StringVar(&g.sandbox, "sandbox", "", "directory receiving files written in dry-run mode (default <run>/dryrun)")
	fs.Parse(os.Args[1:])
	fs.Visit(func(f *flag.Flag) { g.raw = append(g.raw, "-"+f.Name+"="+f.Value.String()) })

	if g.dryRun {
		sb := g.sandbox
		if sb == "" {
			sb = filepath.Join(g.paths.RunDir, "dryrun")
		}
		g.exec = &system.Dry{Sandbox: sb}
	} else {
		g.exec = &system.Real{}
	}
	g.mgr = manager.New(g.paths, g.exec)

	args := fs.Args()
	cmd := "shell"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "shell":
		err = runShell(g)
	case "exec", "-c":
		err = runExec(g, args)
	case "apply":
		err = runApply(g)
	case "serve":
		err = runServe(g, args)
	case "revert-timer":
		err = runRevertTimer(g, args)
	case "hash-password":
		err = runHash()
	case "version":
		fmt.Println("netcli", version)
	default:
		fs.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "netcli:", err)
		os.Exit(1)
	}
}

func (g *globals) newSession(out *os.File) *cli.Session {
	s := &cli.Session{
		Mgr:        g.mgr,
		Out:        out,
		Version:    version,
		RevertArgs: g.raw,
		Sample:     stats.Read,
	}
	if ts, ok := g.exec.(traceSetter); ok {
		s.SetTrace = func(f func(string)) { ts.SetTrace(f) }
	}
	return s
}

func runShell(g *globals) error {
	s := g.newSession(os.Stdout)
	c, err := g.mgr.Running()
	if err != nil {
		return err
	}
	// Like IOS: start in user EXEC mode when an enable secret protects
	// privileged mode, otherwise go straight to privileged EXEC.
	s.Mode = cli.ModeExec
	if c.EnableSecret != "" {
		s.Mode = cli.ModeUser
	}
	if g.dryRun {
		s.DebugApply = true
		if term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Println("netcli: DRY RUN - system commands are shown, not executed. Generated files go to the sandbox.")
		}
	}
	if s.SetTrace != nil && s.DebugApply {
		ts := g.exec.(traceSetter)
		ts.SetTrace(func(l string) { fmt.Fprintf(s.Out, "  %s\n", l) })
		defer ts.SetTrace(nil)
	}
	if !g.dryRun && os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "netcli: warning: not running as root, configuration changes will fail")
	}
	return cli.Interactive(s, os.Stdin, os.Stdout)
}

// runExec runs commands non-interactively, e.g.
//
//	netcli exec "show ip interface brief"
//	netcli exec "conf t; interface wm1; ip address 192.0.2.1 255.255.255.0; end; write"
func runExec(g *globals, args []string) error {
	s := g.newSession(os.Stdout)
	s.Mode = cli.ModeExec
	if ts, ok := g.exec.(traceSetter); ok && g.dryRun {
		ts.SetTrace(func(l string) { fmt.Fprintf(os.Stdout, "  %s\n", l) })
	}
	ok := true
	for _, a := range args {
		for _, line := range strings.Split(a, ";") {
			if !s.Execute(line, -1) {
				ok = false
			}
			if s.Quit() {
				break
			}
		}
	}
	if !ok {
		os.Exit(1)
	}
	return nil
}

func runApply(g *globals) error {
	if ts, ok := g.exec.(traceSetter); ok {
		ts.SetTrace(func(l string) { fmt.Println(" ", l) })
	}
	err := g.mgr.Boot()
	if err != nil {
		return fmt.Errorf("applying startup configuration:\n%w", err)
	}
	fmt.Println("netcli: startup configuration applied")
	return nil
}

func runRevertTimer(g *globals, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: revert-timer <unix-time>")
	}
	ts, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return err
	}
	if w, err := syslog.New(syslog.LOG_NOTICE|syslog.LOG_DAEMON, "netcli"); err == nil {
		log.SetOutput(w)
		log.SetFlags(0)
	}
	signal.Ignore(syscall.SIGHUP)
	err = g.mgr.RunRevertWatchdog(time.Unix(ts, 0))
	if err != nil {
		log.Printf("rollback failed: %v", err)
	} else {
		log.Printf("rollback timer expired: configuration restored (or confirmed)")
	}
	return err
}

func runHash() error {
	fd := int(os.Stdin.Fd())
	var pw string
	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, "Password: ")
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return err
		}
		pw = string(b)
	} else {
		var line string
		fmt.Fscanln(os.Stdin, &line)
		pw = line
	}
	if pw == "" {
		return errors.New("empty password")
	}
	h, err := config.HashSecret(pw)
	if err != nil {
		return err
	}
	fmt.Println(h)
	return nil
}

func runServe(g *globals, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	listen := fs.String("listen", "", "override the listen address (e.g. 127.0.0.1:8080)")
	tlsFlag := fs.String("tls", "", "override TLS: on or off")
	noAuth := fs.Bool("no-auth", false, "disable login (development only, forces a loopback address)")
	interval := fs.Duration("interval", 2*time.Second, "statistics sampling interval")
	pidfile := fs.String("pidfile", "", "write the process id to this file")
	useSyslog := fs.Bool("syslog", false, "log to syslog")
	fs.Parse(args)

	if *useSyslog {
		if w, err := syslog.New(syslog.LOG_NOTICE|syslog.LOG_DAEMON, "netcli-web"); err == nil {
			log.SetOutput(w)
			log.SetFlags(0)
		}
	}
	c, err := g.mgr.Running()
	if err != nil {
		return err
	}
	wc := c.Web
	if *listen == "" && !wc.Enabled {
		return web.ErrDisabled
	}
	switch *tlsFlag {
	case "on":
		wc.TLS = true
	case "off":
		wc.TLS = false
	}
	addr := web.ListenAddr(wc)
	if *listen != "" {
		addr = *listen
	}
	if *noAuth && !strings.HasPrefix(addr, "127.0.0.1:") && !strings.HasPrefix(addr, "[::1]:") && !strings.HasPrefix(addr, "localhost:") {
		return errors.New("-no-auth is only allowed on a loopback address such as 127.0.0.1:8080")
	}
	if *pidfile != "" {
		if err := os.WriteFile(*pidfile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
			return err
		}
		defer os.Remove(*pidfile)
	}

	col := stats.NewCollector(*interval, int((10*time.Minute) / *interval))
	stop := make(chan struct{})
	go col.Run(stop)
	defer close(stop)

	srv := &web.Server{Mgr: g.mgr, Collector: col, Version: version, NoAuth: *noAuth, Secure: wc.TLS}
	hs := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if wc.TLS {
		cert, err := web.EnsureCertificate(g.paths.EtcDir, c.Hostname)
		if err != nil {
			return fmt.Errorf("TLS certificate: %w", err)
		}
		hs.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-ctx.Done()
		sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		hs.Shutdown(sctx)
	}()

	scheme := "http"
	if wc.TLS {
		scheme = "https"
	}
	log.Printf("netcli web service listening on %s://%s", scheme, addr)
	if wc.TLS {
		err = hs.ListenAndServeTLS("", "")
	} else {
		err = hs.ListenAndServe()
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
