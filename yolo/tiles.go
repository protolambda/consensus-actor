package yolo

import (
	"encoding/binary"
	"fmt"
	"github.com/golang/snappy"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"io"
)

const (
	// KeyTile is a:
	// 3 byte prefix for tile keying, followed by:
	// 1 byte tile type
	// 1 byte zoom.
	// 4 byte big endian X
	// 4 byte big endian Y
	//
	// Note: the X is first, so the DB range iterate can range over epochs at zoom 0.
	//
	// Values under this key are snappy block-compressed.
	//
	// The uncompressed value is a square of tileSize x tileSize, with 4 bytes per pixel.
	// Tiles are encoded as RGBA bitmap: 4 byte pixel values, one row at a time, from image top to bottom.
	//
	// TODO: we can encode tiles in with R, G, B grouped together separately. Omit A.
	// And implement the image.Image interface to map back to an image.
	// This way we compress better, and don't store as much alpha-channel data.
	KeyTile string = "til"
)

func tileDbKey(tileType uint8, tX uint64, tY uint64, zoom uint8) []byte {
	var key [3 + 1 + 4 + 4 + 1]byte
	copy(key[:3], KeyTile)
	key[3] = tileType
	key[4] = zoom
	binary.BigEndian.PutUint32(key[3+1+1:3+1+1+4], uint32(tX))
	binary.BigEndian.PutUint32(key[3+1+1+4:3+1+1+4+4], uint32(tY))
	return key[:]
}

func (s *Server) performanceToTiles(tileType uint8, tX uint64) error {
	maxValidators := uint64(len(s.indicesBounded))

	tilesY := (maxValidators + tileSize - 1) / tileSize
	// each tile is an array of 4 byte items. tileSize consecutive of those form a row, and then tileSize rows.
	// RGBA
	tiles := make([][]byte, tilesY, tilesY)
	tileBytes := 4 * tileSize * tileSize
	for tY := uint64(0); tY < tilesY; tY++ {
		tiles[tY] = make([]byte, tileBytes, tileBytes)
	}
	for x := uint64(0); x < tileSize; x++ {
		epoch := common.Epoch(tX*tileSize + x)
		//fmt.Printf("processing epoch %d\n", epoch)
		perf, err := s.getPerf(epoch)
		if err != nil {
			fmt.Printf("no data for epoch %d\n", epoch)
			continue
			//return fmt.Errorf("failed to get epoch data %d: %v", epoch, err)
		}

		// TODO: based on tileType apply a y-axis remapping on the performance

		for vi, vPerf := range perf {
			tY := uint64(vi) / tileSize
			tile := tiles[tY]
			y := uint64(vi) % tileSize
			pos := 4 * (y*tileSize + x)
			// max alpha
			tile[pos+3] = 0xff

			if vPerf&ValidatorExists == 0 {
				// if not existing, then black pixel
				tile[pos] = 0
				tile[pos+1] = 0
				tile[pos+2] = 0
			} else {
				// if existent, but not participating, then color it a special gray
				if vPerf == ValidatorExists {
					tile[pos] = 0x20
					tile[pos+1] = 0x20
					tile[pos+2] = 0x20
				} else {
					// higher head distance becomes darker (unknown is 0xff)
					headDist := uint32((vPerf >> 24) & 0xff)
					if headDist == 0xff {
						tile[pos] = 0x30
					} else {
						q := 64 - headDist
						q = (q * q * q * q * q) >> 22
						tile[pos] = uint8(q)
					}
					// correct target is 0xff, incorrect is 0
					tile[pos+1] = byte(vPerf >> 16)
					// higher inclusion distance becomes darker
					inclDist := uint32((vPerf >> 8) & 0xff)
					if inclDist == 0xff {
						tile[pos+2] = 0x30
					} else {
						q := 64 - inclDist
						q = (q * q * q * q * q) >> 22
						tile[pos+2] = uint8(q)
					}
				}
			}
		}
		for vi := uint64(len(perf)); vi < maxValidators; vi++ {
			tY := vi / tileSize
			tile := tiles[tY]
			y := vi % tileSize
			pos := 4 * (y*tileSize + x)
			// transparent pixel
			tile[pos] = 0
			tile[pos+1] = 0
			tile[pos+2] = 0
			tile[pos+3] = 0
		}
	}
	for tY, tile := range tiles {
		// TODO more types
		key := tileDbKey(tileType, tX, uint64(tY), 0)
		// compress the tile image
		tile = snappy.Encode(nil, tile)
		if err := s.tiles.Put(key, tile, nil); err != nil {
			return fmt.Errorf("failed to write tile %d:%d (zoom 0): %v", tX, tY, err)
		}
	}
	return nil
}

