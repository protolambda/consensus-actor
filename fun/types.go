package fun

import (
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/beacon/phase0"
)

type BoundedIndices []common.BoundedIndex

type RandaoLookup func(epoch common.Epoch) ([32]byte, error)

type BlockRootLookup func(slot common.Slot) (common.Root, error)

type AttestationsLookup func(slot common.Slot) (phase0.Attestations, error)

type BlockLookup func(slot uint64, dest common.SSZObj) error

type StateLookup func(slot uint64, dest common.SSZObj) error

type SlotAttestations struct {
	Slot         common.Slot
	Attestations phase0.Attestations
}

func loadIndicesFromState(validators phase0.ValidatorRegistry) BoundedIndices {
	indices := make([]common.BoundedIndex, len(validators))
	for i, v := range validators {
		indices[i] = common.BoundedIndex{
			Index:      common.ValidatorIndex(i),
			Activation: v.ActivationEpoch,
			Exit:       v.ExitEpoch,
		}
	}
	return indices
}
