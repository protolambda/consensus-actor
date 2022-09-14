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

	spec *common.Spec
}

func NewTileProcessor(ctx *cli.Context, log log.Logger) (*TileProcessor, error) {
	imp := &TileProcessor{
		log:        log,
		startEpoch: common.Epoch(ctx.Uint64(flags.TilesStartEpochFlag.Name)),
		endEpoch:   common.Epoch(ctx.Uint64(flags.TilesEndEpochFlag.Name)),
	}

	// TODO load spec

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

	return imp, nil
}

func (s *TileProcessor) Close() error {
	var result error
	if s.perf != nil {
		if err := s.perf.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close perf db: %w", err))
		}
	}
	return result
}

func (s *TileProcessor) Run(ctx context.Context) error {
	if s.endEpoch < s.startEpoch {
		return fmt.Errorf("end epoch cannot be lower than start epoch: %d < %d", s.endEpoch, s.startEpoch)
	}

	for tX := uint64(s.startEpoch) / tileSize; tX <= uint64(s.endEpoch)/tileSize; tX++ {
		if err := performanceToTiles(s.perf, s.tiles, 0, tX); err != nil {
			return fmt.Errorf("failed to update zoom 0 tiles at tX %d: %v", tX, err)
		}
	}

	for z := uint8(1); z <= maxZoom; z++ {
		tileSizeAbs := uint64(tileSize) << z
		tilesXStart := uint64(s.startEpoch) / tileSizeAbs
		tilesXEnd := (uint64(s.endEpoch) + tileSizeAbs - 1) / tileSizeAbs
		for i := tilesXStart; i < tilesXEnd; i++ {
			if err := convTiles(s.tiles, 0, i, z); err != nil {
				return fmt.Errorf("failed tile convolution layer at zoom %d tX %d: %v", z, i, err)
			}
		}
	}

	s.log.Info("finished computing tile data")
	return nil
}