func (s *Server) convTiles(tileType uint8, tX uint64, zoom uint8) error {
	maxValidators := uint64(len(s.indicesBounded))

	tileSizeAbs := uint64(tileSize) << zoom
	tilesY := (maxValidators + tileSizeAbs - 1) / tileSizeAbs
	for tY := uint64(0); tY < tilesY; tY += 1 {
		topLeft := tileDbKey(tileType, tX*2, tY*2, zoom-1)
		topRight := tileDbKey(tileType, tX*2+1, tY*2, zoom-1)
		// remember, y is downwards
		bottomLeft := tileDbKey(tileType, tX*2, tY*2+1, zoom-1)
		bottomRight := tileDbKey(tileType, tX*2+1, tY*2+1, zoom-1)

		getTile := func(key []byte) ([]byte, error) {
			tile, err := s.tiles.Get(key, nil)
			if err == leveldb.ErrNotFound {
				// use empty tile instead
				tile = make([]byte, 4*tileSize*tileSize, 4*tileSize*tileSize)
				return tile, nil
			} else if err != nil {
				return nil, fmt.Errorf("failed to get top left of (%d; %d): %v", tX, tY, err)
			} else {
				return snappy.Decode(nil, tile)
			}
		}
		topLeftTile, err := getTile(topLeft)
		if err != nil {
			return err
		}
		topRightTile, err := getTile(topRight)
		if err != nil {
			return err
		}
		bottomLeftTile, err := getTile(bottomLeft)
		if err != nil {
			return err
		}
		bottomRightTile, err := getTile(bottomRight)
		if err != nil {
			return err
		}

		outTile := make([]byte, 4*tileSize*tileSize, 4*tileSize*tileSize)
		mix := func(a, b, c, d byte) byte {
			return uint8((uint16(a) + uint16(b) + uint16(c) + uint16(d)) / 4)
		}
		// compress a N x N tile into a N/2 x N/2 tile, writing it to a tile at the given offset.
		compressedFn := func(offX uint64, offY uint64, inTile []byte) {
			for x := uint64(0); x < tileSize/2; x++ {
				for y := uint64(0); y < tileSize/2; y++ {
					// top left, top right, bottom left, bottom right
					p0, p1, p2, p3 := (y*2*tileSize+x*2)*4, (y*2*tileSize+x*2+1)*4,
						((y*2+1)*tileSize+x*2)*4, ((y*2+1)*tileSize+x*2+1)*4
					r0, g0, b0, a0 := inTile[p0], inTile[p0+1], inTile[p0+2], inTile[p0+3]
					r1, g1, b1, a1 := inTile[p1], inTile[p1+1], inTile[p1+2], inTile[p1+3]
					r2, g2, b2, a2 := inTile[p2], inTile[p2+1], inTile[p2+2], inTile[p2+3]
					r3, g3, b3, a3 := inTile[p3], inTile[p3+1], inTile[p3+2], inTile[p3+3]

					r := mix(r0, r1, r2, r3)
					g := mix(g0, g1, g2, g3)
					b := mix(b0, b1, b2, b3)
					a := mix(a0, a1, a2, a3)
					outTile[((offY+y)*tileSize+offX+x)*4] = r
					outTile[((offY+y)*tileSize+offX+x)*4+1] = g
					outTile[((offY+y)*tileSize+offX+x)*4+2] = b
					outTile[((offY+y)*tileSize+offX+x)*4+3] = a
				}
			}
		}
		compressedFn(0, 0, topLeftTile)
		compressedFn(tileSize/2, 0, topRightTile)
		compressedFn(0, tileSize/2, bottomLeftTile)
		compressedFn(tileSize/2, tileSize/2, bottomRightTile)

		key := tileDbKey(tileType, tX, tY, zoom)
		// compress the tile image
		outTile = snappy.Encode(nil, outTile)
		if err := s.tiles.Put(key, outTile, nil); err != nil {
			return fmt.Errorf("failed to write tile %d:%d (zoom %d): %v", tX, tY, zoom, err)
		}
	}
	return nil
}

