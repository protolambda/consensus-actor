package fun

import (
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/log"
	"github.com/golang/snappy"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
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
	// The uncompressed value is 4 squares of tileSize x tileSize, one for R, one for G, B, and A
	// The squares encode column by column.
	//
	// We can encode tiles in with R, G, B, A grouped together separately.
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

func performanceToTiles(log log.Logger, tilesDB *leveldb.DB, perfDB *leveldb.DB, tileType uint8, tX uint64) error {
	maxValidators := uint64(0)
	for x := uint64(0); x < tileSize; x++ {
		epoch := common.Epoch(tX*tileSize + x)
		perf, err := getPerf(perfDB, epoch)
		if err != nil {
			// no data for this epoch
			continue
		}
		if uint64(len(perf)) > maxValidators {
			maxValidators = uint64(len(perf))
		}
	}

	tilesY := (maxValidators + tileSize - 1) / tileSize
	// each tile is an array of 4 byte items. tileSize consecutive of those form a row, and then tileSize rows.
	// RGBA
	tiles := make([][]byte, tilesY)
	tileBytes := 4 * tileSize * tileSize
	for tY := uint64(0); tY < tilesY; tY++ {
		tiles[tY] = make([]byte, tileBytes)
	}
	for x := uint64(0); x < tileSize; x++ {
		epoch := common.Epoch(tX*tileSize + x)
		//fmt.Printf("processing epoch %d\n", epoch)
		perf, err := getPerf(perfDB, epoch)
		if err != nil {
			log.Info("no performance data for epoch", "epoch", epoch)
			continue
			//return fmt.Errorf("failed to get epoch data %d: %v", epoch, err)
		}

		// TODO: based on tileType apply a y-axis remapping on the performance

		for vi, vPerf := range perf {
			tY := uint64(vi) / tileSize
			tile := tiles[tY]
			tileR := tile[:tileSizeSquared]
			tileG := tile[tileSizeSquared : tileSizeSquared*2]
			tileB := tile[tileSizeSquared*2 : tileSizeSquared*3]
			tileA := tile[tileSizeSquared*3:]

			y := uint64(vi) % tileSize
			pos := x*tileSize + y
			// max alpha
			tileA[pos] = 0xff

			if vPerf&ValidatorExists == 0 {
				// if not existing, then black pixel
				tileR[pos] = 0
				tileG[pos] = 0
				tileB[pos] = 0
			} else {
				// if existent, but not participating, then color it a special gray
				if vPerf == ValidatorExists {
					tileR[pos] = 0x20
					tileG[pos] = 0x20
					tileB[pos] = 0x20
				} else {
					// higher head distance becomes darker (unknown is 0xff)
					headDist := uint32((vPerf >> 24) & 0xff)
					if headDist == 0xff {
						tileR[pos] = 0x30
					} else {
						q := 64 - headDist
						q = (q * q * q * q * q) >> 22
						tileR[pos] = uint8(q)
					}
					// correct target is 0xff, incorrect is 0
					tileG[pos] = byte(vPerf >> 16)
					// higher inclusion distance becomes darker
					inclDist := uint32((vPerf >> 8) & 0xff)
					if inclDist == 0xff {
						tileB[pos] = 0x30
					} else {
						q := 64 - inclDist
						q = (q * q * q * q * q) >> 22
						tileB[pos] = uint8(q)
					}
				}
			}
		}
		for vi := uint64(len(perf)); vi < maxValidators; vi++ {
			tY := vi / tileSize
			tile := tiles[tY]
			tileR := tile[:tileSizeSquared]
			tileG := tile[tileSizeSquared : tileSizeSquared*2]
			tileB := tile[tileSizeSquared*2 : tileSizeSquared*3]
			tileA := tile[tileSizeSquared*3:]

			y := vi % tileSize
			pos := x*tileSize + y
			// transparent pixel
			tileR[pos] = 0
			tileG[pos] = 0
			tileB[pos] = 0
			tileA[pos] = 0
		}
	}
	for tY, tile := range tiles {
		// TODO more types
		key := tileDbKey(tileType, tX, uint64(tY), 0)
		// compress the tile image
		tile = snappy.Encode(nil, tile)
		if err := tilesDB.Put(key, tile, nil); err != nil {
			return fmt.Errorf("failed to write tile %d:%d (zoom 0): %v", tX, tY, err)
		}
	}
	return nil
}

func convTiles(tilesDB *leveldb.DB, tileType uint8, tX uint64, zoom uint8) error {
	for tY := uint64(0); true; tY += 1 {
		topLeft := tileDbKey(tileType, tX*2, tY*2, zoom-1)
		topRight := tileDbKey(tileType, tX*2+1, tY*2, zoom-1)
		// remember, y is downwards
		bottomLeft := tileDbKey(tileType, tX*2, tY*2+1, zoom-1)
		bottomRight := tileDbKey(tileType, tX*2+1, tY*2+1, zoom-1)

		hasY := false
		for _, key := range [][]byte{topLeft, topRight, bottomLeft, bottomRight} {
			if haz, err := tilesDB.Has(key, nil); err != nil {
				return fmt.Errorf("failed to check key presence: %w", err)
			} else if haz {
				hasY = true
			}
		}
		if !hasY {
			break
		}

		getTile := func(key []byte) ([]byte, error) {
			tile, err := tilesDB.Get(key, nil)
			if err == leveldb.ErrNotFound {
				// use empty tile instead
				tile = make([]byte, 4*tileSize*tileSize)
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

		outTile := make([]byte, 4*tileSize*tileSize)
		mix := func(a, b, c, d byte) byte {
			return uint8((uint16(a) + uint16(b) + uint16(c) + uint16(d)) / 4)
		}
		// compress a N x N tile into a N/2 x N/2 tile, writing it to a tile at the given offset.
		compressedFn := func(offX uint64, offY uint64, inTile []byte) {
			for x := uint64(0); x < tileSize/2; x++ {
				for y := uint64(0); y < tileSize/2; y++ {
					// top left, top right, bottom left, bottom right
					p0 := x*2*tileSize + y*2
					p1 := p0 + tileSize
					p2 := p0 + 1
					p3 := p2 + tileSize

					r0, r1, r2, r3 := inTile[p0], inTile[p1], inTile[p2], inTile[p3]
					p0, p1, p2, p3 = p0+tileSizeSquared, p1+tileSizeSquared, p2+tileSizeSquared, p3+tileSizeSquared
					g0, g1, g2, g3 := inTile[p0], inTile[p1], inTile[p2], inTile[p3]
					p0, p1, p2, p3 = p0+tileSizeSquared, p1+tileSizeSquared, p2+tileSizeSquared, p3+tileSizeSquared
					b0, b1, b2, b3 := inTile[p0], inTile[p1], inTile[p2], inTile[p3]
					p0, p1, p2, p3 = p0+tileSizeSquared, p1+tileSizeSquared, p2+tileSizeSquared, p3+tileSizeSquared
					a0, a1, a2, a3 := inTile[p0], inTile[p1], inTile[p2], inTile[p3]

					r := mix(r0, r1, r2, r3)
					g := mix(g0, g1, g2, g3)
					b := mix(b0, b1, b2, b3)
					a := mix(a0, a1, a2, a3)
					pos := (offX+x)*tileSize + (offY + y)
					outTile[pos] = r
					pos += tileSizeSquared
					outTile[pos] = g
					pos += tileSizeSquared
					outTile[pos] = b
					pos += tileSizeSquared
					outTile[pos] = a
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
		if err := tilesDB.Put(key, outTile, nil); err != nil {
			return fmt.Errorf("failed to write tile %d:%d (zoom %d): %v", tX, tY, zoom, err)
		}
	}
	return nil
}

func lastTileEpoch(tilesDB *leveldb.DB, tileType uint8) (common.Epoch, error) {
	iter := tilesDB.NewIterator(util.BytesPrefix(append([]byte(KeyTile), tileType, 0)), nil)
	defer iter.Release()
	if iter.Last() {
		epoch := common.Epoch(binary.BigEndian.Uint32(iter.Key()[3+1+1:3+1+1+4])) * tileSize
		return epoch, nil
	} else {
		return 0, iter.Error()
	}
}

func resetTilesTyped(tilesDB *leveldb.DB, spec *common.Spec, tileType uint8, resetSlot common.Slot) error {
	resetEpoch := spec.SlotToEpoch(resetSlot)

	lastEpoch, err := lastTileEpoch(tilesDB, tileType)
	if err != nil {
		return err
	}

	if resetEpoch > lastEpoch { // check if there's anything to reset
		return nil
	}

	var batch leveldb.Batch
	for z := uint8(0); z < maxZoom; z++ {
		start := uint32(resetEpoch >> z)
		end := uint32(lastEpoch >> z)
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

		iter := tilesDB.NewIterator(r, nil)
		for iter.Next() {
			batch.Delete(iter.Key())
		}
		iter.Release()
	}
	if err := tilesDB.Write(&batch, nil); err != nil {
		return fmt.Errorf("failed to remove tile data of type %d, resetting to slot %d: %v", tileType, resetSlot, err)
	}
	return nil
}

func UpdateTiles(log log.Logger, tiles, perf *leveldb.DB, startEpoch, endEpoch common.Epoch) error {
	if endEpoch < startEpoch {
		return fmt.Errorf("end epoch cannot be lower than start epoch: %d < %d", endEpoch, startEpoch)
	}
	lastPerfEpoch, err := lastPerfEpoch(perf)
	if err != nil {
		return fmt.Errorf("could not read max block slot: %w", err)
	}
	if lastPerfEpoch < endEpoch {
		log.Info("reducing end epoch to available performance data", "end", lastPerfEpoch)
		endEpoch = lastPerfEpoch
	}

	for tX := uint64(startEpoch) / tileSize; tX <= uint64(endEpoch)/tileSize; tX++ {
		log.Info("creating base tiles", "tX", tX, "zoom", 0)
		if err := performanceToTiles(log, tiles, perf, 0, tX); err != nil {
			return fmt.Errorf("failed to update zoom 0 tiles at tX %d: %v", tX, err)
		}
	}

	for z := uint8(1); z <= maxZoom; z++ {
		tileSizeAbs := uint64(tileSize) << z
		tilesXStart := uint64(startEpoch) / tileSizeAbs
		tilesXEnd := (uint64(endEpoch) + tileSizeAbs - 1) / tileSizeAbs
		for i := tilesXStart; i < tilesXEnd; i++ {
			log.Info("computing conv tiles", "tX", i, "zoom", z)
			if err := convTiles(tiles, 0, i, z); err != nil {
				return fmt.Errorf("failed tile convolution layer at zoom %d tX %d: %v", z, i, err)
			}
		}
	}

	log.Info("finished computing tile data")
	return nil
}
