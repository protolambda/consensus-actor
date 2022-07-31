package yolo

import (
	"context"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/hashicorp/go-multierror"
	"github.com/protolambda/consensus-actor/flags"
	"github.com/protolambda/eth2api"
	"github.com/protolambda/eth2api/client/beaconapi"
	"github.com/protolambda/eth2api/client/configapi"
	"github.com/protolambda/eth2api/client/debugapi"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/beacon/phase0"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/urfave/cli"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

type Server struct {
	log log.Logger

	listenAddr string

	publicEndpoint string
	indexTempl     *template.Template

	// beacon spec
	spec *common.Spec
	// genesis info
	genesis *eth2api.GenesisResponse
	// initial randao value in the beacon state, spec seeds it with first recognized eth1 block hash
	genesisRandao [32]byte

	// track the last header we fully processed the chain up to, may be nil
	lastHeader   *common.BeaconBlockHeader
	lastProgress Progress

	// track the activation/exit of each validator, this may be nil
	indicesBounded []common.BoundedIndex
	// moment that indicesBounded were loaded from
	indicesProgress Progress

	remappings []ValidatorRemapping

	blocks *leveldb.DB // blocks importing and blocks transform into
	perf   *leveldb.DB // performance per epoch
	tiles  *leveldb.DB // tile data

	blocksLock sync.RWMutex
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

	apiAddr := ctx.GlobalString(flags.BeaconAPIAddrFlag.Name)
	if apiAddr == "" {
		return nil, fmt.Errorf("need beacon API address")
	}
	cl := &eth2api.Eth2HttpClient{
		Addr:  apiAddr,
		Cli:   &http.Client{Timeout: time.Second * 20},
		Codec: eth2api.JSONCodec{},
	}

	var spec common.Spec
	{
		log.Info("Loading beacon spec from beacon api endpoint...")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		if err := configapi.Spec(ctx, cl, &spec); err != nil {
			return nil, err
		}
		log.Info("Loaded beacon spec")
	}

	var genesis eth2api.GenesisResponse
	{
		log.Info("Loading genesis info from beacon api endpoint...")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		if exists, err := beaconapi.Genesis(ctx, cl, &genesis); err != nil {
			return nil, err
		} else if !exists {
			log.Info("genesis is not complete, API says it does not exist")
			return nil, fmt.Errorf("no genesis information available")
		}
		log.Info("Loaded genesis info")
	}

	var genesisRandao [32]byte
	{
		// unfortunately the standard API does not have a method of fetching
		// the genesis eth1 block hash or randaovalue, so we just get the full genesis state.
		log.Info("Loading beacon state from beacon api endpoint...")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		var genesisState phase0.BeaconState
		if exists, err := debugapi.BeaconState(ctx, cl, eth2api.StateGenesis, &genesisState); err != nil {
			return nil, err
		} else if !exists {
			log.Info("genesis is not complete, API says it does not exist")
			return nil, fmt.Errorf("no genesis information available")
		}
		log.Info("Loaded genesis state")
		genesisRandao = genesisState.RandaoMixes[0]
	}

	baseDir := ctx.GlobalString(flags.DataDirFlag.Name)
	if baseDir == "" {
		return nil, fmt.Errorf("need base data dir path")
	}
	if err := os.MkdirAll(baseDir, 644); err != nil {
		return nil, fmt.Errorf("failed to prepare base data dir: %v", err)
	}

	listenPort := ctx.GlobalInt(flags.HttpPortFlag.Name)
	if listenPort < 0 || listenPort > int(^uint16(0)) {
		return nil, fmt.Errorf("invalid listen port: %d", listenPort)
	}
	listenAddr := net.JoinHostPort(ctx.GlobalString(flags.HttpAddrFlag.Name), fmt.Sprintf("%d", listenPort))

	bgCtx, cancel := context.WithCancel(context.Background())
	srv := &Server{
		log: log,

		listenAddr:     listenAddr,
		publicEndpoint: ctx.GlobalString(flags.PublicAPIFlag.Name),
		indexTempl:     indexTempl,

		spec:          &spec,
		genesis:       &genesis,
		genesisRandao: genesisRandao,

		beaconCl: cl,

		ctx:    bgCtx,
		cancel: cancel,

		close: make(chan chan error),
	}

	// load dbs, and make sure to close whatever is already open if something fails.
	if err := srv.loadBlocksDB(baseDir, ctx); err != nil {
		_ = srv.closeDBs()
		return nil, err
	}
	if err := srv.loadPerfDB(baseDir, ctx); err != nil {
		_ = srv.closeDBs()
		return nil, err
	}
	if err := srv.loadTilesDB(baseDir, ctx); err != nil {
		_ = srv.closeDBs()
		return nil, err
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

func (s *Server) Run() {
	s.startHttpServer()

	syncReqs := make(chan struct{})

	reqSync := func() {
		select {
		case syncReqs <- struct{}{}:
		default:
			// already requested sync
		}
	}

	// TODO: align with genesis time
	slotTicker := time.NewTicker(time.Duration(s.spec.SECONDS_PER_SLOT) * time.Second)
	defer slotTicker.Stop()

	for {
		select {
		case <-slotTicker.C:
			reqSync()
		case <-syncReqs:
			if err := s.syncStep(); errors.Is(err, io.EOF) {
				// done syncing
				break
			} else if err != nil {
				// log
			} else {
				reqSync()
			}
		case closer := <-s.close:
			var result *multierror.Error
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
