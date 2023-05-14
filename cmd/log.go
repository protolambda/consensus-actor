package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"
)

var (
	LogLevelFlag = &cli.StringFlag{
		Name:    "log.level",
		Usage:   "The lowest log level that will be output",
		Value:   "info",
		EnvVars: []string{"LOG_LEVEL"},
	}
	LogFormatFlag = &cli.StringFlag{
		Name:    "log.format",
		Usage:   "Format the log output. Supported formats: 'text', 'json'",
		Value:   "text",
		EnvVars: []string{"LOG_FORMAT"},
	}
	LogColorFlag = &cli.BoolFlag{
		Name:    "log.color",
		Usage:   "Color the log output",
		EnvVars: []string{"LOG_COLOR"},
	}
)

func SetupLogger(ctx *cli.Context) (log.Logger, error) {
	fmtStr := ctx.String(LogFormatFlag.Name)
	lvlStr := ctx.String(LogLevelFlag.Name)

	var logFmt log.Format
	switch fmtStr {
	case "json":
		logFmt = log.JSONFormat()
	case "json-pretty":
		logFmt = log.JSONFormatEx(true, true)
	case "text", "terminal":
		logFmt = log.TerminalFormat(ctx.Bool(LogColorFlag.Name))
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
