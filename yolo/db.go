package yolo

import (
	"fmt"
	"github.com/hashicorp/go-multierror"
	"github.com/protolambda/consensus-actor/flags"
	"github.com/syndtr/goleveldb/leveldb"
	lvlerrs "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/urfave/cli"
	"path/filepath"
)

// opens a level DB.
//
// Filepath to locate db at.
// Readonly to limit db writes.
// Cache in megabytes to trade memory for better performance.
func openDB(file string, readonly bool, cache int) (*leveldb.DB, error) {
	options := &opt.Options{
		Filter:                 filter.NewBloomFilter(10),
		DisableSeeksCompaction: true,
		OpenFilesCacheCapacity: 1024,
		BlockCacheCapacity:     cache * opt.MiB,
		WriteBuffer:            cache * opt.MiB,
		ReadOnly:               readonly,
	}

	// Open the db and recover any potential corruptions
	db, err := leveldb.OpenFile(file, options)
	if _, corrupted := err.(*lvlerrs.ErrCorrupted); corrupted {
		db, err = leveldb.RecoverFile(file, nil)
	}
	if err != nil {
		return nil, err
	}
	return db, nil
}

func (s *Server) loadBlocksDB(baseDir string, ctx *cli.Context) error {
	blocksPath, err := filepath.Rel(baseDir, ctx.GlobalString(flags.DataBlocksDBFlag.Name))
	if err != nil {
		blocksPath = ctx.GlobalString(flags.DataBlocksDBFlag.Name)
	}
	if blocksPath == "" {
		return fmt.Errorf("need blocks db path")
	}
	blocks, err := openDB(blocksPath, false, 20)
	if err != nil {
		return fmt.Errorf("failed to open blocks db: %v", err)
	}
	s.blocks = blocks
	return nil
}

func (s *Server) loadPerfDB(baseDir string, ctx *cli.Context) error {
	perfPath, err := filepath.Rel(baseDir, ctx.GlobalString(flags.DataPerfDBFlag.Name))
	if err != nil {
		perfPath = ctx.GlobalString(flags.DataPerfDBFlag.Name)
	}
	if perfPath == "" {
		return fmt.Errorf("need perf db path")
	}
	perf, err := openDB(perfPath, false, 20)
	if err != nil {
		return fmt.Errorf("failed to open perf db: %v", err)
	}
	s.perf = perf
	return nil
}

func (s *Server) loadTilesDB(baseDir string, ctx *cli.Context) error {
	tilesPath, err := filepath.Rel(baseDir, ctx.GlobalString(flags.DataTilesDBFlag.Name))
	if err != nil {
		tilesPath = ctx.GlobalString(flags.DataTilesDBFlag.Name)
	}
	if tilesPath == "" {
		return fmt.Errorf("need tiles db path")
	}
	tiles, err := openDB(tilesPath, false, 20)
	if err != nil {
		return fmt.Errorf("failed to open tiles db: %v", err)
	}
	s.tiles = tiles
	return nil
}

func (s *Server) closeDBs() error {
	s.blocksLock.Lock()
	s.perfLock.Lock()
	s.tilesLock.Lock()

	defer s.blocksLock.Unlock()
	defer s.perfLock.Unlock()
	defer s.tilesLock.Unlock()

	var result *multierror.Error
	if s.blocks != nil {
		if err := s.blocks.Close(); err != nil {
			result = multierror.Append(result, err)
		}
	}
	if s.perf != nil {
		if err := s.perf.Close(); err != nil {
			result = multierror.Append(result, err)
		}
	}
	if s.tiles != nil {
		if err := s.tiles.Close(); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}
