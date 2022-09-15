package yolo

import (
	"fmt"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"io"
)

func (s *Server) performanceToTiles(tileType uint8, tX uint64) error {
	s.log.Info("updating tile column", "type", tileType, "tX", tX, "zoom", 0)
	return performanceToTiles(s.log, s.tiles, s.perf, tileType, tX)
}

func (s *Server) convTiles(tileType uint8, tX uint64, zoom uint8) error {
	s.log.Info("updating tile column", "type", tileType, "tX", tX, "zoom", zoom)

	return convTiles(s.tiles, tileType, tX, zoom)
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
	if lastTileEpoch/tileSize == lastBlockEpoch/tileSize {
		// TODO: override if there are new blocks within the last tile
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

func (s *Server) lastTileEpoch(tileType uint8) (common.Epoch, error) {
	return lastTileEpoch(s.tiles, tileType)
}

func (s *Server) resetTilesTyped(tileType uint8, resetSlot common.Slot) error {
	return resetTilesTyped(s.tiles, s.spec, tileType, resetSlot)
}

func (s *Server) resetTiles(resetSlot common.Slot) error {
	// TODO more types
	return s.resetTilesTyped(0, resetSlot)
}
