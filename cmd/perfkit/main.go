package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/flaticols/perfkit/internal/capture"
	"github.com/flaticols/perfkit/internal/config"
	"github.com/flaticols/perfkit/internal/models"
	"github.com/flaticols/perfkit/internal/server"
	"github.com/flaticols/perfkit/internal/storage"
	"github.com/jessevdk/go-flags"
)

type Options struct {
	Config     string        `short:"c" long:"config" description:"Config file path"`
	Server     ServerCmd     `command:"server" alias:"s" description:"Start the collector server"`
	Capture    CaptureCmd    `command:"capture" description:"Capture profiles from a pprof endpoint"`
	Quickstart QuickstartCmd `command:"quickstart" alias:"q" description:"Show getting started guide"`
	Session    SessionCmd    `command:"session" description:"Manage sessions"`
	Get        GetCmd        `command:"get" description:"Get a profile from a session"`
}

type ServerCmd struct {
	Host  string `short:"H" long:"host" description:"Server host" default:"localhost"`
	Port  int    `short:"p" long:"port" description:"Server port" default:"8080"`
	Pprof bool   `long:"pprof" description:"Enable pprof endpoints for self-profiling"`
}

func (c *ServerCmd) Execute(args []string) error {
	return runServer(c)
}

type CaptureCmd struct {
	Profiles    string        `short:"p" long:"profiles" description:"Comma-separated profiles to capture (cpu,heap,goroutine,block,mutex,allocs,threadcreate)" default:"all"`
	Interval    time.Duration `short:"i" long:"interval" description:"Capture interval for periodic mode (e.g., 30s, 1m)"`
	CPUDuration time.Duration `long:"cpu-duration" description:"CPU profile duration" default:"30s"`
	Session     string        `short:"s" long:"session" description:"Session name for grouping profiles"`
	Project     string        `long:"project" description:"Project name"`
	Server      string        `long:"server" description:"Perfkit server URL" default:"http://localhost:8080"`
	Count       int           `short:"n" long:"count" description:"Number of captures in interval mode (0=infinite)" default:"0"`
	Args        struct {
		Target string `positional-arg-name:"target" description:"Target pprof URL (e.g., http://localhost:6060)"`
	} `positional-args:"yes" required:"yes"`
}

func (c *CaptureCmd) Execute(args []string) error {
	return runCapture(c)
}

type QuickstartCmd struct{}

func (c *QuickstartCmd) Execute(args []string) error {
	fmt.Print(quickstartGuide)
	return nil
}

type SessionCmd struct {
	Ls       SessionLsCmd       `command:"ls" description:"List all sessions"`
	Profiles SessionProfilesCmd `command:"profiles" description:"List profiles in a session"`
}

type SessionLsCmd struct{}

func (c *SessionLsCmd) Execute(args []string) error {
	return runSessionLs()
}

type SessionProfilesCmd struct {
	Args struct {
		SessionName string `positional-arg-name:"session" description:"Session name" required:"yes"`
	} `positional-args:"yes" required:"yes"`
}

func (c *SessionProfilesCmd) Execute(args []string) error {
	return runSessionProfiles(c.Args.SessionName)
}

type GetCmd struct {
	Raw  bool `long:"raw" description:"Return raw profile data"`
	Args struct {
		SessionName string `positional-arg-name:"session" description:"Session name" required:"yes"`
		ProfileID   string `positional-arg-name:"profile_id" description:"Profile ID" required:"yes"`
	} `positional-args:"yes" required:"yes"`
}

func (c *GetCmd) Execute(args []string) error {
	return runGet(c.Args.SessionName, c.Args.ProfileID, c.Raw)
}

