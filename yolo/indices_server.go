package yolo

import (
	"context"
	"io"
	"time"
)

func (s *Server) updateIndicesBoundedDataMaybe() error {
	lastSlot, _, err := s.lastSlot()
	if err != nil {
		return err
	}

	if s.indicesBounded != nil {
		// check if we are up to date
		lastEpoch := s.spec.SlotToEpoch(lastSlot)
		indicesEpoch := s.spec.SlotToEpoch(s.indicesSlot)

		// only update the indices data when the epoch changes, or when there's a reorg (resetIndicesBounded() will be called)
		if lastEpoch == indicesEpoch {
			return io.EOF
		}
	}

	s.indicesBounded = nil

	ctx, cancel := context.WithTimeout(s.ctx, time.Second*30)
	defer cancel()
	indices, err := fetchIndicesBounded(ctx, s.beaconCl)
	if err != nil {
		return err
	}

	s.indicesBounded = indices
	s.indicesSlot = lastSlot

	s.log.Info("updated bounded validator indices", "slot", lastSlot)
	return nil
}

func (s *Server) resetIndicesBounded() {
	// just download it again when we need to
	s.indicesBounded = nil
	s.indicesSlot = 0
}
