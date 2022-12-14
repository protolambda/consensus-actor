package yolo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/log"
	"github.com/hashicorp/go-multierror"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/urfave/cli"

	"github.com/protolambda/consensus-actor/flags"
)

type PerfComputer struct {
	log log.Logger

	startEpoch common.Epoch
	endEpoch   common.Epoch

	blocks *leveldb.DB
	randao *leveldb.DB
	perf   *leveldb.DB

	lhChainDB       *leveldb.DB // lighthouse chain data, stores block contents
	lhChainSnapshot *leveldb.Snapshot

	indices []common.BoundedIndex

	spec *common.Spec
}

func NewPerfComputer(ctx *cli.Context, log log.Logger) (*PerfComputer, error) {
	imp := &PerfComputer{
		log:        log,
		startEpoch: common.Epoch(ctx.Uint64(flags.PerfStartEpochFlag.Name)),
		endEpoch:   common.Epoch(ctx.Uint64(flags.PerfEndEpochFlag.Name)),
	}

	baseDir := ctx.String(flags.DataDirFlag.Name)
	if baseDir == "" {
		return nil, fmt.Errorf("need base data dir path")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to prepare base data dir: %v", err)
	}

	spec, err := loadSpec(filepath.Join(baseDir, specFileName))
	if err != nil {
		return nil, err
	} else {
		imp.spec = spec
	}

	indicesPath := filepath.Join(baseDir, boundedIndicesFileName)
	indices, err := loadBoundedIndices(indicesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load indices data: %w", err)
	}
	imp.indices = indices

	if blocks, err := loadBlocksDB(baseDir, true, ctx); err != nil {
		_ = imp.Close()
		return nil, err
	} else {
		imp.blocks = blocks
	}
	if randao, err := loadRandaoDB(baseDir, true, ctx); err != nil {
		_ = imp.Close()
		return nil, err
	} else {
		imp.randao = randao
	}
	if perf, err := loadPerfDB(baseDir, false, ctx); err != nil {
		_ = imp.Close()
		return nil, err
	} else {
		imp.perf = perf
	}
	if chainDB, err := loadLighthouseChainDB(ctx); err != nil {
		_ = imp.Close()
		return nil, err
	} else {
		imp.lhChainDB = chainDB
		snap, err := chainDB.GetSnapshot()
		if err != nil {
			_ = imp.Close()
			return nil, err
		}
		imp.lhChainSnapshot = snap
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
	if s.randao != nil {
		if err := s.randao.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close randao db: %w", err))
		}
	}
	if s.perf != nil {
		if err := s.perf.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close perf db: %w", err))
		}
	}
	if s.lhChainSnapshot != nil {
		s.lhChainSnapshot.Release()
	}
	if s.lhChainDB != nil {
		if err := s.lhChainDB.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close lh chain db: %w", err))
		}
	}
	return result
}

func (s *PerfComputer) Run(ctx context.Context) error {
	if s.endEpoch < s.startEpoch {
		return fmt.Errorf("end epoch cannot be lower than start epoch: %d < %d", s.endEpoch, s.startEpoch)
	}
	lastBlockSlot, err := lastSlot(s.blocks)
	if err != nil {
		return fmt.Errorf("could not read max block slot: %w", err)
	}
	lastBlockEpoch := s.spec.SlotToEpoch(lastBlockSlot)
	if lastBlockEpoch < s.endEpoch {
		s.log.Info("reducing end epoch to available blocks", "end", lastBlockEpoch)
		s.endEpoch = lastBlockEpoch
	}

	for i := s.startEpoch; i < s.endEpoch; i++ {
		if i%100 == 0 {
			s.log.Info("updating performance data", "epoch", i)
		}
		if err := processPerf(s.perf, s.lhChainSnapshot, s.spec, s.blocks, s.randao, s.indices, i); err != nil {
			return fmt.Errorf("failed to process performance at epoch %d: %w", i, err)
		}
	}

	s.log.Info("finished computing performance data")
	return nil
}
