package yolo

import (
	"fmt"
	"io"

	"github.com/protolambda/zrnt/eth2/beacon/common"
)

func (s *Server) updateRandaoMaybe() error {
	lastSlot, _, err := s.lastSlot()
	if err != nil {
		return fmt.Errorf("failed to get block progress for randao update check: %v", err)
	}

	prevEpoch, err := s.lastRandaoEpoch()
	if err != nil {
		return fmt.Errorf("failed to get randao progress: %v", err)
	}

	blocksEpoch := s.spec.SlotToEpoch(lastSlot)

	// check if there are enough new blocks to update an epoch
	if prevEpoch >= blocksEpoch {
		return io.EOF
	}
	// TODO
	//return updateRandao(s.log, s.spec, s.randao, s.blocks, s.lhChainSnapshot, prevEpoch)
	return nil
}

func (s *Server) getRandao(epoch common.Epoch) ([32]byte, error) {
	return getRandao(s.randao, epoch)
}

func (s *Server) shuffling(epoch common.Epoch) (*common.ShufflingEpoch, error) {
	return shuffling(s.spec, s.randao, s.indicesBounded, epoch)
}

func (s *Server) lastRandaoEpoch() (common.Epoch, error) {
	return lastRandaoEpoch(s.randao)
}

func (s *Server) resetRandaoMixData(resetSlot common.Slot) error {
	return resetRandaoMixData(s.randao, s.spec, resetSlot)
}
