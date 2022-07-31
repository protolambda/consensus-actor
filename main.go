package main

import (
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/protolambda/consensus-actor/flags"
	"github.com/protolambda/consensus-actor/yolo"
	"github.com/urfave/cli"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))

	app := cli.NewApp()
	app.Flags = flags.Flags
	app.Version = "0.0.1"
	app.Name = "consensus-actor"
	app.Usage = "Start consensus actor analysis server"
	app.Description = "Spins up a server that loads block data, processes it, builds tiles, and serves a large-scale map of consensus actor behavior."

	app.Action = ServerMain
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
	log.LvlFilterHandler(lvl, handler)
	logger := log.New()
	logger.SetHandler(handler)
	return logger, nil
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
}
