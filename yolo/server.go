package yolo

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/hashicorp/go-multierror"
	"github.com/protolambda/consensus-actor/flags"
	"github.com/protolambda/eth2api"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/urfave/cli"
	"html/template"
	"net"
	"net/http"
	"os"
	"sync"
)

type Server struct {
	log log.Logger

	listenAddr string

	title          string
	publicEndpoint string
	indexTempl     *template.Template

	syncDisableBlocks bool

	// beacon spec
	spec  *common.Spec
	forks *beacon.ForkDecoder
	// genesis info
	genesis *eth2api.GenesisResponse
	// initial randao value in the beacon state, spec seeds it with first recognized eth1 block hash
	genesisRandao    [32]byte
	genesisBlockRoot common.Root

	// track the activation/exit of each validator, this may be nil
	indicesBounded []common.BoundedIndex
	// moment that indicesBounded were loaded from
	indicesSlot common.Slot

	randao *leveldb.DB // TODO
	blocks *leveldb.DB // blocks importing and blocks transform into
	perf   *leveldb.DB // performance per epoch
	tiles  *leveldb.DB // tile data

	blocksLock sync.RWMutex
	randaoLock sync.RWMutex
	perfLock   sync.RWMutex
	tilesLock  sync.RWMutex

	// beacon api client to load blocks and validator status data from
	beaconCl eth2api.Client

	srv *http.Server

	ctx    context.Context
	cancel context.CancelFunc

	close chan chan error
}

func NewServer(ctx *cli.Context, log log.Logger) (*Server, error) {
	indexTempl, err := template.ParseFS(indexFile, "index.html")
	if err != nil {
		return nil, fmt.Errorf("failed to load index.html template: %v", err)
	}
	//
	//apiAddr := ctx.String(flags.BeaconAPIAddrFlag.Name)
	//if apiAddr == "" {
	//	return nil, fmt.Errorf("need beacon API address")
	//}
	//cl := &eth2api.Eth2HttpClient{
	//	Addr:  apiAddr,
	//	Cli:   &http.Client{Timeout: time.Second * 20},
	//	Codec: eth2api.JSONCodec{},
	//}
	//
	//var spec common.Spec
	//{
	//	log.Info("Loading beacon spec from beacon api endpoint...")
	//	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	//	defer cancel()
	//	if err := configapi.Spec(ctx, cl, &spec); err != nil {
	//		return nil, err
	//	}
	//	log.Info("Loaded beacon spec")
	//}
	//
	//var genesis eth2api.GenesisResponse
	//{
	//	log.Info("Loading genesis info from beacon api endpoint...")
	//	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	//	defer cancel()
	//	if exists, err := beaconapi.Genesis(ctx, cl, &genesis); err != nil {
	//		return nil, err
	//	} else if !exists {
	//		log.Info("genesis is not complete, API says it does not exist")
	//		return nil, fmt.Errorf("no genesis information available")
	//	}
	//	log.Info("Loaded genesis info")
	//}
	//
	//forks := beacon.NewForkDecoder(&spec, genesis.GenesisValidatorsRoot)
	//
	//var genesisRandao [32]byte
	//var genesisBlockRoot [32]byte
	//{
	//	// unfortunately the standard API does not have a method of fetching
	//	// the genesis eth1 block hash or randaovalue, so we just get the full genesis state.
	//	log.Info("Loading genesis beacon state from beacon api endpoint...")
	//	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	//	defer cancel()
	//	var genesisState phase0.BeaconState
	//	if exists, err := debugapi.BeaconState(ctx, cl, eth2api.StateGenesis, &genesisState); err != nil {
	//		return nil, err
	//	} else if !exists {
	//		log.Info("genesis is not complete, API says it does not exist")
	//		return nil, fmt.Errorf("no genesis information available")
	//	}
	//	log.Info("Loaded genesis state")
	//	genesisRandao = genesisState.RandaoMixes[0]
	//	headerCopy := genesisState.LatestBlockHeader
	//	headerCopy.StateRoot = genesisState.HashTreeRoot(&spec, tree.GetHashFn())
	//	genesisBlockRoot = headerCopy.HashTreeRoot(tree.GetHashFn())
	//}

	baseDir := ctx.GlobalString(flags.DataDirFlag.Name)
	if baseDir == "" {
		return nil, fmt.Errorf("need base data dir path")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to prepare base data dir: %v", err)
	}

	listenPort := ctx.Int(flags.HttpPortFlag.Name)
	if listenPort < 0 || listenPort > int(^uint16(0)) {
		return nil, fmt.Errorf("invalid listen port: %d", listenPort)
	}
	listenAddr := net.JoinHostPort(ctx.String(flags.HttpAddrFlag.Name), fmt.Sprintf("%d", listenPort))

	bgCtx, cancel := context.WithCancel(context.Background())
	srv := &Server{
		log: log,

		listenAddr:     listenAddr,
		title:          ctx.String(flags.SiteTitleFlag.Name),
		publicEndpoint: ctx.String(flags.PublicAPIFlag.Name),
		indexTempl:     indexTempl,

		syncDisableBlocks: ctx.Bool(flags.SyncDisableBlocks.Name),
		//
		//spec:             &spec,
		//forks:            forks,
		//genesis:          &genesis,
		//genesisRandao:    genesisRandao,
		//genesisBlockRoot: genesisBlockRoot,
		//
		//beaconCl: cl,

		ctx:    bgCtx,
		cancel: cancel,

		close: make(chan chan error, 1),
	}

	// load dbs, and make sure to close whatever is already open if something fails.
	//if blocks, err := loadBlocksDB(baseDir, false, ctx); err != nil {
	//	_ = srv.closeDBs()
	//	return nil, err
	//} else {
	//	srv.blocks = blocks
	//}
	//if randao, err := loadRandaoDB(baseDir, false, ctx); err != nil {
	//	_ = srv.closeDBs()
	//	return nil, err
	//} else {
	//	srv.randao = randao
	//}
	//if perf, err := loadPerfDB(baseDir, false, ctx); err != nil {
	//	_ = srv.closeDBs()
	//	return nil, err
	//} else {
	//	srv.perf = perf
	//}
	if tiles, err := loadTilesDB(baseDir, false, ctx); err != nil {
		_ = srv.closeDBs()
		return nil, err
	} else {
		srv.tiles = tiles
	}
	return srv, nil
}

