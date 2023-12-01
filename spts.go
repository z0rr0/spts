package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/z0rr0/spts/client"
	"github.com/z0rr0/spts/common"
	"github.com/z0rr0/spts/server"
)

var (
	// Version is program git version
	Version = ""
	// Revision is revision number
	Revision = ""
	// BuildDate is build date
	BuildDate = ""
	// GoVersion is runtime Go language version
	GoVersion = runtime.Version()
)

func main() {
	var (
		serverMode bool
		debug      bool
		version    bool
		dot        bool
		port       uint64 = 28082
		host              = "localhost"
		timeout           = 3 * time.Second
		clients           = 1
	)

	defer func() {
		if r := recover(); r != nil {
			slog.Error("abnormal termination", "version", Version, "error", r)
		}
	}()

	flag.BoolVar(&serverMode, "server", serverMode, "run in server mode")
	flag.Uint64Var(&port, "port", port, "port to listen on"+fmt.Sprintf(" (in range 1..%d)", common.MaxPortNumber))
	flag.DurationVar(&timeout, "timeout", timeout, "timeout for requests (double value for client)")
	flag.StringVar(&host, "host", host, "host (listen on for server, connect to for client)")
	flag.BoolVar(&version, "version", version, "print version and exit")
	flag.BoolVar(&debug, "debug", debug, "enable debug mode")
	flag.BoolVar(&dot, "dot", dot, "show dot progress output (for client mode)")
	flag.IntVar(&clients, "clients", clients, "max clients (for server mode)")

	flag.Parse()
	if version {
		fmt.Printf(
			"Version:   %-20s\nRevision:  %-20s\nBuildDate: %-20s\nGo:        %-20s\n",
			Version, Revision, BuildDate, GoVersion,
		)
		return
	}

	initLogger(debug)
	slog.Debug(
		"starting",
		"version", Version, "revision", Revision, "go", GoVersion, "buildDate", BuildDate,
		"serverMode", serverMode, "host", host, "port", port, "clients", clients, "timeout", timeout,
	)

	ctx, cancel := context.WithCancel(context.Background())

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Signal(syscall.SIGTERM), os.Signal(syscall.SIGQUIT))

	go func() {
		signalValue := <-sigint
		slog.Info("signal received", "signal", signalValue)
		cancel()
	}()

	params := &common.Params{Host: host, Port: port, Timeout: timeout, Clients: clients, Dot: dot}
	if err := start(ctx, serverMode, params); err != nil {
		slog.Error("processing", "error", err)
		os.Exit(1)
	}
}

func initLogger(debug bool) {
	var level = slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}

func start(ctx context.Context, serverMode bool, params *common.Params) error {
	var (
		s   common.Starter
		err error
	)

	if serverMode {
		s, err = server.New(params)
	} else {
		params.Timeout *= 2
		s, err = client.New(params)
	}

	if err != nil {
		slog.Error("start", "error", err)
		return err
	}

	return s.Start(ctx)
}
