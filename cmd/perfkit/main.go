package main

import (
	"context"
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
	Config  string     `short:"c" long:"config" description:"Config file path"`
	Server  ServerCmd  `command:"server" alias:"s" description:"Start the collector server"`
	Capture CaptureCmd `command:"capture" alias:"c" description:"Capture profiles from a pprof endpoint"`
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
