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
	DataBlocksDBFlag = cli.StringFlag{
		Name:   "data.blocks",
		Usage:  "Blocks level db path, relative to data.dir",
		EnvVar: prefixEnvVar("DATA_BLOCKS"),
		Value:  "blocks",
	}
	DataPerfDBFlag = cli.StringFlag{
		Name:   "data.perf",
		Usage:  "Performance level db path, relative to data.dir",
		EnvVar: prefixEnvVar("DATA_PERF"),
		Value:  "perf",
	}
	DataTilesDBFlag = cli.StringFlag{
		Name:   "data.tiles",
		Usage:  "Tiles level db path, relative to data.dir",
		EnvVar: prefixEnvVar("DATA_TILES"),
		Value:  "tiles",
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
	DataBlocksDBFlag,
	DataPerfDBFlag,
	DataTilesDBFlag,
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
	BeaconAPIAddrFlag = cli.StringFlag{
		Name:   "beacon.api",
		Usage:  "Beacon HTTP API endpoint",
		EnvVar: prefixEnvVar("BEACON_API"),
		Value:  "http://localhost:5052",
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
		Name:     "lighthouse.chain",
		Usage:    "Path to lighthouse chain leveldb dir.",
		EnvVar:   prefixEnvVar("IMPORT_LIGHTHOUSE_CHAIN"),
		Required: true,
	}
	ImportLighthouseFreezerFlag = cli.StringFlag{
		Name:     "lighthouse.freezer",
		Usage:    "Path to lighthouse freezer leveldb dir.",
		EnvVar:   prefixEnvVar("IMPORT_LIGHTHOUSE_FREEZER"),
		Required: true,
	}
	ImportStartSlotFlag = cli.Uint64Flag{
		Name:   "startslot",
		Usage:  "Start slot (inclusive) of block import",
		EnvVar: prefixEnvVar("IMPORT_START_SLOT"),
		Value:  uint64(0),
	}
	ImportEndSlotFlag = cli.Uint64Flag{
		Name:   "endslot",
		Usage:  "End slot (exclusive) of block import",
		EnvVar: prefixEnvVar("IMPORT_END_SLOT"),
		Value:  ^uint64(0),
	}
)

var ImportFlags = []cli.Flag{
	ImportLighthouseChainFlag,
	ImportLighthouseFreezerFlag,
	ImportStartSlotFlag,
	ImportEndSlotFlag,
}