const quickstartGuide = `
PERFKIT QUICKSTART
==================

perfkit is a performance data collector and viewer for Go pprof profiles 
and k6 load test results.


STEP 1: ENABLE PPROF IN YOUR GO APP
-----------------------------------

Add this import to expose pprof endpoints:

    import _ "net/http/pprof"

    func main() {
        // Start pprof server on a separate port
        go func() {
            http.ListenAndServe("localhost:6060", nil)
        }()

        // ... your application code
    }

Your app will expose profiles at http://localhost:6060/debug/pprof/


STEP 2: START PERFKIT SERVER
----------------------------

In one terminal, start the perfkit server:

    perfkit server

Server runs at http://localhost:8080 with web UI for browsing profiles.

Options:
    --port 9090       Use different port
    --pprof           Enable self-profiling endpoints


STEP 3: CAPTURE PROFILES
------------------------

In another terminal, capture profiles from your running app:

    # Capture all profile types once
    perfkit capture http://localhost:6060

    # Capture specific profiles
    perfkit capture http://localhost:6060 --profiles heap,goroutine,cpu

    # Capture with session name (groups profiles together)
    perfkit capture http://localhost:6060 --session load-test

    # Capture periodically every 30 seconds
    perfkit capture http://localhost:6060 --interval 30s --session monitoring

    # Capture 5 times with 10s interval
    perfkit capture http://localhost:6060 --interval 10s --count 5


STEP 4: VIEW AND COMPARE
------------------------

Open http://localhost:8080 in your browser to:

    - Browse all captured profiles by session
    - View profile details and metrics
    - Select multiple profiles of same type to compare
    - See deltas between profiles (memory growth, CPU changes)


PROFILE TYPES
-------------

    cpu          CPU usage (sampled over --cpu-duration, default 30s)
    heap         Memory allocations (current snapshot)
    goroutine    Goroutine stacks (current snapshot)
    block        Blocking operations (cumulative since app start)
    mutex        Mutex contention (cumulative since app start)
    allocs       All allocations (cumulative since app start)
    threadcreate Thread creation stacks


EXAMPLE: DEBUGGING MEMORY LEAK
------------------------------

    # Terminal 1: Start perfkit
    perfkit server

    # Terminal 2: Capture baseline
    perfkit capture http://localhost:6060 --profiles heap --session memleak

    # ... run your load test or reproduce the issue ...

    # Terminal 2: Capture after load
    perfkit capture http://localhost:6060 --profiles heap --session memleak

    # Open http://localhost:8080, select both heap profiles, click Compare
    # See memory growth with exact deltas


EXAMPLE: K6 LOAD TESTING
-------------------------

    # Terminal 1: Start perfkit
    perfkit server

    # Terminal 2: Run k6 test with summary export
    k6 run --summary-export=baseline.json script.js

    # Terminal 2: Ingest baseline
    curl -X POST "http://localhost:8080/api/k6/ingest?session=api-test&name=baseline" \
      --data-binary @baseline.json

    # ... make code changes ...

    # Terminal 2: Run k6 again
    k6 run --summary-export=optimized.json script.js

    # Terminal 2: Ingest optimized version
    curl -X POST "http://localhost:8080/api/k6/ingest?session=api-test&name=optimized" \
      --data-binary @optimized.json

    # Open http://localhost:8080, select both k6 profiles, click Compare
    # See performance improvements: P95, P99, RPS, error rate changes


API ENDPOINTS
-------------

    POST /api/pprof/ingest?type=heap&session=test    Ingest pprof profile
    POST /api/k6/ingest?session=test&name=run1       Ingest k6 summary
    GET  /api/profiles                                List profiles
    GET  /api/profiles/{id}                           Get profile
    GET  /api/profiles/{id}?raw=true                  Download raw data
    GET  /api/profiles/compare?ids=id1,id2            Compare profiles


MORE INFO
---------

    perfkit server --help     Server options
    perfkit capture --help    Capture options

    GitHub: https://github.com/flaticols/perfkit

`

var opts Options

func main() {
	parser := flags.NewParser(&opts, flags.Default)
	parser.CommandHandler = func(command flags.Commander, args []string) error {
		if command == nil {
			parser.WriteHelp(os.Stdout)
			return nil
		}
		return command.Execute(args)
	}

	if _, err := parser.Parse(); err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(1)
	}
}

func runServer(cmd *ServerCmd) error {
	cfg, err := config.Load(opts.Config)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Override config with command line flags
	if cmd.Host != "localhost" {
		cfg.Server.Host = cmd.Host
	}
	if cmd.Port != 8080 {
		cfg.Server.Port = cmd.Port
	}
	cfg.Server.EnablePprof = cmd.Pprof

	if err := cfg.EnsureDataDir(); err != nil {
		return fmt.Errorf("ensure data dir: %w", err)
	}

	store, err := storage.New(cfg.DBPath())
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer store.Close()

	srv := server.New(cfg, store)

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down...")
		cancel()
		srv.Shutdown(ctx)
	}()

	return srv.Start()
}

