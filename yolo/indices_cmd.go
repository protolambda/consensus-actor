package yolo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/protolambda/eth2api"
	"github.com/urfave/cli"

	"github.com/protolambda/consensus-actor/flags"
)

type BoundedIndicesFetcher struct {
	log log.Logger

	cl *eth2api.Eth2HttpClient

	destPath string
}

func NewBoundedIndicesFetcher(ctx *cli.Context, log log.Logger) (*BoundedIndicesFetcher, error) {
	apiAddr := ctx.String(flags.BeaconAPIAddrFlag.Name)
	if apiAddr == "" {
		return nil, fmt.Errorf("need beacon API address")
	}
	imp := &BoundedIndicesFetcher{
		log: log,
		cl: &eth2api.Eth2HttpClient{
			Addr:  apiAddr,
			Cli:   &http.Client{Timeout: time.Second * 20},
			Codec: eth2api.JSONCodec{},
		},
	}
	baseDir := ctx.String(flags.DataDirFlag.Name)
	if baseDir == "" {
		return nil, fmt.Errorf("need base data dir path")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to prepare base data dir: %v", err)
	}
	imp.destPath = filepath.Join(baseDir, boundedIndicesFileName)
	return imp, nil
}

func (s *BoundedIndicesFetcher) Close() error {
	return nil
}

func (s *BoundedIndicesFetcher) Run(ctx context.Context) error {
	indices, err := fetchIndicesBounded(ctx, s.cl)
	if err != nil {
		return fmt.Errorf("failed to fetch bounded validator indices data: %w", err)
	}
	data, err := json.Marshal(&indices)
	if err != nil {
		return fmt.Errorf("failed to encode indices data: %w", err)
	}
	if err := os.WriteFile(s.destPath, data, 0755); err != nil {
		return fmt.Errorf("failed to write bounded indices json data: %w", err)
	}
	s.log.Info("successfully fetched bounded indices data", "validators", len(indices))
	return nil
}
