package yolo

import (
	"encoding/binary"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/util/hashing"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

const (
	// KeyRandaoMix is a:
	// 3 byte prefix for keying randao mixes, followed by:
	// 8 byte big-endian epoch number
	//
	// The epoch number represents the epoch that the mix was completed at.
	// I.e. block 1,2,3...31 randao reveals are mixed into the mix at epoch 1.
	// The mix at epoch 0 is not stored, but instead loaded as special genesis mix value.
	//
	// The value is a 32 byte mix, XOR'ing all past randao inputs together, as defined in the consensus spec.
	KeyRandaoMix string = "rnd"
)

func updateRandao(log log.Logger, spec *common.Spec, randaoDB *leveldb.DB, blocks *leveldb.DB, prevEpoch common.Epoch) error {
	// with look-ahead
	prevMix, err := getRandao(randaoDB, prevEpoch)
	if err != nil {
		return fmt.Errorf("failed to get previous randao mix: %v", err)
	}
	mix := prevMix

	startSlot, err := spec.EpochStartSlot(prevEpoch)
	if err != nil {
		return fmt.Errorf("no start slot: %v", err)
	}
	endSlot := startSlot + spec.SLOTS_PER_EPOCH - 1

	for slot := startSlot; slot <= endSlot; slot++ {
		if slot == 0 {
			continue
		}
		dat, err := getBlock(blocks, spec, slot)
		if err == ErrBlockNotFound {
			log.Info("skipping gap block in randao processing", "slot", slot)
			continue
		}
		mix = hashing.XorBytes32(mix, hashing.Hash(dat.RandaoReveal[:]))
	}
	epoch := prevEpoch + 1
	var batch leveldb.Batch
	{
		// store the mix
		var key [3 + 8]byte
		copy(key[:3], KeyRandaoMix)
		binary.BigEndian.PutUint64(key[3:], uint64(epoch))
		batch.Put(key[:], mix[:])
	}
	if err := randaoDB.Write(&batch, nil); err != nil {
		return fmt.Errorf("failed to write randao mix of epoch %d to db: %v", epoch, err)
	}
	return nil
}

func getRandao(db *leveldb.DB, epoch common.Epoch) ([32]byte, error) {
	var key [3 + 8]byte
	copy(key[:3], KeyRandaoMix)
	binary.BigEndian.PutUint64(key[3:], uint64(epoch))
	var out [32]byte
	v, err := db.Get(key[:], nil)
	copy(out[:], v)
	return out, err
}

func shufflingSeed(spec *common.Spec, randaoDB *leveldb.DB, epoch common.Epoch) ([32]byte, error) {
	buf := make([]byte, 4+8+32)

	// domain type
	copy(buf[0:4], common.DOMAIN_BEACON_ATTESTER[:])

	// epoch
	binary.LittleEndian.PutUint64(buf[4:4+8], uint64(epoch))
	// apply lookahead to rando lookup
	if epoch >= spec.MIN_SEED_LOOKAHEAD {
		epoch -= spec.MIN_SEED_LOOKAHEAD
	}
	// And no need for the -1 like the spec,
	// we store the randao mix not at the epoch of the blocks it was created with, but the epoch after.
	mix, err := getRandao(randaoDB, epoch)
	if err != nil {
		return [32]byte{}, err
	}
	copy(buf[4+8:], mix[:])

	return hashing.Hash(buf), nil
}

func shuffling(spec *common.Spec, randaoDB *leveldb.DB, indicesBounded []common.BoundedIndex, epoch common.Epoch) (*common.ShufflingEpoch, error) {
	seed, err := shufflingSeed(spec, randaoDB, epoch)
	if err != nil {
		return nil, fmt.Errorf("failed to compute seed: %v", err)
	}
	return common.NewShufflingEpoch(spec, indicesBounded, seed, epoch), nil
}

func lastRandaoEpoch(db *leveldb.DB) (common.Epoch, error) {
	iter := db.NewIterator(util.BytesPrefix([]byte(KeyRandaoMix)), nil)
	defer iter.Release()
	if iter.Last() {
		epoch := common.Epoch(binary.BigEndian.Uint64(iter.Key()[3:]))
		return epoch, nil
	} else {
		err := iter.Error()
		if err != nil {
			return 0, err
		}
		return 0, nil
	}
}

func resetRandaoMixData(randaoDB *leveldb.DB, spec *common.Spec, resetSlot common.Slot) error {
	prevEpoch, err := lastRandaoEpoch(randaoDB)
	if err != nil {
		return err
	}
	if prevEpoch < spec.SlotToEpoch(resetSlot) {
		return nil
	}

	prefix := []byte(KeyRandaoMix)
	start := uint64(spec.SlotToEpoch(resetSlot)) + 1
	end := uint64(prevEpoch) + 1

	keyRange := &util.Range{
		Start: make([]byte, 3+8),
		Limit: make([]byte, 3+8),
	}
	copy(keyRange.Start[:3], prefix)
	binary.BigEndian.PutUint64(keyRange.Start[3:], start)
	copy(keyRange.Limit[:3], prefix)
	binary.BigEndian.PutUint64(keyRange.Limit[3:], end)

	iter := randaoDB.NewIterator(keyRange, nil)
	defer iter.Release()

	var batch leveldb.Batch
	for iter.Next() {
		batch.Delete(iter.Key())
	}

	if err := randaoDB.Write(&batch, nil); err != nil {
		return fmt.Errorf("failed to cleanup conflicting randao mix data with key %v", err)
	}

	return nil
}
