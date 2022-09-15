package flags

import "github.com/urfave/cli"

const envVarPrefix = "CONSENSUS_ACTOR_"

func prefixEnvVar(name string) string {
	return envVarPrefix + name
}

// global flags
var (
	DataDirFlag = cli.StringFlag{
		Name:   "data.dir",
		Usage:  "Base data dir to locate application databases in",
		EnvVar: prefixEnvVar("DATA_DIR"),
		Value:  "consensus_actor_data",
	}
	DataBlocksCacheSizeFlag = cli.IntFlag{
		Name:   "data.blocks.cachesize",
		Usage:  "Leveldb memory cache size in megabytes for data blocks cache",
		EnvVar: prefixEnvVar("DATA_BLOCKS_CACHE_SIZE"),
		Value:  4000,
	}
	DataRandaoCacheSizeFlag = cli.IntFlag{
		Name:   "data.randao.cachesize",
		Usage:  "Leveldb memory cache size in megabytes for data randao cache",
		EnvVar: prefixEnvVar("DATA_RANDAO_CACHE_SIZE"),
		Value:  500,
	}
	DataPerfCacheSizeFlag = cli.IntFlag{
		Name:   "data.perf.cachesize",
		Usage:  "Leveldb memory cache size in megabytes for perf cache",
		EnvVar: prefixEnvVar("DATA_PERF_CACHE_SIZE"),
		Value:  4000,
	}
	DataTilesCacheSizeFlag = cli.IntFlag{
		Name:   "data.tiles.cachesize",
		Usage:  "Leveldb memory cache size in megabytes for tiles cache",
		EnvVar: prefixEnvVar("DATA_TILES_CACHE_SIZE"),
		Value:  4000,
	}
	BeaconAPIAddrFlag = cli.StringFlag{
		Name:   "beacon.api",
		Usage:  "Beacon HTTP API endpoint",
		EnvVar: prefixEnvVar("BEACON_API"),
		Value:  "http://localhost:5052",
	}
	LogLevelFlag = cli.StringFlag{
		Name:   "log.level",
		Usage:  "The lowest log level that will be output",
		Value:  "info",
		EnvVar: prefixEnvVar("LOG_LEVEL"),
	}
	LogFormatFlag = cli.StringFlag{
		Name:   "log.format",
		Usage:  "Format the log output. Supported formats: 'text', 'json'",
		Value:  "text",
		EnvVar: prefixEnvVar("LOG_FORMAT"),
	}
	LogColorFlag = cli.BoolFlag{
		Name:   "log.color",
		Usage:  "Color the log output",
		EnvVar: prefixEnvVar("LOG_COLOR"),
	}
)

var GlobalFlags = []cli.Flag{
	DataDirFlag,
	DataBlocksCacheSizeFlag,
	DataRandaoCacheSizeFlag,
	DataPerfCacheSizeFlag,
	DataTilesCacheSizeFlag,
	LogLevelFlag,
	LogFormatFlag,
	LogColorFlag,
}

// server flags
var (
	HttpAddrFlag = cli.StringFlag{
		Name:   "http.addr",
		Usage:  "Address to bind http server to",
		EnvVar: prefixEnvVar("HTTP_ADDR"),
		Value:  "0.0.0.0",
	}
	HttpPortFlag = cli.IntFlag{
		Name:   "http.port",
		Usage:  "Port to bind http server to",
		EnvVar: prefixEnvVar("HTTP_PORT"),
		Value:  8080,
	}
	SiteTitleFlag = cli.StringFlag{
		Name:   "site.title",
		Usage:  "Title to put in webpage.",
		EnvVar: prefixEnvVar("SITE_TITLE"),
		Value:  "Mainnet | consensus.actor",
	}
	PublicAPIFlag = cli.StringFlag{
		Name:   "public.api",
		Usage:  "Http endpoint for frontend to talk to server with. Empty if the same server.",
		EnvVar: prefixEnvVar("PUBLIC_API"),
		Value:  "",
	}
	SyncDisableBlocks = cli.BoolFlag{
		Name:   "sync.disable.blocks",
		Usage:  "Disables syncing of new blocks, to keep server content the same",
		EnvVar: prefixEnvVar("SYNC_DISABLE_BLOCKS"),
	}
)

var ServerFlags = []cli.Flag{
	HttpAddrFlag,
	HttpPortFlag,
	SiteTitleFlag,
	PublicAPIFlag,
	BeaconAPIAddrFlag,
	SyncDisableBlocks,
	LogLevelFlag,
	LogFormatFlag,
	LogColorFlag,
}

