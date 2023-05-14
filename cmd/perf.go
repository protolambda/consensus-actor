package cmd

import (
	"fmt"

	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/configs"
	"github.com/urfave/cli/v2"

	"github.com/protolambda/consensus-actor/fun"
	"github.com/protolambda/consensus-actor/fun/era"
)

var (
	PerfPerfFlag = &cli.PathFlag{
		Name:      "perf",
		Usage:     "Path to validator perf database to output to",
		TakesFile: true,
		Value:     "perf_db",
		Required:  false,
	}
	PerfEraFlag = &cli.PathFlag{
		Name:      "era",
		Usage:     "Path to era store dir",
		TakesFile: true,
		Required:  true,
	}
	PerfStartEpochFlag = &cli.Uint64Flag{
		Name:  "start-epoch",
		Usage: "Start epoch (inclusive) of validator performance data to update",
		Value: uint64(0),
	}
	PerfEndEpochFlag = &cli.Uint64Flag{
		Name:  "end-epoch",
		Usage: "End epoch (exclusive) of validator performance data to update",
		Value: ^uint64(0),
	}
	PerfWorkersFlag = &cli.IntFlag{
		Name:  "workers",
		Usage: "number of workers to used to process in parallel",
		Value: 8,
	}
	// TODO spec flag
)

var PerfCmd = &cli.Command{
	Name:        "perf",
	Usage:       "Compute validators performance in epoch range.",
	Description: "Compute validators performance in epoch range.",
	Action:      Perf,
	Flags: []cli.Flag{
		LogLevelFlag,
		LogFormatFlag,
		LogColorFlag,
		PerfPerfFlag,
		PerfEraFlag,
		PerfStartEpochFlag,
		PerfEndEpochFlag,
		PerfWorkersFlag,
	},
}

func Perf(ctx *cli.Context) error {
	log, err := SetupLogger(ctx)
	if err != nil {
		return err
	}
	startEpoch := common.Epoch(ctx.Uint64(PerfStartEpochFlag.Name))
	endEpoch := common.Epoch(ctx.Uint64(PerfEndEpochFlag.Name))

	workers := ctx.Int(PerfWorkersFlag.Name)
	if workers < 0 || workers > 128 {
		return fmt.Errorf("invalid workers count: %d", workers)
	}

	perfDB, err := fun.OpenDB(ctx.Path(PerfPerfFlag.Name), false, 100, 0)
	if err != nil {
		return fmt.Errorf("failed to open perf db: %w", err)
	}
	defer perfDB.Close()

	es := era.NewStore()
	if err := es.Load(ctx.Path(PerfEraFlag.Name)); err != nil {
		return fmt.Errorf("failed to index era store: %w", err)
	}
	spec := configs.Mainnet

	minSlot, maxSlot := es.Bounds()
	minEpoch, maxEpoch := spec.SlotToEpoch(minSlot), spec.SlotToEpoch(maxSlot)
	if minEpoch >= maxEpoch {
		return fmt.Errorf("no era data")
	}

	epochsPerEra := spec.SlotToEpoch(era.SlotsPerEra)
	if minEpoch > startEpoch+epochsPerEra-2 {
		startEpoch = minEpoch - epochsPerEra + 2
		log.Warn("adjusting lower bound", "start_epoch", startEpoch, "min_era_epoch", minEpoch)
	}
	if maxEpoch < endEpoch {
		endEpoch = maxEpoch
		log.Warn("adjusting upper bound", "end_epoch", startEpoch, "max_era_epoch", maxEpoch)
	}

	if err := fun.UpdatePerf(ctx.Context, log, perfDB, spec, es, startEpoch, endEpoch, workers); err != nil {
		return fmt.Errorf("failed to update validator performance data: %w", err)
	}
	return nil
}
