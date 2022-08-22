package yolo

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/protolambda/eth2api"
	"github.com/protolambda/eth2api/client/beaconapi"
	"github.com/protolambda/zrnt/eth2/beacon/altair"
	"github.com/protolambda/zrnt/eth2/beacon/bellatrix"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/beacon/phase0"
	"github.com/protolambda/ztyp/codec"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"time"
)

const (
	// KeyBlock is a:
	// 3 byte prefix for keying beacon block roots, followed by:
	// 8 byte big-endian slot number
	//
	// The value is a SSZ-encoded beacon block (no compression).
	//
	// Some blocks may be missing.
	KeyBlock string = "blk"

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

func (s *Server) getBlock(slot common.Slot) (*BlockData, error) {
	var key [3 + 8]byte
	copy(key[:3], KeyBlock)
	binary.BigEndian.PutUint64(key[3:], uint64(slot))
	data, err := s.blocks.Get(key[:], nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, ErrBlockNotFound
		}
		return nil, err
	}

	if uint64(s.spec.BELLATRIX_FORK_EPOCH)*uint64(s.spec.SLOTS_PER_EPOCH) <= uint64(slot) {
		var dest bellatrix.SignedBeaconBlock
		if err := dest.Deserialize(s.spec, codec.NewDecodingReader(bytes.NewReader(data), uint64(len(data)))); err != nil {
			return nil, err
		}
		return &BlockData{
			Slot:         slot,
			StateRoot:    dest.Message.StateRoot,
			Attestations: dest.Message.Body.Attestations,
			RandaoReveal: dest.Message.Body.RandaoReveal,
		}, nil
	} else if uint64(s.spec.ALTAIR_FORK_EPOCH)*uint64(s.spec.SLOTS_PER_EPOCH) <= uint64(slot) {
		var dest altair.SignedBeaconBlock
		if err := dest.Deserialize(s.spec, codec.NewDecodingReader(bytes.NewReader(data), uint64(len(data)))); err != nil {
			return nil, err
		}
		return &BlockData{
			Slot:         slot,
			StateRoot:    dest.Message.StateRoot,
			Attestations: dest.Message.Body.Attestations,
			RandaoReveal: dest.Message.Body.RandaoReveal,
		}, nil
	} else {
		var dest phase0.SignedBeaconBlock
		if err := dest.Deserialize(s.spec, codec.NewDecodingReader(bytes.NewReader(data), uint64(len(data)))); err != nil {
			return nil, err
		}
		return &BlockData{
			Slot:         slot,
			StateRoot:    dest.Message.StateRoot,
			Attestations: dest.Message.Body.Attestations,
			RandaoReveal: dest.Message.Body.RandaoReveal,
		}, nil
	}
}

func (s *Server) getBlockRoot(slot common.Slot) (common.Root, error) {
	if slot == 0 {
		return s.genesisBlockRoot, nil // not stored in actual db, not really a full block either, just the root of latest block header in genesis state
	}
	var key [3 + 8]byte
	copy(key[:3], KeyBlockRoot)
	binary.BigEndian.PutUint64(key[3:], uint64(slot))
	root, err := s.blocks.Get(key[:], nil)
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

func (s *Server) lastSlot() (common.Slot, common.Root, error) {
	blockIter := s.blocks.NewIterator(util.BytesPrefix([]byte(KeyBlockRoot)), nil)
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
		return 0, s.genesisBlockRoot, nil
	}
}

