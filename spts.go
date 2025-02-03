package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/z0rr0/spts/auth0/token"
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
		debug   bool
		version bool
		dot     bool

		port     uint16 = 28082
		portHelp        = fmt.Sprintf("port to listen on (integer in range 1..%d)", common.MaxPortNumber)

		host    = "localhost"
		timeout = 3 * time.Second
		clients = 1

		chunk     = token.ChunkSize
		chunkHelp = fmt.Sprintf("chunk size in kilobytes (integer in range 1..%d)", token.MaxChunkSize)
	)

	defer func() {
		if r := recover(); r != nil {
			slog.Error("abnormal termination", "version", Version, "error", r)
		}
	}()

	flag.DurationVar(&timeout, "timeout", timeout, "timeout for requests, run in server mode")
	flag.StringVar(&host, "host", host, "host to listen on for server mode or connect to for client mode")
	flag.BoolVar(&version, "version", version, "print version and exit")
	flag.BoolVar(&debug, "debug", debug, "enable debug mode")
	flag.BoolVar(&dot, "dot", dot, "show dot progress output (for client mode)")
	flag.IntVar(&clients, "clients", clients, "max clients (for server mode)")
	flag.Func("port", portHelp, func(s string) error {
		if p, err := common.ParsePort(s); err != nil {
			return err
		} else {
			port = p
		}
		return nil
	})
	flag.Func("chunk", chunkHelp, func(s string) error {
		if c, err := strconv.ParseUint(s, 10, 32); err != nil {
			return fmt.Errorf("parse chunk size: %w", err)
		} else {
			chunk = uint32(c)
		}
		if chunk < 1 {
			return fmt.Errorf("chunk size must be greater than 0")
		}
		return nil
	})

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
		"serverMode", timeout > 0, "host", host, "port", port, "clients", clients, "timeout", timeout,
	)

	ctx, cancel := context.WithCancel(context.Background())

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Signal(syscall.SIGTERM), os.Signal(syscall.SIGQUIT))

	go func() {
		signalValue := <-sigint
		slog.Info("signal received", "signal", signalValue)
		cancel()
	}()

	params := &common.Params{Host: host, Port: port, Timeout: timeout, Clients: clients, Dot: dot, Chunk: chunk}
	if err := start(ctx, params); err != nil {
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

func start(ctx context.Context, params *common.Params) error {
	var (
		s   common.Starter
		err error
	)

	if params.ServerMode() {
		s, err = server.New(params)
	} else {
		s, err = client.New(params)
	}

	if err != nil {
		slog.Error("start", "error", err)
		return err
	}

	return s.Start(ctx)
}
