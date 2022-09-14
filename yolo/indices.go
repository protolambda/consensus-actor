package yolo

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/protolambda/eth2api"
	"github.com/protolambda/eth2api/client/beaconapi"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"os"
)

func fetchIndicesBounded(ctx context.Context, client eth2api.Client) ([]common.BoundedIndex, error) {
	var resp []eth2api.ValidatorResponse
	exists, err := beaconapi.StateValidators(ctx, client, eth2api.StateHead, nil, nil, &resp)

	if err != nil {
		return nil, fmt.Errorf("failed to fetch validators data: %v", err)
	} else if !exists {
		return nil, fmt.Errorf("validators data not available: %v", err)
	}

	indices := make([]common.BoundedIndex, len(resp))
	for i, v := range resp {
		if v.Index != common.ValidatorIndex(i) {
			return nil, fmt.Errorf("inconsistent returned validators info: %d <> %d", i, v.Index)
		}
		indices[i] = common.BoundedIndex{
			Index:      common.ValidatorIndex(i),
			Activation: v.Validator.ActivationEpoch,
			Exit:       v.Validator.ExitEpoch,
		}
	}
	return indices, nil
}

func loadBoundedIndices(jsonPath string) ([]common.BoundedIndex, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read bounded indices: %w", err)
	}
	var x []common.BoundedIndex
	if err := json.Unmarshal(data, &x); err != nil {
		return nil, fmt.Errorf("failed to decode bounded indices: %w", err)
	}
	return x, nil
}
