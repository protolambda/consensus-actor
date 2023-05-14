package main

import (
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"

	"github.com/protolambda/consensus-actor/cmd"
)

func main() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))

	app := cli.NewApp()
	app.Version = "0.0.4"
	app.Name = "consensus-actor"
	app.Usage = "Consensus actor analysis tool by @protolambda"
	app.Description = "Build and serve a maps-like view of the consensus actor data of ethereum."
	app.Commands = []*cli.Command{
		cmd.PerfCmd,
		cmd.ServerCmd,
		cmd.TilesCmd,
	}
	err := app.Run(os.Args)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v", err)
	}
}
