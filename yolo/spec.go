package yolo

import (
	"encoding/json"
	"fmt"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"os"
)

func loadSpec(specFilePath string) (*common.Spec, error) {
	data, err := os.ReadFile(specFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec file: %w", err)
	}
	var x common.Spec
	if err := json.Unmarshal(data, &x); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json spec: %w", err)
	}
	return &x, nil
}
