package yolo

import (
	"encoding/binary"
	"fmt"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/util/hashing"
)

func (s *Server) updateRandao(epoch common.Epoch) error {
	var mix [32]byte
	if epoch == 0 {
		// special genesis randao value
		mix = s.genesisRandao
	} else {
		prev := epoch - 1
		prevMix, err := s.getRandao(prev)
		if err != nil {
			return fmt.Errorf("failed to get previous randao mix: %v", err)
		}
		mix = prevMix
	}

	startSlot, err := s.spec.EpochStartSlot(epoch)
	if err != nil {
		return fmt.Errorf("no start slot: %v", err)
	}
	endSlot := startSlot + s.spec.SLOTS_PER_EPOCH

	for slot := startSlot; slot < endSlot; slot++ {
		dat, err := s.getBlock(slot)
		if err == ErrBlockNotFound {
			continue
		}
		mix = hashing.XorBytes32(mix, hashing.Hash(dat.RandaoReveal[:]))
	}
	// store the mix
	var key [3 + 8]byte
	copy(key[:3], KeyRandaoMix)
	binary.BigEndian.PutUint64(key[3:], uint64(epoch))
	if err := s.blocks.Put(key[:], mix[:], nil); err != nil {
		return fmt.Errorf("failed to write randao mix of epoch %d to db: %v", epoch, err)
	}
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
