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

type TileProcessor struct {
	log log.Logger

	startEpoch common.Epoch
	endEpoch   common.Epoch

	perf  *leveldb.DB
	tiles *leveldb.DB
}

func NewTileProcessor(ctx *cli.Context, log log.Logger) (*TileProcessor, error) {
	imp := &TileProcessor{
		log:        log,
		startEpoch: common.Epoch(ctx.Uint64(flags.TilesStartEpochFlag.Name)),
		endEpoch:   common.Epoch(ctx.Uint64(flags.TilesEndEpochFlag.Name)),
	}

	baseDir := ctx.GlobalString(flags.DataDirFlag.Name)
	if baseDir == "" {
		return nil, fmt.Errorf("need base data dir path")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to prepare base data dir: %v", err)
	}

	if perf, err := loadPerfDB(baseDir, true, ctx); err != nil {
		_ = imp.Close()
		return nil, err
	} else {
		imp.perf = perf
	}

	if tiles, err := loadTilesDB(baseDir, false, ctx); err != nil {
		_ = imp.Close()
		return nil, err
	} else {
		imp.tiles = tiles
	}

	return imp, nil
}

func (s *TileProcessor) Close() error {
	var result error
	if s.perf != nil {
		if err := s.perf.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close perf db: %w", err))
		}
	}
	if s.tiles != nil {
		if err := s.tiles.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close tiles db: %w", err))
		}
	}
	return result
}

func (s *TileProcessor) Run(ctx context.Context) error {
	if s.endEpoch < s.startEpoch {
		return fmt.Errorf("end epoch cannot be lower than start epoch: %d < %d", s.endEpoch, s.startEpoch)
	}
	lastPerfEpoch, err := lastPerfEpoch(s.perf)
	if err != nil {
		return fmt.Errorf("could not read max block slot: %w", err)
	}
	if lastPerfEpoch < s.endEpoch {
		s.log.Info("reducing end epoch to available performance data", "end", lastPerfEpoch)
		s.endEpoch = lastPerfEpoch
	}

	for tX := uint64(s.startEpoch) / tileSize; tX <= uint64(s.endEpoch)/tileSize; tX++ {
		log.Info("creating base tiles", "tX", tX, "zoom", 0)
		if err := performanceToTiles(s.log, s.tiles, s.perf, 0, tX); err != nil {
			return fmt.Errorf("failed to update zoom 0 tiles at tX %d: %v", tX, err)
		}
	}

	for z := uint8(1); z <= maxZoom; z++ {
		tileSizeAbs := uint64(tileSize) << z
		tilesXStart := uint64(s.startEpoch) / tileSizeAbs
		tilesXEnd := (uint64(s.endEpoch) + tileSizeAbs - 1) / tileSizeAbs
		for i := tilesXStart; i < tilesXEnd; i++ {
			log.Info("computing conv tiles", "tX", i, "zoom", z)
			if err := convTiles(s.tiles, 0, i, z); err != nil {
				return fmt.Errorf("failed tile convolution layer at zoom %d tX %d: %v", z, i, err)
			}
		}
	}

	s.log.Info("finished computing tile data")
	return nil
}
