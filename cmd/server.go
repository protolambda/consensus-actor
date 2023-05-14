package cmd

import (
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/protolambda/consensus-actor/fun"
)

var (
	ServerListenAddrFlag = &cli.StringFlag{
		Name:  "listen",
		Usage: "listen address to bind to",
		Value: "0.0.0.0:8080",
	}
	ServerPublicFlag = cli.StringFlag{
		Name:  "public",
		Usage: "public endpoint to use for API queries",
		Value: "127.0.0.1:8080",
	}
	ServerTilesFlag = &cli.PathFlag{
		Name:  "tiles",
		Usage: "path to tiles db to read tile data from",
		Value: "tiles_db",
	}
)

var ServerCmd = &cli.Command{
	Name:        "server",
	Usage:       "Run http server.",
	Description: "Run http server.",
	Action:      Server,
	Flags: []cli.Flag{
		LogLevelFlag,
		LogFormatFlag,
		LogColorFlag,
		ServerListenAddrFlag,
		ServerTilesFlag,
	},
}

func Server(ctx *cli.Context) error {
	log, err := SetupLogger(ctx)
	if err != nil {
		return err
	}

	listenAddr := ctx.String(ServerListenAddrFlag.Name)
	publicEndpoint := ctx.String(ServerPublicFlag.Name)

	tilesDB, err := fun.OpenDB(ctx.Path(ServerTilesFlag.Name), true, 100, 0)
	if err != nil {
		return fmt.Errorf("failed to open perf db: %w", err)
	}
	defer tilesDB.Close()

	log.Info("starting server", "listen", listenAddr, "public", publicEndpoint)

	imgHandler := &fun.ImageHandler{Log: log, TilesDB: tilesDB}

	srv := fun.StartHttpServer(log, listenAddr, &fun.IndexData{
		Title: "Consensus.actor | mainnet",
		API:   publicEndpoint,
	}, imgHandler.HandleImgRequest)

	<-ctx.Done()

	log.Info("closing server")

	if err := srv.Close(); err != nil {
		log.Error("failed to close server", "err", err)
	}

	log.Info("closed server")
	return nil
}
