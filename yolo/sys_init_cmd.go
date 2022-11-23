package yolo

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/hashicorp/go-multierror"
	"github.com/protolambda/eth2api"
	"github.com/protolambda/eth2api/client/configapi"
	"github.com/protolambda/eth2api/client/debugapi"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/beacon/phase0"
	"github.com/protolambda/ztyp/tree"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/urfave/cli"

	"github.com/protolambda/consensus-actor/flags"
)

type SystemInitializer struct {
	log log.Logger

	cl *eth2api.Eth2HttpClient

	specPath string

	blocks *leveldb.DB
	randao *leveldb.DB
}

func NewSystemInitializer(ctx *cli.Context, log log.Logger) (*SystemInitializer, error) {
	apiAddr := ctx.String(flags.BeaconAPIAddrFlag.Name)
	if apiAddr == "" {
		return nil, fmt.Errorf("need beacon API address")
	}
	imp := &SystemInitializer{
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
	imp.specPath = filepath.Join(baseDir, specFileName)
	if blocks, err := loadBlocksDB(baseDir, false, ctx); err != nil {
		_ = imp.Close()
		return nil, err
	} else {
		imp.blocks = blocks
	}
	if randao, err := loadRandaoDB(baseDir, false, ctx); err != nil {
		_ = imp.Close()
		return nil, err
	} else {
		imp.randao = randao
	}
	return imp, nil
}

func (s *SystemInitializer) Close() error {
	var result error
	if s.blocks != nil {
		if err := s.blocks.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close blocks db: %w", err))
		}
	}
	if s.randao != nil {
		if err := s.randao.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close randao db: %w", err))
		}
	}
	return result
}

func (s *SystemInitializer) Run(ctx context.Context) error {
	var spec common.Spec
	{
		s.log.Info("Loading beacon spec from beacon api endpoint...")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		if err := configapi.Spec(ctx, s.cl, &spec); err != nil {
			return err
		}
		s.log.Info("Loaded beacon spec")
	}

	var genesisRandao [32]byte
	var genesisBlockRoot [32]byte
	{
		// unfortunately the standard API does not have a method of fetching
		// the genesis eth1 block hash or randaovalue, so we just get the full genesis state.
		s.log.Info("Loading genesis beacon state from beacon api endpoint...")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		var genesisState phase0.BeaconState
		if exists, err := debugapi.BeaconState(ctx, s.cl, eth2api.StateGenesis, &genesisState); err != nil {
			return err
		} else if !exists {
			s.log.Info("genesis is not complete, API says it does not exist")
			return fmt.Errorf("no genesis information available")
		}
		s.log.Info("Loaded genesis state")
		genesisRandao = genesisState.RandaoMixes[0]
		headerCopy := genesisState.LatestBlockHeader
		headerCopy.StateRoot = genesisState.HashTreeRoot(&spec, tree.GetHashFn())
		genesisBlockRoot = headerCopy.HashTreeRoot(tree.GetHashFn())
	}

	// store initial randao value
	{
		var key [3 + 8]byte
		copy(key[:3], KeyRandaoMix)
		binary.BigEndian.PutUint64(key[3:], 0)
		if err := s.randao.Put(key[:], genesisRandao[:], nil); err != nil {
			return fmt.Errorf("failed to write genesis randao value")
		}
	}

	// store initial genesis block root value
	{
		var key [3 + 8]byte
		copy(key[:3], KeyBlockRoot)
		binary.BigEndian.PutUint64(key[3:], 0)
		if err := s.blocks.Put(key[:], genesisBlockRoot[:], nil); err != nil {
			return fmt.Errorf("failed to write genesis block root value")
		}
	}

	data, err := json.MarshalIndent(&spec, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode spec data: %w", err)
	}
	if err := os.WriteFile(s.specPath, data, 0755); err != nil {
		return fmt.Errorf("failed to write spec json data: %w", err)
	}
	s.log.Info("successfully initialized system")
	return nil
}
