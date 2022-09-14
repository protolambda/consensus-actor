package yolo

const (
	maxZoom = 9
	// maxArtificialZoom are zoom levels past maxZoom that just scale contents, rather than providing more detail.
	// diff with maxZoom may not be larger than log2(tileSize)
	// log2(128) = 7
	maxArtificialZoom = 13
	tileSize          = 128
	tileSizeSquared   = tileSize * tileSize

	boundedIndicesFileName = "bounded_indices.json"
	specFileName           = "spec.json"
	dataBlocksDBName       = "blocks"
	dataRandaoDBName       = "randao"
	dataPerfDBName         = "perf"
	dataTilesDBName        = "tiles"
)
