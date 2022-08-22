package yolo

import (
	"context"
	"fmt"
	"github.com/protolambda/eth2api"
	"github.com/protolambda/eth2api/client/beaconapi"
	"github.com/protolambda/zrnt/eth2/beacon/common"
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
	var resp []eth2api.ValidatorResponse

	if exists, err := beaconapi.StateValidators(ctx, s.beaconCl, eth2api.StateHead, nil, nil, &resp); err != nil {
		return fmt.Errorf("failed to fetch validators data: %v", err)
	} else if !exists {
		return fmt.Errorf("validators data not available: %v", err)
	}
	cancel()

	indices := make([]common.BoundedIndex, len(resp))
	for i, v := range resp {
		if v.Index != common.ValidatorIndex(i) {
			return fmt.Errorf("inconsistent returned validators info: %d <> %d", i, v.Index)
		}
		indices[i] = common.BoundedIndex{
			Index:      common.ValidatorIndex(i),
			Activation: v.Validator.ActivationEpoch,
			Exit:       v.Validator.ExitEpoch,
		}
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
