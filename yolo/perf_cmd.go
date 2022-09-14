package yolo

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/hashicorp/go-multierror"
	"github.com/protolambda/consensus-actor/flags"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/urfave/cli"
	"os"
)

type PerfComputer struct {
	log log.Logger

	startSlot common.Slot
	endSlot   common.Slot

	blocks *leveldb.DB // blocks db to import from
	perf   *leveldb.DB // performance per epoch

	// TODO: fetch bounded indices
}

func NewPerfComputer(ctx *cli.Context, log log.Logger) (*PerfComputer, error) {
	imp := &PerfComputer{
		log:       log,
		startSlot: common.Slot(ctx.Uint64(flags.ImportStartSlotFlag.Name)),
		endSlot:   common.Slot(ctx.Uint64(flags.ImportEndSlotFlag.Name)),
	}

	baseDir := ctx.GlobalString(flags.DataDirFlag.Name)
	if baseDir == "" {
		return nil, fmt.Errorf("need base data dir path")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to prepare base data dir: %v", err)
	}

	if blocks, err := loadBlocksDB(baseDir, true, ctx); err != nil {
		_ = imp.Close()
		return nil, err
	} else {
		imp.blocks = blocks
	}
	if perf, err := loadPerfDB(baseDir, true, ctx); err != nil {
		_ = imp.Close()
		return nil, err
	} else {
		imp.perf = perf
	}
	return imp, nil
}

func (s *PerfComputer) Close() error {
	var result error
	if s.blocks != nil {
		if err := s.blocks.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close blocks db: %w", err))
		}
	}
	if s.perf != nil {
		if err := s.perf.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close perf db: %w", err))
		}
	}
	return result
}

func (s *PerfComputer) Run(ctx context.Context) error {
	if s.endSlot < s.startSlot {
		return fmt.Errorf("end slot cannot be lower than start slot: %d < %d", s.endSlot, s.startSlot)
	}

	s.log.Info("finished computing performance data")
	return nil
}
