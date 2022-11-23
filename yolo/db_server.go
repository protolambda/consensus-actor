package yolo

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
)

func (s *Server) closeDBs() error {
	s.blocksLock.Lock()
	s.randaoLock.Lock()
	s.perfLock.Lock()
	s.tilesLock.Lock()

	defer s.blocksLock.Unlock()
	defer s.randaoLock.Unlock()
	defer s.perfLock.Unlock()
	defer s.tilesLock.Unlock()

	var result error
	if s.blocks != nil {
		if err := s.blocks.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close blocks db: %w", err))
		}
	}
	if s.randao != nil {
		if err := s.randao.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close randao db: %w", err))
		}
	}
	if s.perf != nil {
		if err := s.perf.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close perf db: %w", err))
		}
	}
	if s.tiles != nil {
		if err := s.tiles.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close tiles db: %w", err))
		}
	}
	if s.lhChainSnapshot != nil {
		s.lhChainSnapshot.Release()
	}
	if s.lhChainDB != nil {
		if err := s.lhChainDB.Close(); err != nil {
			result = multierror.Append(result, fmt.Errorf("failed to close lh chain db: %w", err))
		}
	}
	return result
}