func runCapture(cmd *CaptureCmd) error {
	if cmd.Args.Target == "" {
		return fmt.Errorf("target URL is required")
	}

	// Parse profile types
	var profiles []models.ProfileType
	if cmd.Profiles == "all" {
		profiles = capture.AllProfiles
	} else {
		for _, p := range strings.Split(cmd.Profiles, ",") {
			pt := models.ProfileType(strings.TrimSpace(p))
			if !pt.IsValid() {
				return fmt.Errorf("invalid profile type: %s", p)
			}
			profiles = append(profiles, pt)
		}
	}

	// Create capturer
	c := capture.New(cmd.Args.Target, cmd.Server)
	c.CPUDuration = cmd.CPUDuration
	c.Session = cmd.Session
	c.Project = cmd.Project

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\nStopping capture...")
		cancel()
	}()

	fmt.Printf("Capturing from %s → %s\n", cmd.Args.Target, cmd.Server)
	if cmd.Session != "" {
		fmt.Printf("Session: %s\n", cmd.Session)
	}
	if cmd.Interval > 0 {
		fmt.Printf("Interval: %s | Profiles: %s\n", cmd.Interval, cmd.Profiles)
	} else {
		fmt.Printf("Profiles: %s\n", cmd.Profiles)
	}
	fmt.Println()

	captureRound := func(round int) bool {
		if round > 0 {
			fmt.Printf("[%s] Capture round %d\n", time.Now().Format("15:04:05"), round)
		} else {
			fmt.Printf("[%s] Capturing profiles...\n", time.Now().Format("15:04:05"))
		}

		for _, pt := range profiles {
			select {
			case <-ctx.Done():
				return false
			default:
			}

			result := c.CaptureAndSend(pt)
			if result.Error != nil {
				fmt.Printf("  ✗ %-12s %v\n", pt, result.Error)
			} else {
				label := "snapshot"
				if pt.IsCumulative() {
					label = "cumulative"
				} else if pt == models.ProfileTypeCPU {
					label = fmt.Sprintf("%s sample", cmd.CPUDuration)
				}
				fmt.Printf("  ✓ %-12s %s  (%s)\n", pt, formatSize(result.Size), label)
			}
		}
		return true
	}

	// Single capture mode
	if cmd.Interval == 0 {
		captureRound(0)
		return nil
	}

	// Interval mode
	round := 1
	ticker := time.NewTicker(cmd.Interval)
	defer ticker.Stop()

	// First capture immediately
	if !captureRound(round) {
		return nil
	}
	round++

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("\nCaptured %d rounds.\n", round-1)
			return nil
		case <-ticker.C:
			if cmd.Count > 0 && round > cmd.Count {
				fmt.Printf("\nCompleted %d captures.\n", cmd.Count)
				return nil
			}
			if !captureRound(round) {
				return nil
			}
			round++
		}
	}
}

func formatSize(bytes int) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := int64(bytes) / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func runSessionLs() error {
	cfg, err := config.Load(opts.Config)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	store, err := storage.New(cfg.DBPath())
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer store.Close()

	ctx := context.Background()
	sessions, err := store.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	for _, session := range sessions {
		fmt.Println(session)
	}
	return nil
}

func runSessionProfiles(sessionName string) error {
	cfg, err := config.Load(opts.Config)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	store, err := storage.New(cfg.DBPath())
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer store.Close()

	ctx := context.Background()
	profiles, err := store.ListProfilesBySession(ctx, sessionName)
	if err != nil {
		return fmt.Errorf("list profiles: %w", err)
	}

	if len(profiles) == 0 {
		fmt.Printf("No profiles found in session %q.\n", sessionName)
		return nil
	}

	for _, p := range profiles {
		fmt.Printf("%s  %-12s  %s  %s\n", p.ID, p.ProfileType, p.CreatedAt.Format("2006-01-02 15:04:05"), p.Name)
	}
	return nil
}

func runGet(sessionName, profileID string, raw bool) error {
	cfg, err := config.Load(opts.Config)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	store, err := storage.New(cfg.DBPath())
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer store.Close()

	ctx := context.Background()
	profile, err := store.GetProfile(ctx, profileID)
	if err != nil {
		return fmt.Errorf("get profile: %w", err)
	}

	// Verify the profile belongs to the specified session
	if profile.Session != sessionName {
		return fmt.Errorf("profile %s does not belong to session %q", profileID, sessionName)
	}

	if raw {
		_, err = os.Stdout.Write(profile.RawData)
		return err
	}

	// Output profile metadata as JSON
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(profile)
}
