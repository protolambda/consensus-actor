package yolo

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/hashicorp/go-multierror"
	"github.com/protolambda/consensus-actor/flags"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/urfave/cli"
	"os"
)

const lhBeaconBlockRootsPrefix = "bbr"
const lhBeaconBlocksPrefix = "blk"

type BlocksImporter struct {
	log log.Logger

	startSlot common.Slot
	endSlot   common.Slot

	blocks    *leveldb.DB // blocks db to import into
	lhFreezer *leveldb.DB // lighthouse freezer data, stores beacon block roots in batches
	lhChainDB *leveldb.DB // lighthouse chain data, stores block contents
}

func NewBlocksImporter(ctx *cli.Context, log log.Logger) (*BlocksImporter, error) {
	imp := &BlocksImporter{
		log:       log,
		startSlot: common.Slot(ctx.Uint64(flags.ImportStartSlotFlag.Name)),
		endSlot:   common.Slot(ctx.Uint64(flags.ImportEndSlotFlag.Name)),
		lhFreezer: nil,
		lhChainDB: nil,
	}

	baseDir := ctx.GlobalString(flags.DataDirFlag.Name)
	if baseDir == "" {
		return nil, fmt.Errorf("need base data dir path")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to prepare base data dir: %v", err)
	}

	if blocks, err := loadBlocksDB(baseDir, false, ctx); err != nil {
		_ = imp.Close()
		return nil, err
	} else {
		imp.blocks = blocks
	}
	if err := imp.loadLighthouseChainDBMaybe(ctx); err != nil {
		_ = imp.Close()
		return nil, err
	}
	if err := imp.loadLighthouseFreezerDBMaybe(ctx); err != nil {
		_ = imp.Close()
		return nil, err
	}
	return imp, nil
}

func (s *BlocksImporter) loadLighthouseChainDBMaybe(ctx *cli.Context) error {
	cacheSize := ctx.Int(flags.ImportLighthouseChainCacheSizeFlag.Name)
	chPath := ctx.String(flags.ImportLighthouseChainFlag.Name)
	if chPath == "" {
		s.log.Info("No lighthouse chain db specified for import")
		return nil
	}
	db, err := openDB(chPath, true, cacheSize, 0)
	if err != nil {
		return fmt.Errorf("failed to open lighthouse chain db: %w", err)
	}
	s.lhChainDB = db
	return nil
}

func (s *BlocksImporter) loadLighthouseFreezerDBMaybe(ctx *cli.Context) error {
	cacheSize := ctx.Int(flags.ImportLighthouseFreezerCacheSizeFlag.Name)
	frPath := ctx.String(flags.ImportLighthouseFreezerFlag.Name)
	if frPath == "" {
		s.log.Info("No lighthouse freezer db specified for import")
		return nil
	}
	db, err := openDB(frPath, true, cacheSize, 0)
	if err != nil {
		return fmt.Errorf("failed to open lighthouse freezer db: %w", err)
	}
	s.lhFreezer = db
	return nil
}

func (s *BlocksImporter) Close() error {
	var result error
	if s.blocks != nil {
		if err := s.blocks.Close(); err != nil {
			result = multierror.Append(result, err)
		}
	}
	if s.lhChainDB != nil {
		if err := s.lhChainDB.Close(); err != nil {
			result = multierror.Append(result, err)
		}
	}
	if s.lhFreezer != nil {
		if err := s.lhFreezer.Close(); err != nil {
			result = multierror.Append(result, err)
		}
	}
	return result
}

func (s *BlocksImporter) Run(ctx context.Context) error {
	if s.endSlot < s.startSlot {
		return fmt.Errorf("end slot cannot be lower than start slot: %d < %d", s.endSlot, s.startSlot)
	}

	iter := s.lhFreezer.NewIterator(util.BytesPrefix([]byte(lhBeaconBlockRootsPrefix)), nil)
	defer iter.Release()
	{
		var tmp [3 + 8]byte
		copy(tmp[:3], lhBeaconBlockRootsPrefix)
		slot := uint64(s.startSlot)

		binary.BigEndian.PutUint64(tmp[3:], slot/128)
		if !iter.Seek(tmp[:]) {
			return fmt.Errorf("failed to seek to resume block migration, at: %x slot %d", tmp[:], slot)
		}
	}

	s.log.Info("starting import", "start_slot", s.startSlot, "end_slot", s.endSlot)

	gapSlots := 0

	for iter.Next() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		key := iter.Key()
		keyInt := binary.BigEndian.Uint64(key[3:])
		if keyInt == 0 {
			continue
		}
		value := iter.Value()
		slot := common.Slot((keyInt - 1) * 128)

		var tmpKey [3 + 32]byte
		copy(tmpKey[:3], lhBeaconBlocksPrefix)

		// import all blocks in this lighthouse block roots batch at once
		// copy over block root from lighthouse db instead of recomputing it
		b := new(leveldb.Batch)
		var prevRoot [32]byte

		var outKeyBlock [3 + 8]byte
		copy(outKeyBlock[:3], KeyBlock)
		var outKeyBlockRoot [3 + 8]byte
		copy(outKeyBlockRoot[:3], KeyBlockRoot)
		for i := 0; i < 128; i++ {
			if slot < s.startSlot {
				slot += 1
				continue
			}
			progress := slot - s.startSlot
			if progress%1000 == 0 {
				s.log.Info("importing blocks from lighthouse db", "progress", progress, "slot", slot, "gaps", gapSlots)
			}
			if slot >= s.endSlot {
				break
			}

			// init prevRoot if we need to
			if prevRoot == ([32]byte{}) {
				// parent hash of genesis has a special batch entry
				var batchIndex, rootIndex uint64
				if slot == 0 {
					batchIndex = 0
					rootIndex = 0
				} else {
					batchIndex = (uint64(slot-1) / 128) + 1
					rootIndex = uint64(slot-1) % 128
				}

				var tmp [3 + 8]byte
				copy(tmp[:3], lhBeaconBlockRootsPrefix)
				binary.BigEndian.PutUint64(tmp[3:], batchIndex)
				b, err := s.lhFreezer.Get(tmp[:], nil)
				if err != nil {
					return fmt.Errorf("failed to read block roots batch from db to find prev block root at batch %d: %w", batchIndex, err)
				}
				copy(prevRoot[:], b[rootIndex*32:rootIndex*32+32])
			}

			root := value[i*32 : i*32+32]
			binary.BigEndian.PutUint64(outKeyBlockRoot[3:], uint64(slot))
			b.Put(outKeyBlockRoot[:], root)

			// if root matches previous root:
			if bytes.Equal(prevRoot[:], root) {
				// gap block
				slot += 1
				gapSlots += 1
				continue
			}

			copy(tmpKey[3:], root)
			blockContents, err := s.lhChainDB.Get(tmpKey[:], nil)
			if err != nil {
				return fmt.Errorf("lighthouse chain db is missing contents referenced by lighthouse freezer db: %w", err)
			}
			binary.BigEndian.PutUint64(outKeyBlock[3:], uint64(slot))
			b.Put(outKeyBlock[:], blockContents)

			slot += 1

			copy(prevRoot[:], root)
		}

		if err := s.blocks.Write(b, nil); err != nil {
			return fmt.Errorf("failed to write blocks: key %x slot %d err %w", key, slot, err)
		}

		if slot >= s.endSlot {
			break
		}
	}
	s.log.Info("finished importing blocks")
	return nil
}