func (s *Server) updateBlocksMaybe() error {
	lastSlot, lastBlockRoot, err := s.lastSlot()
	if err != nil {
		return fmt.Errorf("cannot start importing blocks without block progress info: %v", err)
	}
	commonSlot := lastSlot
	commonBlockRoot := lastBlockRoot
	// walk back until we are on the canonical chain (instant if already on the canonical chain)
	for slot := lastSlot; slot > 0; slot-- {

		localRoot, err := s.getBlockRoot(slot)
		if err == ErrBlockNotFound {
			continue
		} else if err != nil {
			return fmt.Errorf("failed to fetch local block root from db at slot %d: %v", slot, err)
		}

		ctx, cancel := context.WithTimeout(s.ctx, time.Second*10)
		remoteRoot, exists, err := beaconapi.BlockRoot(ctx, s.beaconCl, eth2api.BlockIdSlot(slot))
		cancel()
		if err != nil {
			return fmt.Errorf("failed to fetch next beacon block root at slot %d: %v", slot, err)
		} else if !exists {
			// if the remote root does not exist (e.g. gap slot), then continue traversing back further,
			// but allow commonSlot to stay the way it is
			continue
		}

		if remoteRoot != localRoot {
			// if the roots are different, traverse back further, and reset back commonSlot
			commonSlot = slot - 1
			continue
		}

		commonBlockRoot = localRoot
		break
	}
	// if we had to rewind back to handle a reorg, then reset any invalidated processing work
	if commonSlot < lastSlot {
		if err := s.resetBlocks(commonSlot, lastSlot); err != nil {
			return fmt.Errorf("failed to reset blocks: %v", err)
		}

		if err := s.resetRandaoMixData(commonSlot); err != nil {
			return fmt.Errorf("failed to reset randao data upon reorg: %v", err)
		}

		s.resetIndicesBounded()

		if err := s.resetPerf(commonSlot); err != nil {
			return fmt.Errorf("failed to reset perf data upon reorg: %v", err)
		}

		if err := s.resetTiles(commonSlot); err != nil {
			return fmt.Errorf("failed to reset tile data upon reorg: %v", err)
		}
		// return, we can do the rest of the import work in the next sync step
		return nil
	}

	// fetch the block for that we'd like to import
	nextSlot := commonSlot + 1
	ctx, cancel := context.WithTimeout(s.ctx, time.Second*10)
	var dest eth2api.VersionedSignedBeaconBlock
	exists, err := beaconapi.BlockV2(ctx, s.beaconCl, eth2api.BlockIdSlot(nextSlot), &dest)
	cancel()

	if err != nil {
		return fmt.Errorf("failed to fetch beacon block at slot %d: %v", nextSlot, err)
	} else if !exists {
		// If there exists no block, then copy the last block root instead.
		// It will be reorged out if there really is a block.
		// But we'll fill gaps with block roots always
		var key [3 + 8]byte
		copy(key[:3], KeyBlockRoot)
		binary.BigEndian.PutUint64(key[3:], uint64(nextSlot))
		if err := s.blocks.Put(key[:], commonBlockRoot[:], nil); err != nil {
			return fmt.Errorf("failed to write gap-filling block root %s at slot %d: %v", commonBlockRoot, nextSlot, err)
		}
		return nil
	}

	// envelop it so it's nice to work with
	forkDigest := s.forks.ForkDigest(s.spec.SlotToEpoch(nextSlot))
	block := dest.Data.Envelope(s.spec, forkDigest)

	// check if there was a reorg between the moment of finding a common chain, and retrieving the next block
	if block.ParentRoot != commonBlockRoot {
		return fmt.Errorf("block import needs to start over, reorg detected after finding common block at slot %d with root %s, now expected parent root %s from next block: %s",
			commonSlot, commonBlockRoot, block.ParentRoot, block.BlockRoot)
	}

	var batch leveldb.Batch
	{
		var key [3 + 8]byte
		copy(key[:3], KeyBlock)
		binary.BigEndian.PutUint64(key[3:], uint64(block.Slot))
		var buf bytes.Buffer
		if err := block.Serialize(codec.NewEncodingWriter(&buf)); err != nil {
			return fmt.Errorf("failed to encode new block %s at slot %d: %v", block.BlockRoot, block.Slot, err)
		}
		batch.Put(key[:], buf.Bytes())
	}
	{
		var key [3 + 8]byte
		copy(key[:3], KeyBlockRoot)
		binary.BigEndian.PutUint64(key[3:], uint64(block.Slot))
		batch.Put(key[:], block.BlockRoot[:])
	}
	if err := s.blocks.Write(&batch, nil); err != nil {
		return fmt.Errorf("failed to import new block %s at slot %d: %v", block.BlockRoot, block.Slot, err)
	}
	s.log.Info("imported block", "slot", block.Slot)
	return nil
}

func (s *Server) resetBlocks(slot common.Slot, prev common.Slot) error {
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

		iter := s.blocks.NewIterator(keyRange, nil)
		defer iter.Release()

		for iter.Next() {
			batch.Delete(iter.Key())
		}
	}

	deletePrefix(KeyBlock)
	deletePrefix(KeyBlockRoot)

	if err := s.blocks.Write(&batch, nil); err != nil {
		return fmt.Errorf("failed to cleanup conflicting block data: %v", err)
	}
	return nil
}
