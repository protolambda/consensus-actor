package yolo

import (
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"time"
)

func (s *Server) updateIndicesBoundedData() error {
	// TODO
	return nil
}

// Progress represents to which point in the chain we've processed so far
type Progress struct {
	BlockRoot common.Root
	Slot      common.Slot
}

func (s *Server) perfProgress() (*Progress, error) {
	// load progress from DB
	return nil, nil
}

func (s *Server) tilesProgress() (*Progress, error) {
	// load tiles from DB
	return nil, nil
}

func (s *Server) BlockRoot(slot common.Slot) (common.Root, error) {
	// TODO
	return common.Root{}, nil
}

func (s *Server) WallClockSlot() common.Slot {
	return s.spec.TimeToSlot(common.Timestamp(uint64(time.Now().Unix())), s.genesis.GenesisTime)
}

// syncStep does a little bit of work, like fetching data, processing an epoch, updating tiles, etc.
// This returns io.EOF if there is no work left
func (s *Server) syncStep() error {
	// note: we process blocks eagerly, before processing epochs, since we can reduce work that way.
	// similarly we try to process all epochs before we start processing tiles

	// TODO check if there's block processing to do
	if s.lastHeader == nil || s.WallClockSlot() > s.lastHeader.Slot {
		return s.importNextBlock()
	}

	//blockProgress := Progress{}

	// TODO check if the indicesBounded needs to be updated (i.e. if it doesn't match the latest block)
	if s.indicesBounded == nil {
		return s.updateIndicesBoundedData()
	}

	perfProgress, err := s.perfProgress()
	if err != nil {
		return err
	}

	// TODO check if block root is still canonical
	if root, err := s.BlockRoot(perfProgress.Slot); err != nil {
		return err
	} else if root != perfProgress.BlockRoot {
		// TODO: reorg
	}

	// TODO check if there's epoch processing to do

	// TODO check if there's tile processing to do

	return nil
}