func (s *Server) Close() error {
	s.log.Info("closing server...")
	s.cancel()
	closing := make(chan error)
	select {
	case s.close <- closing:
		return <-closing
	default:
		// already requested close
		return fmt.Errorf("already closing")
	}
}

func (s *Server) Sync() {
	s.startHttpServer()

	//syncReqs := make(chan struct{}, 1)
	//
	//reqSync := func() {
	//	select {
	//	case syncReqs <- struct{}{}:
	//	default:
	//		// already requested sync
	//	}
	//}
	//
	//// Every slot, aligned with genesis, with an offset of 2 seconds, we fire the ticker to check for new blocks.
	//slotDuration := time.Duration(s.spec.SECONDS_PER_SLOT) * time.Second
	//startTime := time.Unix(int64(s.genesis.GenesisTime)+2, 0)
	//tillPoll := slotDuration - (time.Since(startTime) % slotDuration)
	//slotTicker := time.NewTicker(tillPoll)
	//defer slotTicker.Stop()
	//
	//reqSync()

	for {
		select {
		//case <-slotTicker.C:
		//	slotTicker.Reset(slotDuration - (time.Since(startTime) % slotDuration)) // correct ticker period
		//	reqSync()
		//case <-syncReqs:
		//	if err := s.syncStep(); errors.Is(err, io.EOF) {
		//		s.log.Info("done syncing")
		//		break
		//	} else if err != nil {
		//		s.log.Error("failed sync step", "err", err)
		//	} else {
		//		reqSync()
		//	}
		case closer := <-s.close:
			var result error
			if err := s.srv.Close(); err != nil {
				result = multierror.Append(result, err)
			}
			if err := s.closeDBs(); err != nil {
				result = multierror.Append(result, err)
			}
			closer <- result
			return
		}
	}
}
