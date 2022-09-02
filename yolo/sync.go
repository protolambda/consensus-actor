package yolo

import (
	"io"
	"time"

	"github.com/protolambda/zrnt/eth2/beacon/common"
)

func (s *Server) WallClockSlot() common.Slot {
	return s.spec.TimeToSlot(common.Timestamp(uint64(time.Now().Unix())), s.genesis.GenesisTime)
}

// syncStep does a little bit of work, like fetching data, processing an epoch, updating tiles, etc.
// This returns io.EOF if there is no work left
func (s *Server) syncStep() error {
	// note: we process blocks eagerly, before processing epochs, since we can reduce work that way.
	// similarly we try to process all epochs before we start processing tiles

	// repeat: try update stage, unless we're done with updating that stage already,
	// then continue with next stage

	if !s.syncDisableBlocks {
		if err := s.updateBlocksMaybe(); err != io.EOF {
			return err
		}
	}

	if err := s.updateRandaoMaybe(); err != io.EOF {
		return err
	}

	if err := s.updateIndicesBoundedDataMaybe(); err != io.EOF {
		return err
	}

	if err := s.updatePerfMaybe(); err != io.EOF {
		return err
	}

	if err := s.updateTilesMaybe(); err != io.EOF {
		return err
	}

	// no more stages
	return io.EOF
}
