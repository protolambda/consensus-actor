package yolo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/golang/snappy"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	// KeyPerf is a:
	// 3 byte prefix for per-epoch performance keying, followed by:
	// 8 byte big-endian epoch value. (big endian to make db byte-prefix iteration and range-slices follow epoch order)
	//
	// The epoch key represents the boundary when the data became available.
	// I.e. epoch == 2 means that 0 == prev and 1 == current were processed.
	//
	// Values under this key are snappy block-compressed.
	//
	// The value is a []ValidatorPerformance
	KeyPerf string = "prf"
)

type ValidatorPerformance uint32

const (
	// and the next 64 values (6 bits). Always non-zero
	InclusionDistance ValidatorPerformance = 0x00_00_01_00

	InclusionDistanceMask = 0x00_00_ff_00

	// source is always correct, or wouldn't be included on-chain
	TargetCorrect ValidatorPerformance = 0x00_ff_00_00

	// up to 64, or 0xff if unknown
	HeadDistance ValidatorPerformance = 0x01_00_00_00

	ValidatorExists ValidatorPerformance = 0x00_00_00_01
)

// with 1 epoch delay (inclusion can be delayed), check validator performance
// if currEp == 0, then process nothing
// if currEp == 1, then process 0 and 1, filtered for target == 0
// if currEp == 2, then process 1 and 2, filtered for target == 1
// etc.
func processPerf(perfDB *leveldb.DB, spec *common.Spec, blocksDB *leveldb.DB, randaoDB *leveldb.DB, indicesBounded []common.BoundedIndex, currEp common.Epoch) error {
	if currEp == 0 {
		return errors.New("epoch 0 should never be processed, performance data is only available with epoch 1 completed")
	}

	// don't have to re-hash the block if we just load the hashes

	// get all block roots in previous and current epoch (or just current if genesis)
	var roots []common.Root

	// clips to start
	prevEp := currEp.Previous()
	prevStart, err := spec.EpochStartSlot(prevEp)
	if err != nil {
		return fmt.Errorf("bad epoch start slot of prev epoch: %w", err)
	}

	count := spec.SLOTS_PER_EPOCH * 2

	for i := common.Slot(0); i < count; i++ {
		slot := prevStart + i
		blockRoot, err := getBlockRoot(blocksDB, slot)
		if err != nil {
			return fmt.Errorf("failed to get block root of slot: %d", slot)
		}
		roots = append(roots, blockRoot)
	}

	// get all blocks in previous epoch
	blocks := make([]*BlockData, 0, spec.SLOTS_PER_EPOCH)
	for i := common.Slot(0); i < spec.SLOTS_PER_EPOCH; i++ {
		if b, err := getBlock(blocksDB, spec, prevStart+i); err == ErrBlockNotFound {
			continue
		} else if err != nil {
			return fmt.Errorf("failed to get block at slot %d: %v", prevStart+i, err)
		} else {
			blocks = append(blocks, b)
		}
	}

	prevShuf, err := shuffling(spec, randaoDB, indicesBounded, prevEp)
	if err != nil {
		return fmt.Errorf("failed to get shuffling for epoch %d: %v", prevEp, err)
	}

	// figure out how much space we need. There may be some gaps,
	// if validators didn't immediately activate, those values will just be 0
	maxValidatorIndex := common.ValidatorIndex(0)
	for _, vi := range prevShuf.ActiveIndices {
		if vi > maxValidatorIndex {
			maxValidatorIndex = vi
		}
	}
	// per validator, track who was already included for work this epoch
	validatorPerfs := make([]ValidatorPerformance, maxValidatorIndex+1)
	for i := range validatorPerfs {
		validatorPerfs[i] = ValidatorExists
	}
	// TODO: second perf array, in order of committees, so next stage doesn't deal with shuffling
	///      and per slot / committee index, instead of per epoch

	expectedTargetRoot := roots[0]

	// early blocks first, previous epoch (if any), then current epoch
	for _, bl := range blocks {
		for _, att := range bl.Attestations {
			// skip newer attestations. Anyone who votes for the same target epoch in two conflicting ways is slashable,
			// and although it is accounted for in performance on-chain, we ignore it here.
			if att.Data.Target.Epoch != prevEp {
				continue
			}

			perf := ValidatorExists
			// target performance
			if expectedTargetRoot == att.Data.Target.Root {
				perf |= TargetCorrect
			}

			// head accuracy
			headDist := 1
			found := false
			for i := int(att.Data.Slot); i >= int(prevStart); i-- {
				if att.Data.BeaconBlockRoot != roots[i-int(prevStart)] {
					headDist++
				} else {
					found = true
					break
				}
			}
			if !found {
				headDist = 0xff
			}
			perf |= HeadDistance * ValidatorPerformance(headDist)

			// inclusion distance
			perf |= InclusionDistance * ValidatorPerformance(bl.Slot-att.Data.Slot)

			comm := prevShuf.Committees[att.Data.Slot-prevStart][att.Data.Index]
			for bitIndex, valIndex := range comm {
				if bl := att.AggregationBits.BitLen(); bl != uint64(len(comm)) {
					return fmt.Errorf("unexpected attestation bitfield length: %d (expected %d) in epoch %d", bl, len(comm), prevEp)
				}
				if att.AggregationBits.GetBit(uint64(bitIndex)) {
					// only if the validator was not already seen
					if validatorPerfs[valIndex]&InclusionDistanceMask == 0 {
						validatorPerfs[valIndex] = perf
					}
				}
			}
		}
	}
	out := make([]byte, len(validatorPerfs)*4)
	for i, v := range validatorPerfs {
		binary.LittleEndian.PutUint32(out[i*4:i*4+4], uint32(v))
	}

	// compress the output (validators often behave the same, and there are a lot of them)
	out = snappy.Encode(nil, out)

	var outKey [3 + 8]byte
	copy(outKey[:3], KeyPerf)
	binary.BigEndian.PutUint64(outKey[3:], uint64(currEp))
	if err := perfDB.Put(outKey[:], out, nil); err != nil {
		return fmt.Errorf("failed to store epoch performance")
	}
	return nil
}

