package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/z0rr0/spts/args"
	auth2 "github.com/z0rr0/spts/auth"
	"github.com/z0rr0/spts/config"
	"github.com/z0rr0/spts/db"
	"github.com/z0rr0/spts/user"
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
		debug      bool
		production bool
		version    bool
		cfg        string
		secret     string
		userAction args.UserAction

		port   uint16 = 28082
		host          = "localhost"
		dbFile        = "spts.csv"
	)

	defer func() {
		if r := recover(); r != nil {
			slog.Error("abnormal termination", "version", Version, "error", r)
		}
	}()

	flag.StringVar(&host, "host", host, "host to listen on for server mode or connect to for client mode")
	flag.StringVar(&cfg, "config", cfg, "config file name")
	flag.StringVar(&dbFile, "db", dbFile, "database file name")
	flag.StringVar(&secret, "secret", secret, "secret key")
	flag.BoolVar(&version, "version", version, "print version and exit")
	flag.BoolVar(&debug, "debug", debug, "enable debug mode")
	flag.BoolVar(&production, "production", production, "enable production mode")
	flag.Func("port", args.PortHelp(port), func(s string) error {
		if p, err := args.Port(s); err != nil {
			return err
		} else {
			port = p
		}
		return nil
	})
	flag.Func("user", args.UserModHelp(), func(s string) error {
		if ua, err := args.UserMod(s); err != nil {
			return err
		} else {
			userAction = ua
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

	cmdCfg := &config.Config{
		Host:       host,
		Port:       port,
		Debug:      debug,
		Production: production,
		Database:   dbFile,
		Secret:     secret,
	}
	c, err := config.New(cfg, cmdCfg)
	if err != nil {
		panic(err)
	}

	initLogger(c.Debug)
	slog.Debug(
		"starting",
		"version", Version, "revision", Revision, "go", GoVersion, "buildDate", BuildDate,
		"host", host, "port", port, "debug", debug, "production", production,
		"userAction", userAction, "config", cfg, "dbFile", dbFile,
	)
	slog.Debug("config", "config", c)

	dbStorage, err := db.NewStorage(c.Database)
	if err != nil {
		panic(err)
	}

	authConfig := auth2.Config{
		IsProd:        c.Production,
		GlobalSalt:    c.SaltBytes,
		JWTSecret:     c.SecretBytes,
		TokenDuration: c.TokenDuration.Timed(),
	}

	authenticator := auth2.NewAuthenticator(dbStorage, authConfig)
	if userAction != 0 {
		slog.Debug("user action", "action", userAction)
		if err = user.Action(userAction, authenticator); err != nil {
			panic(err)
		}
		return
	}
}

func initLogger(debug bool) {
	var level = slog.LevelInfo

	if debug {
		level = slog.LevelDebug
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}
