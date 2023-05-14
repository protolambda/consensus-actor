package fun

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/log"
	"github.com/golang/snappy"
	"github.com/protolambda/zrnt/eth2/beacon/altair"
	"github.com/protolambda/zrnt/eth2/beacon/bellatrix"
	"github.com/protolambda/zrnt/eth2/beacon/capella"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/beacon/phase0"
	"github.com/protolambda/zrnt/eth2/util/hashing"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/protolambda/consensus-actor/fun/era"
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

func shufflingSeed(spec *common.Spec, randaoFn RandaoLookup, epoch common.Epoch) ([32]byte, error) {
	buf := make([]byte, 4+8+32)

	// domain type
	copy(buf[0:4], common.DOMAIN_BEACON_ATTESTER[:])

	// epoch
	binary.LittleEndian.PutUint64(buf[4:4+8], uint64(epoch))

	mix, err := randaoFn(epoch)
	if err != nil {
		return [32]byte{}, err
	}
	copy(buf[4+8:], mix[:])

	return hashing.Hash(buf), nil
}

func shuffling(spec *common.Spec, randaoFn RandaoLookup, indicesBounded []common.BoundedIndex, epoch common.Epoch) (*common.ShufflingEpoch, error) {
	seed, err := shufflingSeed(spec, randaoFn, epoch)
	if err != nil {
		return nil, fmt.Errorf("failed to compute seed: %v", err)
	}
	return common.NewShufflingEpoch(spec, indicesBounded, seed, epoch), nil
}

// with 1 epoch delay (inclusion can be delayed), check validator performance
// if currEp == 0, then process only 0, filtered for target == 0
// if currEp == 1, then process 0 and 1, filtered for target == 0
// if currEp == 2, then process 1 and 2, filtered for target == 1
// etc.
func processPerf(spec *common.Spec,
	blockRootFn BlockRootLookup,
	attFn AttestationsLookup, randaoFn RandaoLookup,
	indicesBounded []common.BoundedIndex, currEp common.Epoch) ([]ValidatorPerformance, error) {
	// don't have to re-hash the block if we just load the hashes

	// get all block roots in previous and current epoch (or just current if genesis)
	var roots []common.Root

	// clips to start
	prevEp := currEp.Previous()
	prevStart, err := spec.EpochStartSlot(prevEp)
	if err != nil {
		return nil, fmt.Errorf("bad epoch start slot of prev epoch: %w", err)
	}

	count := spec.SLOTS_PER_EPOCH * 2
	if prevEp == currEp {
		count = spec.SLOTS_PER_EPOCH
	}

	for i := common.Slot(0); i < spec.SLOTS_PER_EPOCH; i++ {
		slot := prevStart + i
		blockRoot, err := blockRootFn(slot)
		if err != nil {
			return nil, fmt.Errorf("failed to get block root of slot: %d", slot)
		}
		roots = append(roots, blockRoot)
	}

	// get all blocks in previous and/or current epoch
	blocks := make([]SlotAttestations, 0, count)
	for i := common.Slot(0); i < count; i++ {
		slot := prevStart + i
		if atts, err := attFn(slot); err != nil {
			return nil, fmt.Errorf("failed to get block at slot %d: %v", slot, err)
		} else {
			blocks = append(blocks, SlotAttestations{Slot: slot, Attestations: atts})
		}
	}

	prevShuf, err := shuffling(spec, randaoFn, indicesBounded, prevEp)
	if err != nil {
		return nil, fmt.Errorf("failed to get shuffling for epoch %d: %v", prevEp, err)
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
					return nil, fmt.Errorf("unexpected attestation bitfield length: %d (expected %d) in epoch %d", bl, len(comm), prevEp)
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
	return validatorPerfs, nil
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

type perfJob struct {
	start common.Epoch
	end   common.Epoch
}

func UpdatePerf(ctx context.Context, log log.Logger, perf *leveldb.DB, spec *common.Spec, st *era.Store, start, end common.Epoch, workers int) error {
	if end < start {
		return fmt.Errorf("invalid epoch range %d - %d", start, end)
	}
	epochsPerEra := common.Epoch(era.SlotsPerEra / spec.SLOTS_PER_EPOCH)
	log.Info("starting", "start_epoch", start, "end_epoch", end, "epochs_per_era", epochsPerEra)

	work := make(chan perfJob, workers)

	var wg sync.WaitGroup
	wg.Add(workers)

	ctx, cancelCause := context.WithCancelCause(ctx)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()

			log.Info("started worker", "i", i)

			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-work:
					if !ok {
						return
					}
					err := updateJob(ctx, log, perf, spec, st, job.start, job.end)
					if err != nil {
						cancelCause(fmt.Errorf("worker %d failed job (%d - %d): %w", i, job.start, job.end, err))
					}
				}
			}
		}(i)
	}

	// We can make jobs smaller than an era for more parallel work,
	// but then we just end up using more resources in total because of overhead, and we only have limited workers
	// TODO: consider scheduling smaller work jobs

	// schedule all the work
	go func() {
		for ep := start; ep < end; ep += epochsPerEra - (ep % epochsPerEra) {
			jobStart := ep
			jobEnd := ep + epochsPerEra
			if jobEnd > end {
				jobEnd = end
			}
			select {
			case work <- perfJob{start: jobStart, end: jobEnd}:
				continue
			case <-ctx.Done():
				wg.Wait() // wait for workers to all shut down
				return
			}
		}
		// signal all work has been scheduled
		close(work)
	}()

	// wait for all workers to shut down
	wg.Wait()

	if err := context.Cause(ctx); err != nil {
		log.Error("interrupted work", "err", err)
		return err
	}

	log.Info("finished", "start_epoch", start, "end_epoch", end)
	return nil
}

