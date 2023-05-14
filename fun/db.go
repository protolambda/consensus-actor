package fun

import (
	"github.com/syndtr/goleveldb/leveldb"
	lvlerrs "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// OpenDB opens a level DB.
//
// Filepath to locate db at.
// Readonly to limit db writes.
// Cache in megabytes to trade memory for better performance.
// writeBuf in megabytes to improve writing performance
func OpenDB(file string, readonly bool, cache int, writeBuf int) (*leveldb.DB, error) {
	options := &opt.Options{
		Filter:                 filter.NewBloomFilter(10),
		DisableSeeksCompaction: true,
		OpenFilesCacheCapacity: 1024,
		BlockCacheCapacity:     cache * opt.MiB,
		WriteBuffer:            writeBuf * opt.MiB,
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
