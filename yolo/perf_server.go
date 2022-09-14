package yolo

import (
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"io"
)

func (s *Server) getPerf(currEp common.Epoch) ([]ValidatorPerformance, error) {
	return getPerf(s.perf, currEp)
}

func (s *Server) lastPerfEpoch() (common.Epoch, error) {
	return lastPerfEpoch(s.perf)
}

func (s *Server) updatePerfMaybe() error {
	lastSlot, _, err := s.lastSlot()
	if err != nil {
		return err
	}
	perfEpoch, err := s.lastPerfEpoch()
	if err != nil {
		return err
	}
	lastEpoch := s.spec.SlotToEpoch(lastSlot)
	if lastEpoch > perfEpoch {
		if perfEpoch%100 == 0 {
			s.log.Info("updating performance data", "epoch", perfEpoch+1)
		}
		return processPerf(s.perf, s.spec, s.blocks, s.randao, s.indicesBounded, perfEpoch+1)
	}
	return io.EOF
}

func (s *Server) resetPerf(resetSlot common.Slot) error {
	return resetPerf(s.perf, s.spec, resetSlot)
}