func updateJob(ctx context.Context, log log.Logger, perfDB *leveldb.DB, spec *common.Spec, st *era.Store, start, end common.Epoch) error {
	log.Info("starting job", "start_epoch", start, "end_epoch", end)

	if spec.SLOTS_PER_HISTORICAL_ROOT != era.SlotsPerEra {
		return fmt.Errorf("weird spec, expected %d slots per historical root: %w")
	}
	if start+era.SlotsPerEra < end {
		return fmt.Errorf("range too large: %d ... %d: %d diff", start, end, end-start)
	}

	epochsPerEra := common.Epoch(era.SlotsPerEra / spec.SLOTS_PER_EPOCH)
	currEraEpoch := end
	if rem := end % epochsPerEra; rem > 0 {
		currEraEpoch += epochsPerEra - rem
	}
	currEraSlot, _ := spec.EpochStartSlot(currEraEpoch)

	var currEraBlockRoots phase0.HistoricalBatchRoots
	var prevEraBlockRoots phase0.HistoricalBatchRoots
	var randaoMixes phase0.RandaoMixes

	var indicesBounded BoundedIndices

	if currEraEpoch < spec.ALTAIR_FORK_EPOCH {
		var state phase0.BeaconState
		if err := st.State(currEraSlot, spec.Wrap(&state)); err != nil {
			return err
		}
		currEraBlockRoots = state.BlockRoots
		randaoMixes = state.RandaoMixes
		indicesBounded = loadIndicesFromState(state.Validators)
	} else if currEraEpoch < spec.BELLATRIX_FORK_EPOCH {
		var state altair.BeaconState
		if err := st.State(currEraSlot, spec.Wrap(&state)); err != nil {
			return err
		}
		currEraBlockRoots = state.BlockRoots
		randaoMixes = state.RandaoMixes
		indicesBounded = loadIndicesFromState(state.Validators)
	} else if currEraEpoch < spec.CAPELLA_FORK_EPOCH {
		var state bellatrix.BeaconState
		if err := st.State(currEraSlot, spec.Wrap(&state)); err != nil {
			return err
		}
		currEraBlockRoots = state.BlockRoots
		randaoMixes = state.RandaoMixes
		indicesBounded = loadIndicesFromState(state.Validators)
	} else {
		var state capella.BeaconState
		if err := st.State(currEraSlot, spec.Wrap(&state)); err != nil {
			return err
		}
		currEraBlockRoots = state.BlockRoots
		randaoMixes = state.RandaoMixes
		indicesBounded = loadIndicesFromState(state.Validators)
	}

	if currEraEpoch >= epochsPerEra {
		prevEraEpoch := currEraEpoch - epochsPerEra
		prevEraSlot, _ := spec.EpochStartSlot(prevEraEpoch)
		if prevEraEpoch+2 >= start { // if the start is close to the era boundary, we'll need to load the prev era state.
			if prevEraEpoch < spec.ALTAIR_FORK_EPOCH {
				var state phase0.BeaconState
				if err := st.State(prevEraSlot, spec.Wrap(&state)); err != nil {
					return err
				}
				prevEraBlockRoots = state.BlockRoots
			} else if prevEraEpoch < spec.BELLATRIX_FORK_EPOCH {
				var state altair.BeaconState
				if err := st.State(prevEraSlot, spec.Wrap(&state)); err != nil {
					return err
				}
				prevEraBlockRoots = state.BlockRoots
			} else if prevEraEpoch < spec.CAPELLA_FORK_EPOCH {
				var state bellatrix.BeaconState
				if err := st.State(prevEraSlot, spec.Wrap(&state)); err != nil {
					return err
				}
				prevEraBlockRoots = state.BlockRoots
			} else {
				var state capella.BeaconState
				if err := st.State(prevEraSlot, spec.Wrap(&state)); err != nil {
					return err
				}
				prevEraBlockRoots = state.BlockRoots
			}
		}
	}

	blockRootFn := BlockRootLookup(func(slot common.Slot) (common.Root, error) {
		if slot > currEraSlot {
			return common.Root{}, fmt.Errorf("cannot get block root of slot %d, era stops at slot %d", slot, currEraSlot)
		}
		if slot+era.SlotsPerEra >= currEraSlot {
			return currEraBlockRoots[slot%era.SlotsPerEra], nil
		}
		if prevEraBlockRoots == nil {
			return common.Root{}, fmt.Errorf("no previous era block roots, cannot get block root of slot %d", slot)
		}
		if slot+era.SlotsPerEra*2 >= currEraSlot {
			return prevEraBlockRoots[slot%era.SlotsPerEra], nil
		}
		return common.Root{}, fmt.Errorf("slot %d too old to serve", slot)
	})

	attFn := AttestationsLookup(func(slot common.Slot) (phase0.Attestations, error) {
		if slot == 0 {
			return nil, nil
		}
		ep := spec.SlotToEpoch(slot)
		if ep < spec.ALTAIR_FORK_EPOCH {
			var block phase0.SignedBeaconBlock
			if err := st.Block(slot, spec.Wrap(&block)); errors.Is(err, era.ErrNotExist) {
				return nil, nil
			} else if err != nil {
				return nil, err
			}
			if slot != block.Message.Slot {
				return nil, fmt.Errorf("loaded wrong block, got slot %d, but requested %d", block.Message.Slot, slot)
			}
			return block.Message.Body.Attestations, nil
		} else if ep < spec.BELLATRIX_FORK_EPOCH {
			var block altair.SignedBeaconBlock
			if err := st.Block(slot, spec.Wrap(&block)); errors.Is(err, era.ErrNotExist) {
				return nil, nil
			} else if err != nil {
				return nil, err
			}
			if slot != block.Message.Slot {
				return nil, fmt.Errorf("loaded wrong block, got slot %d, but requested %d", block.Message.Slot, slot)
			}
			return block.Message.Body.Attestations, nil
		} else if ep < spec.CAPELLA_FORK_EPOCH {
			var block bellatrix.SignedBeaconBlock
			if err := st.Block(slot, spec.Wrap(&block)); errors.Is(err, era.ErrNotExist) {
				return nil, nil
			} else if err != nil {
				return nil, err
			}
			if slot != block.Message.Slot {
				return nil, fmt.Errorf("loaded wrong block, got slot %d, but requested %d", block.Message.Slot, slot)
			}
			return block.Message.Body.Attestations, nil
		} else {
			var block capella.SignedBeaconBlock
			if err := st.Block(slot, spec.Wrap(&block)); errors.Is(err, era.ErrNotExist) {
				return nil, nil
			} else if err != nil {
				return nil, err
			}
			if slot != block.Message.Slot {
				return nil, fmt.Errorf("loaded wrong block, got slot %d, but requested %d", block.Message.Slot, slot)
			}
			return block.Message.Body.Attestations, nil
		}
	})

	randaoFn := RandaoLookup(func(epoch common.Epoch) ([32]byte, error) {
		if epoch > currEraEpoch {
			return [32]byte{}, fmt.Errorf("epoch too high, cannot get randao mix of epoch %d from era state at epoch %d", epoch, currEraEpoch)
		}
		if epoch+spec.EPOCHS_PER_HISTORICAL_VECTOR < currEraEpoch {
			return [32]byte{}, fmt.Errorf("epoch too low, cannot get randao mix of epoch %d from era state at epoch %d", epoch, currEraEpoch)
		}
		i := epoch + spec.EPOCHS_PER_HISTORICAL_VECTOR - spec.MIN_SEED_LOOKAHEAD - 1
		return randaoMixes[i%spec.EPOCHS_PER_HISTORICAL_VECTOR], nil
	})

	for currEp := start; currEp < end; currEp++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("stopped before processing epoch %d: %w", currEp, err)
		}
		validatorPerfs, err := processPerf(spec, blockRootFn, attFn, randaoFn, indicesBounded, currEp)
		if err != nil {
			return fmt.Errorf("failed to process epoch %d: %w", currEp, err)
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
	}
	return nil
}
