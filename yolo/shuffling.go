package yolo

import (
	"encoding/binary"
	"fmt"
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
	// The value is a 32 byte mix, XOR'ing all past randao inputs together, as defined in the consensus spec.
	KeyRandaoMix string = "rnd"
)

func (s *Server) updateRandaoMaybe() error {
	lastSlot, _, err := s.lastSlot()
	if err != nil {
		return fmt.Errorf("failed to get block progress for randao update check: %v", err)
	}

	epoch, err := s.lastRandaoEpoch()
	if err != nil {
		return fmt.Errorf("failed to get randao progress: %v", err)
	}
	// go to next epoch
	epoch += 1

	blocksEpoch := s.spec.SlotToEpoch(lastSlot)

	// check if there are enough new blocks to update an epoch
	if epoch < blocksEpoch {
		return nil
	}

	prevEpoch := epoch - 1
	prevMix, err := s.getRandao(prevEpoch)
	if err != nil {
		return fmt.Errorf("failed to get previous randao mix: %v", err)
	}
	mix := prevMix

	startSlot, err := s.spec.EpochStartSlot(epoch)
	if err != nil {
		return fmt.Errorf("no start slot: %v", err)
	}
	endSlot := startSlot + s.spec.SLOTS_PER_EPOCH - 1

	for slot := startSlot; slot <= endSlot; slot++ {
		dat, err := s.getBlock(slot)
		if err == ErrBlockNotFound {
			continue
		}
		mix = hashing.XorBytes32(mix, hashing.Hash(dat.RandaoReveal[:]))
	}
	var batch leveldb.Batch
	{
		// store the mix
		var key [3 + 8]byte
		copy(key[:3], KeyRandaoMix)
		binary.BigEndian.PutUint64(key[3:], uint64(epoch))
		batch.Put(key[:], mix[:])
	}
	if err := s.blocks.Write(&batch, nil); err != nil {
		return fmt.Errorf("failed to write randao mix of epoch %d to db: %v", epoch, err)
	}
	s.log.Info("updated randao mixes", "epoch", epoch)
	return nil
}

func (s *Server) getRandao(epoch common.Epoch) ([32]byte, error) {
	if epoch == 0 {
		return s.genesisRandao, nil
	}
	var key [3 + 8]byte
	copy(key[:3], KeyRandaoMix)
	// lookahead applies
	if epoch > 0 {
		epoch -= 1
	}
	binary.BigEndian.PutUint64(key[3:], uint64(epoch))
	var out [32]byte
	v, err := s.blocks.Get(key[:], nil)
	copy(out[:], v)
	return out, err
}

func (s *Server) shufflingSeed(epoch common.Epoch) ([32]byte, error) {
	buf := make([]byte, 4+8+32)

	// domain type
	copy(buf[0:4], common.DOMAIN_BEACON_ATTESTER[:])

	// epoch
	binary.LittleEndian.PutUint64(buf[4:4+8], uint64(epoch))
	mix, err := s.getRandao(epoch)
	if err != nil {
		return [32]byte{}, err
	}
	copy(buf[4+8:], mix[:])

	return hashing.Hash(buf), nil
}

func (s *Server) shuffling(epoch common.Epoch) (*common.ShufflingEpoch, error) {
	seed, err := s.shufflingSeed(epoch)
	if err != nil {
		return nil, fmt.Errorf("failed to compute seed: %v", err)
	}
	return common.NewShufflingEpoch(s.spec, s.indicesBounded, seed, epoch), nil
}

func (s *Server) lastRandaoEpoch() (common.Epoch, error) {
	iter := s.blocks.NewIterator(util.BytesPrefix([]byte(KeyRandaoMix)), nil)
	defer iter.Release()
	if iter.Last() {
		epoch := common.Epoch(binary.BigEndian.Uint64(iter.Key()[3:]))
		return epoch, nil
	} else {
		return 0, iter.Error()
	}
}

func (s *Server) resetRandaoMixData(resetSlot common.Slot) error {
	prevEpoch, err := s.lastRandaoEpoch()
	if err != nil {
		return err
	}
	if prevEpoch < s.spec.SlotToEpoch(resetSlot) {
		return nil
	}

	prefix := []byte(KeyRandaoMix)
	start := uint64(s.spec.SlotToEpoch(resetSlot)) + 1
	end := uint64(prevEpoch) + 1

	keyRange := &util.Range{
		Start: make([]byte, 3+8),
		Limit: make([]byte, 3+8),
	}
	copy(keyRange.Start[:3], prefix)
	binary.BigEndian.PutUint64(keyRange.Start[3:], start)
	copy(keyRange.Limit[:3], prefix)
	binary.BigEndian.PutUint64(keyRange.Limit[3:], end)

	iter := s.blocks.NewIterator(keyRange, nil)
	defer iter.Release()

	var batch leveldb.Batch
	for iter.Next() {
		batch.Delete(iter.Key())
	}

	if err := s.blocks.Write(&batch, nil); err != nil {
		return fmt.Errorf("failed to cleanup conflicting randao mix data with key %v", err)
	}

	return nil
}