func getPerf(perfDB *leveldb.DB, currEp common.Epoch) ([]ValidatorPerformance, error) {
	var key [3 + 8]byte
	copy(key[:3], KeyPerf)
	binary.BigEndian.PutUint64(key[3:], uint64(currEp))
	out, err := perfDB.Get(key[:], nil)
	if err != nil {
		return nil, err
	}
	out, err = snappy.Decode(nil, out)
	if err != nil {
		return nil, err
	}
	perf := make([]ValidatorPerformance, len(out)/4)
	for i := 0; i < len(out); i += 4 {
		perf[i/4] = ValidatorPerformance(binary.LittleEndian.Uint32(out[i : i+4]))
	}
	return perf, nil
}

func lastPerfEpoch(perfDB *leveldb.DB) (common.Epoch, error) {
	iter := perfDB.NewIterator(util.BytesPrefix([]byte(KeyPerf)), nil)
	defer iter.Release()
	if iter.Last() {
		epoch := common.Epoch(binary.BigEndian.Uint64(iter.Key()[3:]))
		return epoch, nil
	} else {
		return 0, iter.Error()
	}
}

func resetPerf(perfDB *leveldb.DB, spec *common.Spec, resetSlot common.Slot) error {
	ep, err := lastPerfEpoch(perfDB)
	if err != nil {
		return err
	}
	if ep < spec.SlotToEpoch(resetSlot) {
		return nil
	}

	prefix := []byte(KeyPerf)
	start := uint64(spec.SlotToEpoch(resetSlot))
	end := uint64(ep) + 1

	keyRange := &util.Range{
		Start: make([]byte, 3+8),
		Limit: make([]byte, 3+8),
	}
	copy(keyRange.Start[:3], prefix)
	binary.BigEndian.PutUint64(keyRange.Start[3:], start)
	copy(keyRange.Limit[:3], prefix)
	binary.BigEndian.PutUint64(keyRange.Limit[3:], end)

	iter := perfDB.NewIterator(keyRange, nil)
	defer iter.Release()

	var batch leveldb.Batch
	for iter.Next() {
		batch.Delete(iter.Key())
	}

	if err := perfDB.Write(&batch, nil); err != nil {
		return fmt.Errorf("failed to cleanup conflicting perf mix data with key %v", err)
	}

	return nil
}
