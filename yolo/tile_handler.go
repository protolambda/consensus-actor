package yolo

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"strconv"

	"github.com/golang/snappy"
	"github.com/syndtr/goleveldb/leveldb"
)

func (s *Server) handleImgRequest(tileType uint8) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		xStr := q.Get("x")
		x, err := strconv.ParseInt(xStr, 10, 32)
		if err != nil {
			w.WriteHeader(400)
			s.log.Debug("query with bad x value", "err", err)
			_, _ = w.Write([]byte(fmt.Sprintf("bad x value: %v", err)))
			return
		}
		yStr := q.Get("y")
		y, err := strconv.ParseInt(yStr, 10, 32)
		if err != nil {
			w.WriteHeader(400)
			s.log.Debug("query with bad y value", "err", err)
			_, _ = w.Write([]byte(fmt.Sprintf("bad y value: %v", err)))
			return
		}
		zStr := q.Get("z")
		z, err := strconv.ParseInt(zStr, 10, 32)
		if err != nil {
			w.WriteHeader(400)
			s.log.Debug("query with bad z value", "err", err)
			_, _ = w.Write([]byte(fmt.Sprintf("bad z value: %v", err)))
			return
		}
		if x < 0 || y < 0 || z < 0 {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(fmt.Sprintf("negative x %d or y %d or z %d\n", x, y, z)))
			return
		}

		if z > int64(maxZoom) {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(fmt.Sprintf("z too large: %d\n", z)))
			return
		}

		zoom := maxZoom - uint8(z)
		key := tileDbKey(tileType, uint64(x), uint64(y), zoom)
		tilePix, err := s.tiles.Get(key, nil)
		if err == leveldb.ErrNotFound {
			w.WriteHeader(404)
			s.log.Debug(fmt.Sprintf("could not find tile: %d:%d zoom %d (translated zoom: %d)\n", x, y, z, zoom))
			_, _ = w.Write([]byte(fmt.Sprintf("could not find tile: %d:%d:%d", x, y, z)))
			return
		} else if err != nil {
			w.WriteHeader(500)
			s.log.Debug(fmt.Sprintf("server error while getting tile: %d:%d zoom %d (translated zoom: %d)\n", x, y, z, zoom))
			_, _ = w.Write([]byte(fmt.Sprintf("server error while getting tile: %d:%d:%d", x, y, z)))
			return
		}

		tilePix, err = snappy.Decode(nil, tilePix)
		if err != nil {
			s.log.Warn("snappy err", "err", err)
			w.WriteHeader(500)
			return
		}

		// lowest validator index, first epoch, is top left
		img := &image.RGBA{
			Pix: tilePix,
			// from 1 vertical pixel to another means skipping a whole row
			Stride: 4 * int(tileSize),
			Rect:   image.Rectangle{image.Point{0, 0}, image.Point{tileSize, tileSize}},
		}

		var buf bytes.Buffer
		if err = png.Encode(&buf, img); err != nil {
			s.log.Warn("PNG encoding err", "err", err)
			w.WriteHeader(500)
			return
		}
		// TODO: set cache policy based on coordinates

		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(buf.Bytes())
	})
}
