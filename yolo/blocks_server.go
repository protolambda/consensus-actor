package yolo

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/protolambda/eth2api"
	"github.com/protolambda/eth2api/client/beaconapi"
	"github.com/protolambda/zrnt/eth2/beacon/altair"
	"github.com/protolambda/zrnt/eth2/beacon/bellatrix"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/beacon/phase0"
	"github.com/protolambda/ztyp/tree"
	"github.com/syndtr/goleveldb/leveldb"
)

func (s *Server) getBlock(slot common.Slot) (*BlockData, error) {
	return getBlock(s.blocks, s.lhChainSnapshot, s.spec, slot)
}

func (s *Server) getBlockRoot(slot common.Slot) (common.Root, error) {
	return getBlockRoot(s.blocks, slot)
}

func (s *Server) lastSlot() (common.Slot, common.Root, error) {
	return lastSlotAndRoot(s.blocks, s.genesisBlockRoot)
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

	var parentRoot common.Root
	var blockRoot common.Root
	var blockSlot common.Slot
	switch sbl := dest.Data.(type) {
	case *phase0.SignedBeaconBlock:
		parentRoot = sbl.Message.ParentRoot
		blockSlot = sbl.Message.Slot
		blockRoot = sbl.Message.HashTreeRoot(s.spec, tree.GetHashFn())
	case *altair.SignedBeaconBlock:
		parentRoot = sbl.Message.ParentRoot
		blockSlot = sbl.Message.Slot
		blockRoot = sbl.Message.HashTreeRoot(s.spec, tree.GetHashFn())
	case *bellatrix.SignedBeaconBlock:
		parentRoot = sbl.Message.ParentRoot
		blockSlot = sbl.Message.Slot
		blockRoot = sbl.Message.HashTreeRoot(s.spec, tree.GetHashFn())
		bod := &sbl.Message.Body
		// API only serves full blocks, but we only want the blinded version, like lighthouse.
		// Standard API should really get a block retrieval method with blinded block data
		dest.Data = &SignedBeaconBlockLH{
			Message: BeaconBlockLH{
				Slot:          sbl.Message.Slot,
				ProposerIndex: sbl.Message.ProposerIndex,
				ParentRoot:    sbl.Message.ParentRoot,
				StateRoot:     sbl.Message.StateRoot,
				Body: BeaconBlockBodyLH{
					RandaoReveal:      bod.RandaoReveal,
					Eth1Data:          bod.Eth1Data,
					Graffiti:          bod.Graffiti,
					ProposerSlashings: bod.ProposerSlashings,
					AttesterSlashings: bod.AttesterSlashings,
					Attestations:      bod.Attestations,
					Deposits:          bod.Deposits,
					VoluntaryExits:    bod.VoluntaryExits,
					SyncAggregate:     bod.SyncAggregate,
					ExecutionPayload:  *bod.ExecutionPayload.Header(s.spec),
				},
			},
			Signature: sbl.Signature,
		}
	}

	// check if there was a reorg between the moment of finding a common chain, and retrieving the next block
	if parentRoot != commonBlockRoot {
		return fmt.Errorf("block import needs to start over, reorg detected after finding common block at slot %d with root %s, now expected parent root %s from next block: %s",
			commonSlot, commonBlockRoot, parentRoot, blockRoot)
	}

	// TODO: nowhere to import blocks to, we rely on lighthouse db
	var batch leveldb.Batch
	//{
	//	var key [3 + 8]byte
	//	copy(key[:3], KeyBlock)
	//	binary.BigEndian.PutUint64(key[3:], uint64(blockSlot))
	//	var buf bytes.Buffer
	//	if err := dest.Data.Serialize(s.spec, codec.NewEncodingWriter(&buf)); err != nil {
	//		return fmt.Errorf("failed to encode new block %s at slot %d: %v", blockRoot, blockSlot, err)
	//	}
	//	batch.Put(key[:], buf.Bytes())
	//}
	{
		var key [3 + 8]byte
		copy(key[:3], KeyBlockRoot)
		binary.BigEndian.PutUint64(key[3:], uint64(blockSlot))
		batch.Put(key[:], blockRoot[:])
	}
	if err := s.blocks.Write(&batch, nil); err != nil {
		return fmt.Errorf("failed to import new block %s at slot %d: %v", blockRoot, blockSlot, err)
	}
	s.log.Info("imported block", "slot", blockSlot)
	return nil
}

func (s *Server) resetBlocks(slot common.Slot, prev common.Slot) error {
	return resetBlocks(s.blocks, slot, prev)
}
