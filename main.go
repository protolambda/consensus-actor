package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ethereum/go-ethereum/log"
	"github.com/protolambda/consensus-actor/flags"
	"github.com/protolambda/consensus-actor/yolo"
	"github.com/urfave/cli"
)

func main() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))

	app := cli.NewApp()
	app.Flags = flags.GlobalFlags
	app.Version = "0.0.2"
	app.Name = "consensus-actor"
	app.Usage = "Consensus actor analysis tool by @protolambda"
	app.Description = "Build and serve a maps-like view of the consensus actor data of ethereum."
	app.Commands = []cli.Command{
		{
			Name:        "import",
			Usage:       "Import blocks from Lighthouse",
			Description: "Imports blocks from a lighthouse DB (db must not be locked, i.e. lighthouse must not be running).",
			Action:      ImportMain,
			Flags:       flags.ImportFlags,
		},
		{
			Name:        "server",
			Usage:       "Serve app and sync live data",
			Description: "Spins up a server that loads block data, processes it, builds tiles, and serves a large-scale map of consensus actor behavior.",
			Action:      ServerMain,
			Flags:       flags.ServerFlags,
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Crit("Failed to start server", "err", err)
	}
}

func SetupLogger(ctx *cli.Context) (log.Logger, error) {
	fmtStr := ctx.GlobalString(flags.LogFormatFlag.Name)
	lvlStr := ctx.GlobalString(flags.LogLevelFlag.Name)

	var logFmt log.Format
	switch fmtStr {
	case "json":
		logFmt = log.JSONFormat()
	case "json-pretty":
		logFmt = log.JSONFormatEx(true, true)
	case "text", "terminal":
		logFmt = log.TerminalFormat(ctx.GlobalBool(flags.LogColorFlag.Name))
	default:
		return nil, fmt.Errorf("unrecognized log format: %q", fmtStr)
	}

	lvl, err := log.LvlFromString(strings.ToLower(lvlStr))
	if err != nil {
		return nil, fmt.Errorf("unrecognized log level: %w", err)
	}
	handler := log.StreamHandler(os.Stdout, logFmt)
	handler = log.SyncHandler(handler)
	handler = log.LvlFilterHandler(lvl, handler)
	logger := log.New()
	logger.SetHandler(handler)
	return logger, nil
}

func ImportMain(clictx *cli.Context) {
	logger, err := SetupLogger(clictx)
	if err != nil {
		log.Crit("failed to setup logger", "err", err) // os exit 1
		return
	}
	imp, err := yolo.NewImporter(clictx, logger)
	if err != nil {
		logger.Crit("failed to create importer", "err", err) // os exit 1
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error)
	go func() {
		err := imp.Run(ctx)
		runDone <- err
	}()

	interruptChannel := make(chan os.Signal, 1)
	signal.Notify(interruptChannel, []os.Signal{
		os.Interrupt,
		os.Kill,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	}...)
	go func() {
		<-interruptChannel
		logger.Info("closing on interrupt signal")
		cancel()
	}()

	err = <-runDone
	if err != nil {
		logger.Error("stopped import with error", "err", err)
	} else {
		logger.Info("import completed")
	}

	if err := imp.Close(); err != nil {
		logger.Crit("shutdown error", "err", err) // os exit 1
	}
	logger.Info("goodbye")
}

func ServerMain(ctx *cli.Context) {
	logger, err := SetupLogger(ctx)
	if err != nil {
		log.Crit("failed to setup logger", "err", err) // os exit 1
		return
	}
	server, err := yolo.NewServer(ctx, logger)
	if err != nil {
		logger.Crit("failed to create server", "err", err) // os exit 1
		return
	}
	go server.Run()

	interruptChannel := make(chan os.Signal, 1)
	signal.Notify(interruptChannel, []os.Signal{
		os.Interrupt,
		os.Kill,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	}...)
	<-interruptChannel

	if err := server.Close(); err != nil {
		logger.Crit("shutdown error", "err", err) // os exit 1
	}
	logger.Info("goodbye")
}
