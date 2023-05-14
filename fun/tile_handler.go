package fun

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/log"
	"github.com/golang/snappy"
	"github.com/syndtr/goleveldb/leveldb"
)

// Tile represents an image tile, as stored in the DB.
// R,G,B,A are slices stored next to eachother, since they compress better individually.
//
// Each color slice encodes column by column first, since correlation is larger per-column, for better compression.
// I.e. values 0,1,2,3,4...tilesize-1 all correspond to column 0 (X == 0).
type Tile struct {
	R []byte
	G []byte
	B []byte
	A []byte
	// added to x lookups
	OffsetX int
	// added to y lookups
	OffsetY int
	// Shifts x and y by given amount to create zoomed image effect,
	// while really serving the same tile.
	// This is useful to zoom in more than 1:1 pixel definition,
	// enlarging tiles without image scaling artifacts on client side.
	Scale uint8
}

// ColorModel returns the Image's color model.
func (t *Tile) ColorModel() color.Model {
	return color.RGBAModel
}

// Bounds returns the domain for which At can return non-zero color.
// The bounds do not necessarily contain the point (0, 0).
func (t *Tile) Bounds() image.Rectangle {
	return image.Rectangle{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: tileSize, Y: tileSize}}
}

// At returns the color of the pixel at (x, y).
// At(Bounds().Min.X, Bounds().Min.Y) returns the upper-left pixel of the grid.
// At(Bounds().Max.X-1, Bounds().Max.Y-1) returns the lower-right one.
func (t *Tile) At(x, y int) color.Color {
	return t.RGBAAt(x, y)
}

func (t *Tile) RGBAAt(x, y int) color.RGBA {
	x += t.OffsetX
	y += t.OffsetY
	x >>= t.Scale
	y >>= t.Scale
	if x < 0 || x >= tileSize || y < 0 || y >= tileSize {
		return color.RGBA{}
	}
	pos := x*tileSize + y
	return color.RGBA{
		R: t.R[pos],
		G: t.G[pos],
		B: t.B[pos],
		A: t.A[pos],
	}
}

func (t *Tile) Opaque() bool {
	return false
}

type ImageHandler struct {
	Log     log.Logger
	TilesDB *leveldb.DB
}

func (s *ImageHandler) HandleImgRequest(tileType uint8) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		xStr := q.Get("x")
		x, err := strconv.ParseInt(xStr, 10, 32)
		if err != nil {
			w.WriteHeader(400)
			s.Log.Debug("query with bad x value", "err", err)
			_, _ = w.Write([]byte(fmt.Sprintf("bad x value: %v", err)))
			return
		}
		yStr := q.Get("y")
		y, err := strconv.ParseInt(yStr, 10, 32)
		if err != nil {
			w.WriteHeader(400)
			s.Log.Debug("query with bad y value", "err", err)
			_, _ = w.Write([]byte(fmt.Sprintf("bad y value: %v", err)))
			return
		}
		zStr := q.Get("z")
		z, err := strconv.ParseInt(zStr, 10, 32)
		if err != nil {
			w.WriteHeader(400)
			s.Log.Debug("query with bad z value", "err", err)
			_, _ = w.Write([]byte(fmt.Sprintf("bad z value: %v", err)))
			return
		}
		if x < 0 || y < 0 || z < 0 {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(fmt.Sprintf("negative x %d or y %d or z %d\n", x, y, z)))
			return
		}

		if z > int64(maxArtificialZoom) {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(fmt.Sprintf("z too large: %d\n", z)))
			return
		}
		var zoom, scale uint8
		var tileX, tileY uint64
		var offsetX, offsetY int
		if z > int64(maxZoom) {
			zoom = 0
			scale = uint8(z) - maxZoom
			tileX = uint64(x) >> scale
			tileY = uint64(y) >> scale
			offsetX = int(uint64(x)-(tileX<<scale)) * (tileSize >> scale)
			offsetY = int(uint64(y)-(tileY<<scale)) * (tileSize >> scale)
		} else {
			zoom = maxZoom - uint8(z)
			scale = 0
			offsetX = 0
			offsetY = 0
			tileX = uint64(x)
			tileY = uint64(y)
		}

		key := tileDbKey(tileType, tileX, tileY, zoom)
		tilePix, err := s.TilesDB.Get(key, nil)
		if err == leveldb.ErrNotFound {
			w.WriteHeader(404)
			s.Log.Debug(fmt.Sprintf("could not find tile: %d:%d zoom %d (translated zoom: %d)\n", x, y, z, zoom))
			_, _ = w.Write([]byte(fmt.Sprintf("could not find tile: %d:%d:%d", x, y, z)))
			return
		} else if err != nil {
			w.WriteHeader(500)
			s.Log.Debug(fmt.Sprintf("server error while getting tile: %d:%d zoom %d (translated zoom: %d)\n", x, y, z, zoom))
			_, _ = w.Write([]byte(fmt.Sprintf("server error while getting tile: %d:%d:%d", x, y, z)))
			return
		}

		tilePix, err = snappy.Decode(nil, tilePix)
		if err != nil {
			s.Log.Warn("snappy err", "err", err)
			w.WriteHeader(500)
			return
		}

		// lowest validator index, first epoch, is top left
		img := Tile{
			R:       tilePix[:tileSizeSquared],
			G:       tilePix[tileSizeSquared : tileSizeSquared*2],
			B:       tilePix[tileSizeSquared*2 : tileSizeSquared*3],
			A:       tilePix[tileSizeSquared*3:],
			OffsetX: offsetX,
			OffsetY: offsetY,
			Scale:   scale,
		}

		var buf bytes.Buffer
		if err = png.Encode(&buf, &img); err != nil {
			s.Log.Warn("PNG encoding err", "err", err)
			w.WriteHeader(500)
			return
		}
		// TODO: set cache policy based on coordinates

		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(buf.Bytes())
	})
}
