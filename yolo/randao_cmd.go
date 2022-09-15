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
	"path/filepath"
)

type RandaoComputer struct {
	log log.Logger

	spec *common.Spec

	startEpoch common.Epoch
	endEpoch   common.Epoch

	blocks *leveldb.DB
	randao *leveldb.DB
}

func NewRandaoComputer(ctx *cli.Context, log log.Logger) (*RandaoComputer, error) {
	imp := &RandaoComputer{
		log:        log,
		startEpoch: common.Epoch(ctx.Uint64(flags.RandaoStartEpochFlag.Name)),
		endEpoch:   common.Epoch(ctx.Uint64(flags.RandaoEndEpochFlag.Name)),
	}

	baseDir := ctx.GlobalString(flags.DataDirFlag.Name)
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
	if blocks, err := loadBlocksDB(baseDir, true, ctx); err != nil {
		_ = imp.Close()
		return nil, err
	} else {
		imp.blocks = blocks
	}
	if randao, err := loadRandaoDB(baseDir, false, ctx); err != nil {
		_ = imp.Close()
		return nil, err
	} else {
		imp.randao = randao
	}
	return imp, nil
}

func (s *RandaoComputer) Close() error {
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
	return result
}

func (s *RandaoComputer) Run(ctx context.Context) error {
	if s.endEpoch < s.startEpoch {
		return fmt.Errorf("end epoch cannot be lower than start epoch: %d < %d", s.endEpoch, s.startEpoch)
	}

	lastEpoch, err := lastRandaoEpoch(s.randao)
	if err != nil {
		return fmt.Errorf("could not read last randao epoch: %w", err)
	}
	if lastEpoch < s.startEpoch {
		return fmt.Errorf("missing randao data, expected to start from at most epoch %d, but got %d", lastEpoch, s.startEpoch)
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
			s.log.Info("updating randao data", "prev_epoch", i)
		}
		if err := updateRandao(s.log, s.spec, s.randao, s.blocks, s.startEpoch); err != nil {
			return fmt.Errorf("failed to update randao at prev epoch %d: %w", i, err)
		}
	}

	s.log.Info("finished computing randao data")
	return nil
}
