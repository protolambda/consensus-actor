package yolo

import (
	"fmt"
	"github.com/protolambda/consensus-actor/flags"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/urfave/cli"
	"path/filepath"
)

func loadBlocksDB(baseDir string, readOnly bool, ctx *cli.Context) (*leveldb.DB, error) {
	cacheSize := ctx.Int(flags.DataBlocksCacheSizeFlag.Name)
	blocksPath := filepath.Join(baseDir, ctx.GlobalString(flags.DataBlocksDBFlag.Name))
	if blocksPath == "" {
		return nil, fmt.Errorf("need blocks db path")
	}
	blocks, err := openDB(blocksPath, readOnly, cacheSize)
	if err != nil {
		return nil, fmt.Errorf("failed to open blocks db %q: %w", blocksPath, err)
	}
	return blocks, nil
}

func loadRandaoDB(baseDir string, readOnly bool, ctx *cli.Context) (*leveldb.DB, error) {
	cacheSize := ctx.Int(flags.DataRandaoCacheSizeFlag.Name)
	randaoPath := filepath.Join(baseDir, ctx.GlobalString(flags.DataRandaoDBFlag.Name))
	if randaoPath == "" {
		return nil, fmt.Errorf("need randao db path")
	}
	randao, err := openDB(randaoPath, readOnly, cacheSize)
	if err != nil {
		return nil, fmt.Errorf("failed to open randao db %q: %w", randaoPath, err)
	}
	return randao, nil
}

func loadPerfDB(baseDir string, readOnly bool, ctx *cli.Context) (*leveldb.DB, error) {
	cacheSize := ctx.Int(flags.DataPerfCacheSizeFlag.Name)
	perfPath := filepath.Join(baseDir, ctx.GlobalString(flags.DataPerfDBFlag.Name))
	if perfPath == "" {
		return nil, fmt.Errorf("need perf db path")
	}
	perf, err := openDB(perfPath, readOnly, cacheSize)
	if err != nil {
		return nil, fmt.Errorf("failed to open perf db %q: %w", perfPath, err)
	}
	return perf, nil
}

func loadTilesDB(baseDir string, readOnly bool, ctx *cli.Context) (*leveldb.DB, error) {
	cacheSize := ctx.Int(flags.DataTilesCacheSizeFlag.Name)
	tilesPath := filepath.Join(baseDir, ctx.GlobalString(flags.DataTilesDBFlag.Name))
	if tilesPath == "" {
		return nil, fmt.Errorf("need tiles db path")
	}
	tiles, err := openDB(tilesPath, readOnly, cacheSize)
	if err != nil {
		return nil, fmt.Errorf("failed to open tiles db %q: %w", tilesPath, err)
	}
	return tiles, nil
}
