package yolo

import (
	"github.com/syndtr/goleveldb/leveldb"
	lvlerrs "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
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
