package cmd

import (
	"fmt"

	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/urfave/cli/v2"

	"github.com/protolambda/consensus-actor/fun"
)

var (
	TilesPerfFlag = &cli.PathFlag{
		Name:      "perf",
		Usage:     "Path to validator perf database to read from",
		TakesFile: true,
		Value:     "perf_db",
		Required:  false,
	}
	TilesTilesFlag = &cli.PathFlag{
		Name:  "tiles",
		Usage: "path to tiles db to write tile data to",
		Value: "tiles_db",
	}
	TilesStartEpochFlag = &cli.Uint64Flag{
		Name:  "start-epoch",
		Usage: "Start epoch (inclusive) of tiles to update",
		Value: uint64(0),
	}
	TilesEndEpochFlag = &cli.Uint64Flag{
		Name:  "end-epoch",
		Usage: "End epoch (exclusive) of tiles to update",
		Value: ^uint64(0),
	}
)

var TilesCmd = &cli.Command{
	Name:        "tiles",
	Usage:       "Compute tiles for range of epochs.",
	Description: "Compute tiles for range of epochs.",
	Action:      Tiles,
	Flags: []cli.Flag{
		LogLevelFlag,
		LogFormatFlag,
		LogColorFlag,
		TilesPerfFlag,
		TilesTilesFlag,
		TilesStartEpochFlag,
		TilesEndEpochFlag,
	},
}

func Tiles(ctx *cli.Context) error {
	log, err := SetupLogger(ctx)
	if err != nil {
		return err
	}
	startEpoch := common.Epoch(ctx.Uint64(TilesStartEpochFlag.Name))
	endEpoch := common.Epoch(ctx.Uint64(TilesEndEpochFlag.Name))
	perfDB, err := fun.OpenDB(ctx.Path(TilesPerfFlag.Name), true, 100, 0)
	if err != nil {
		return fmt.Errorf("failed to open perf db: %w", err)
	}
	defer perfDB.Close()
	tilesDB, err := fun.OpenDB(ctx.Path(TilesTilesFlag.Name), false, 100, 100)
	if err != nil {
		return fmt.Errorf("failed to open perf db: %w", err)
	}
	defer tilesDB.Close()
	return fun.UpdateTiles(log, tilesDB, perfDB, startEpoch, endEpoch)
}
