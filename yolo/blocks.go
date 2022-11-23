package yolo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/log"
	"github.com/protolambda/zrnt/eth2/beacon/altair"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/beacon/phase0"
	"github.com/protolambda/ztyp/codec"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	// KeyBlockRoot is a:
	// 3 byte prefix for keying beacon block roots, followed by:
	// 8 byte big-endian slot number
	//
	// The value is the canonical beacon block root, bytes32
	//
	// Roots are repeated in gap slots
	KeyBlockRoot string = "blr"
)

type BlockData struct {
	Slot         common.Slot
	StateRoot    common.Root
	Attestations phase0.Attestations
	RandaoReveal common.BLSSignature
}

var ErrBlockNotFound = errors.New("block not found")

func getBlock(blocks *leveldb.DB, lhChainDB *leveldb.Snapshot, spec *common.Spec, slot common.Slot) (*BlockData, error) {
	root, err := getBlockRoot(blocks, slot)
	if err != nil {
		return nil, fmt.Errorf("failed to find block root for given slot %d: %w", slot, err)
	}
	var tmpKey [3 + 32]byte
	copy(tmpKey[:3], lhBeaconBlocksPrefix)
	copy(tmpKey[3:], root[:])
	data, err := lhChainDB.Get(tmpKey[:], &opt.ReadOptions{
		DontFillCache: true,
	})
	if err != nil {
		return nil, fmt.Errorf("lighthouse chain db is missing contents referenced by root %s for slot %d db index: %w", root, slot, err)
	}
	return parseBlock(spec, data, slot)
}

func parseBlock(spec *common.Spec, data []byte, slot common.Slot) (*BlockData, error) {
	if uint64(spec.BELLATRIX_FORK_EPOCH)*uint64(spec.SLOTS_PER_EPOCH) <= uint64(slot) {
		var dest SignedBeaconBlockLH
		if err := dest.Deserialize(spec, codec.NewDecodingReader(bytes.NewReader(data), uint64(len(data)))); err != nil {
			return nil, err
		}
		if dest.Message.Slot != slot {
			return nil, ErrBlockNotFound
		}
		return &BlockData{
			Slot:         slot,
			StateRoot:    dest.Message.StateRoot,
			Attestations: dest.Message.Body.Attestations,
			RandaoReveal: dest.Message.Body.RandaoReveal,
		}, nil
	} else if uint64(spec.ALTAIR_FORK_EPOCH)*uint64(spec.SLOTS_PER_EPOCH) <= uint64(slot) {
		var dest altair.SignedBeaconBlock
		if err := dest.Deserialize(spec, codec.NewDecodingReader(bytes.NewReader(data), uint64(len(data)))); err != nil {
			return nil, err
		}
		if dest.Message.Slot != slot {
			return nil, ErrBlockNotFound
		}
		return &BlockData{
			Slot:         slot,
			StateRoot:    dest.Message.StateRoot,
			Attestations: dest.Message.Body.Attestations,
			RandaoReveal: dest.Message.Body.RandaoReveal,
		}, nil
	} else {
		var dest phase0.SignedBeaconBlock
		if err := dest.Deserialize(spec, codec.NewDecodingReader(bytes.NewReader(data), uint64(len(data)))); err != nil {
			return nil, err
		}
		if dest.Message.Slot != slot {
			return nil, ErrBlockNotFound
		}
		return &BlockData{
			Slot:         slot,
			StateRoot:    dest.Message.StateRoot,
			Attestations: dest.Message.Body.Attestations,
			RandaoReveal: dest.Message.Body.RandaoReveal,
		}, nil
	}
}

func getAllBlockRoots(blocks *leveldb.DB, log log.Logger) (map[common.Root]common.Slot, error) {
	iter := blocks.NewIterator(util.BytesPrefix([]byte(KeyBlockRoot)), nil)
	defer iter.Release()
	roots := map[common.Root]common.Slot{}
	for iter.Next() {
		key := iter.Key()
		value := iter.Value()
		var root common.Root
		copy(root[:], value)
		if root == (common.Root{}) {
			continue
		}
		slot := common.Slot(binary.BigEndian.Uint64(key[3:]))
		if _, ok := roots[root]; !ok || slot < roots[root] { // pick the lowest. The higher ones are gap slots
			roots[root] = slot
		}
		if len(roots)%1000 == 0 {
			log.Info("block roots", "count", len(roots))
		}
	}
	return roots, nil
}

func getBlockRoot(blocks *leveldb.DB, slot common.Slot) (common.Root, error) {
	var key [3 + 8]byte
	copy(key[:3], KeyBlockRoot)
	binary.BigEndian.PutUint64(key[3:], uint64(slot))
	root, err := blocks.Get(key[:], nil)
	if err == leveldb.ErrNotFound {
		return common.Root{}, ErrBlockNotFound
	}
	if err != nil {
		return [32]byte{}, err
	}
	if len(root) != 32 {
		return [32]byte{}, fmt.Errorf("unexpected block root value length (%d): %x", len(root), root)
	}
	return *(*[32]byte)(root), nil
}

func lastSlot(blocks *leveldb.DB) (common.Slot, error) {
	blockIter := blocks.NewIterator(util.BytesPrefix([]byte(KeyBlockRoot)), nil)
	// big-endian slot number, last key is highest slot block we have
	if blockIter.Last() { // if we have a block, get it
		slot := common.Slot(binary.BigEndian.Uint64(blockIter.Key()[3:]))
		blockIter.Release()
		if err := blockIter.Error(); err != nil {
			return slot, fmt.Errorf("failed to get last block root from db: %v", err)
		}
		return slot, nil
	} else {
		blockIter.Release()
		// no block in DB, return genesis
		return 0, nil
	}
}

func lastSlotAndRoot(blocks *leveldb.DB, genesisBlockRoot common.Root) (common.Slot, common.Root, error) {
	blockIter := blocks.NewIterator(util.BytesPrefix([]byte(KeyBlockRoot)), nil)
	// big-endian slot number, last key is highest slot block we have
	if blockIter.Last() { // if we have a block, get it
		slot := common.Slot(binary.BigEndian.Uint64(blockIter.Key()[3:]))
		var blockRoot common.Root
		copy(blockRoot[:], blockIter.Value())
		blockIter.Release()
		if err := blockIter.Error(); err != nil {
			return slot, common.Root{}, fmt.Errorf("failed to get last block root from db: %v", err)
		}
		return slot, blockRoot, nil
	} else {
		blockIter.Release()
		// no block in DB, return genesis
		return 0, genesisBlockRoot, nil
	}
}

func resetBlocks(blocks *leveldb.DB, slot common.Slot, prev common.Slot) error {
	var batch leveldb.Batch

	deletePrefix := func(prefix string) {
		keyRange := &util.Range{
			Start: make([]byte, len(prefix)+8),
			Limit: make([]byte, len(prefix)+8),
		}
		copy(keyRange.Start[:len(prefix)], prefix)
		binary.BigEndian.PutUint64(keyRange.Start[len(prefix):], uint64(slot+1))
		copy(keyRange.Limit[:len(prefix)], prefix)
		binary.BigEndian.PutUint64(keyRange.Limit[len(prefix):], uint64(prev+1))

		iter := blocks.NewIterator(keyRange, nil)
		defer iter.Release()

		for iter.Next() {
			batch.Delete(iter.Key())
		}
	}

	//deletePrefix(KeyBlock)
	deletePrefix(KeyBlockRoot)

	if err := blocks.Write(&batch, nil); err != nil {
		return fmt.Errorf("failed to cleanup conflicting block data: %v", err)
	}
	return nil
}
