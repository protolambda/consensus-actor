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
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/urfave/cli"

	"github.com/protolambda/consensus-actor/flags"
)

type RandaoComputer struct {
	log log.Logger

	spec *common.Spec

	startEpoch common.Epoch
	endEpoch   common.Epoch

	blocks *leveldb.DB
	randao *leveldb.DB

	lhChainDB       *leveldb.DB // lighthouse chain data, stores block contents
	lhChainSnapshot *leveldb.Snapshot

	lhFreezerDB *leveldb.DB
}

func NewRandaoComputer(ctx *cli.Context, log log.Logger) (*RandaoComputer, error) {
	imp := &RandaoComputer{
		log:        log,
		startEpoch: common.Epoch(ctx.Uint64(flags.RandaoStartEpochFlag.Name)),
		endEpoch:   common.Epoch(ctx.Uint64(flags.RandaoEndEpochFlag.Name)),
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
	//if freezerDB, err := loadLighthouseFreezerDB(ctx); err != nil {
	//	_ = imp.Close()
	//	return nil, err
	//} else {
	//	imp.lhFreezerDB = freezerDB
	//}
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
	if s.lhChainSnapshot != nil {
		s.lhChainSnapshot.Release()
	}
	if s.lhChainDB != nil {
		if err := s.lhChainDB.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close lh chain db: %w", err))
		}
	}
	//if s.lhFreezerDB != nil {
	//	if err := s.lhFreezerDB.Close(); err != nil {
	//		result = multierror.Append(result, fmt.Errorf("failed to close lh freezer db: %w", err))
	//	}
	//}
	return result
}

func (s *RandaoComputer) Run(ctx context.Context) error {
	if s.endEpoch < s.startEpoch {
		return fmt.Errorf("end epoch cannot be lower than start epoch: %d < %d", s.endEpoch, s.startEpoch)
	}

	roots, err := getAllBlockRoots(s.blocks, s.log)
	if err != nil {
		return fmt.Errorf("failed to get block roots: %w", err)
	}

	// iterate over all blocks in order of the DB, to allow leveldb to speed it up.
	iter := s.lhChainDB.NewIterator(util.BytesPrefix([]byte(lhBeaconBlocksPrefix)), nil)
	defer iter.Release()

	randaos := make(map[common.Slot]common.BLSSignature, len(roots))
	for iter.Next() {
		key := iter.Key()
		value := iter.Value()

		var root common.Root
		copy(root[:], key[3:])
		slot, ok := roots[root]
		if !ok {
			continue
		}
		block, err := parseBlock(s.spec, value, slot)
		if err != nil {
			return fmt.Errorf("failed to parse block at slot %d: %w", slot, err)
		}
		randaos[slot] = block.RandaoReveal

		if len(randaos)%100 == 0 {
			s.log.Info("block parsing", "count", len(randaos))
		}
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
		if err := updateRandao(s.log, s.spec, s.randao, randaos, i); err != nil {
			return fmt.Errorf("failed to update randao at prev epoch %d: %w", i, err)
		}
	}

	s.log.Info("finished computing randao data")
	return nil
}

// Much faster, but LH doesn't seem to retain all historical randao data, so this is incomplete
//func MigrateLHRandao(randoDB *leveldb.DB, lhFreezer *leveldb.DB) error {
//	// keyed by epoch divided by interval (default 128), u64
//	// value is interval times a root 128*32=8192 bytes
//	iter := lhFreezer.NewIterator(util.BytesPrefix([]byte("brm")), nil)
//	for iter.Next() {
//		key := iter.Key()
//		value := iter.Value()
//		ep := common.Epoch(binary.BigEndian.Uint64(key[3:]))
//		first := ep == 0
//		end := common.Epoch(1)
//		// first entry only has for genesis state
//		// then it continues with 128 epochs at a time after.
//		if first {
//			if len(value) != 32 {
//				return fmt.Errorf("unexpected value length for first epoch: %d", len(value))
//			}
//		} else {
//			ep -= 1
//			ep *= 128
//			ep += 1
//			end = ep + 128
//			if len(value) != 128*32 {
//				return fmt.Errorf("unexpected value length: %d", len(value))
//			}
//		}
//
//		if ep%128 == 1 {
//			fmt.Printf("processing epoch %d\n", ep)
//		}
//		b := new(leveldb.Batch)
//		var outKey [3 + 8]byte
//		copy(outKey[:3], KeyRandaoMix)
//		i := 0
//		for {
//			if ep >= end {
//				break
//			}
//			binary.BigEndian.PutUint64(outKey[3:], uint64(ep))
//			b.Put(outKey[:], value[i*32:i*32+32])
//			i++
//			ep += 1
//		}
//		if err := randoDB.Write(b, nil); err != nil {
//			return fmt.Errorf("failed at lh key %x: %v", key, err)
//		}
//	}
//	return nil
//}