func (s *Server) lastTileEpoch(tileType uint8) (common.Epoch, error) {
	iter := s.blocks.NewIterator(util.BytesPrefix(append([]byte(KeyPerf), tileType, 0)), nil)
	defer iter.Release()
	if iter.Last() {
		epoch := common.Epoch(binary.BigEndian.Uint32(iter.Key()[3+1+1 : 3+1+1+4]))
		return epoch, nil
	} else {
		return 0, iter.Error()
	}
}

func (s *Server) updateTilesMaybe() error {
	lastTileEpoch, err := s.lastTileEpoch(0)
	if err != nil {
		return fmt.Errorf("failed to retrieve last tile epoch: %v", err)
	}
	lastSlot, _, err := s.lastSlot()
	if err != nil {
		return fmt.Errorf("failed to retrieve last block slot during tile update: %v", err)
	}
	lastBlockEpoch := s.spec.SlotToEpoch(lastSlot)
	if lastTileEpoch == lastBlockEpoch {
		return io.EOF
	}

	for tX := uint64(lastTileEpoch) / tileSize; tX <= uint64(lastBlockEpoch)/tileSize; tX++ {
		if err := s.performanceToTiles(0, tX); err != nil {
			return fmt.Errorf("failed to update zoom 0 tiles at tX %d: %v", tX, err)
		}
	}

	for z := uint8(1); z <= maxZoom; z++ {
		tileSizeAbs := uint64(tileSize) << z
		tilesXStart := uint64(lastTileEpoch) / tileSizeAbs
		tilesXEnd := (uint64(lastBlockEpoch) + tileSizeAbs - 1) / tileSizeAbs
		for i := tilesXStart; i < tilesXEnd; i++ {
			if err := s.convTiles(0, i, z); err != nil {
				return fmt.Errorf("failed tile convolution layer at zoom %d tX %d: %v", z, i, err)
			}
		}
	}
	return nil
}

func (s *Server) resetTilesTyped(tileType uint8, resetSlot common.Slot) error {
	resetEpoch := s.spec.SlotToEpoch(resetSlot)

	lastEpoch, err := s.lastTileEpoch(tileType)
	if err != nil {
		return err
	}

	if resetEpoch > lastEpoch { // check if there's anything to reset
		return nil
	}

	var batch leveldb.Batch
	for z := uint8(0); z < maxZoom; z++ {
		start := uint32(lastEpoch >> z)
		end := uint32(resetEpoch >> z)
		r := &util.Range{
			Start: make([]byte, 3+1+1+4),
			Limit: make([]byte, 3+1+1+4),
		}
		copy(r.Start[:3], KeyTile)
		r.Start[3] = tileType
		r.Start[3+1] = z
		binary.BigEndian.PutUint32(r.Start[3+1+1:], start)

		copy(r.Limit[:3], KeyTile)
		r.Limit[3] = tileType
		r.Limit[3+1] = z
		binary.BigEndian.PutUint32(r.Limit[3+1+1:], end+1)

		iter := s.blocks.NewIterator(r, nil)
		for iter.Next() {
			batch.Delete(iter.Key())
		}
		iter.Release()
	}
	if err := s.tiles.Write(&batch, nil); err != nil {
		return fmt.Errorf("failed to remove tile data of type %d, resetting to slot %d: %v", tileType, resetSlot, err)
	}
	return nil
}

func (s *Server) resetTiles(resetSlot common.Slot) error {
	// TODO more types
	return s.resetTilesTyped(0, resetSlot)
}