// import flags
var (
	ImportLighthouseChainFlag = cli.StringFlag{
		Name:      "lighthouse.chain",
		Usage:     "Path to lighthouse chain leveldb dir.",
		EnvVar:    prefixEnvVar("IMPORT_LIGHTHOUSE_CHAIN"),
		Required:  true,
		TakesFile: true,
	}
	ImportLighthouseFreezerFlag = cli.StringFlag{
		Name:      "lighthouse.freezer",
		Usage:     "Path to lighthouse freezer leveldb dir.",
		EnvVar:    prefixEnvVar("IMPORT_LIGHTHOUSE_FREEZER"),
		Required:  true,
		TakesFile: true,
	}
	ImportLighthouseChainCacheSizeFlag = cli.IntFlag{
		Name:   "lighthouse.chain.cachesize",
		Usage:  "Leveldb memory cache size in megabytes for chain db cache",
		EnvVar: prefixEnvVar("IMPORT_LIGHTHOUSE_CHAIN_CACHE_SIZE"),
		Value:  2000,
	}
	ImportLighthouseFreezerCacheSizeFlag = cli.IntFlag{
		Name:   "lighthouse.freezer.cachesize",
		Usage:  "Leveldb memory cache size in megabytes for freezer db cache",
		EnvVar: prefixEnvVar("IMPORT_LIGHTHOUSE_FREEZER_CACHE_SIZE"),
		Value:  2000,
	}
	ImportStartSlotFlag = cli.Uint64Flag{
		Name:   "import.startslot",
		Usage:  "Start slot (inclusive) of block import",
		EnvVar: prefixEnvVar("IMPORT_START_SLOT"),
		Value:  uint64(0),
	}
	ImportEndSlotFlag = cli.Uint64Flag{
		Name:   "import.endslot",
		Usage:  "End slot (exclusive) of block import",
		EnvVar: prefixEnvVar("IMPORT_END_SLOT"),
		Value:  ^uint64(0),
	}
)

var ImportFlags = []cli.Flag{
	ImportLighthouseChainFlag,
	ImportLighthouseFreezerFlag,
	ImportLighthouseChainCacheSizeFlag,
	ImportLighthouseFreezerCacheSizeFlag,
	ImportStartSlotFlag,
	ImportEndSlotFlag,
}

var (
	RandaoStartEpochFlag = cli.Uint64Flag{
		Name:   "randao.startepoch",
		Usage:  "Start epoch (inclusive) of randao import",
		EnvVar: prefixEnvVar("RANDAO_START_EPOCH"),
		Value:  uint64(0),
	}
	RandaoEndEpochFlag = cli.Uint64Flag{
		Name:   "randao.endepoch",
		Usage:  "End epoch (exclusive) of randao import",
		EnvVar: prefixEnvVar("RANDAO_END_EPOCH"),
		Value:  ^uint64(0),
	}
)

var SysInitFlags = []cli.Flag{
	BeaconAPIAddrFlag,
}

var RandaoFlags = []cli.Flag{
	RandaoStartEpochFlag,
	RandaoEndEpochFlag,
}

var BoundedIndicesFlags = []cli.Flag{
	BeaconAPIAddrFlag,
}

var (
	PerfStartEpochFlag = cli.Uint64Flag{
		Name:   "perf.startepoch",
		Usage:  "Start epoch (inclusive) of perf import",
		EnvVar: prefixEnvVar("PERF_START_EPOCH"),
		Value:  uint64(0),
	}
	PerfEndEpochFlag = cli.Uint64Flag{
		Name:   "perf.endepoch",
		Usage:  "End epoch (exclusive) of perf import",
		EnvVar: prefixEnvVar("PERF_END_EPOCH"),
		Value:  ^uint64(0),
	}
)

var PerfFlags = []cli.Flag{
	PerfStartEpochFlag,
	PerfEndEpochFlag,
}

var (
	TilesStartEpochFlag = cli.Uint64Flag{
		Name:   "tiles.startepoch",
		Usage:  "Start epoch (inclusive) of tiles import",
		EnvVar: prefixEnvVar("TILES_START_EPOCH"),
		Value:  uint64(0),
	}
	TilesEndEpochFlag = cli.Uint64Flag{
		Name:   "tiles.endepoch",
		Usage:  "End epoch (exclusive) of tiles import",
		EnvVar: prefixEnvVar("TILES_END_EPOCH"),
		Value:  ^uint64(0),
	}
)

var TilesFlags = []cli.Flag{
	TilesStartEpochFlag,
	TilesEndEpochFlag,
}

// TODO refactor server

// TODO reset command
