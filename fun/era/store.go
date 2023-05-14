package era

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/golang/snappy"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/ztyp/codec"
)

type Store struct {
	// era file paths indexed by state starting-slot
	Files map[common.Slot]string

	// TODO cache open era files
}

var (
	stateBufPool = sync.Pool{New: func() any { return bytes.NewBuffer(make([]byte, 100_000_000)) }}
	blockBufPool = sync.Pool{New: func() any { return bytes.NewBuffer(make([]byte, 10_000_000)) }}
	snappyPool   = sync.Pool{New: func() any { return snappy.NewReader(nil) }}
)

func NewStore() *Store {
	return &Store{
		Files: make(map[common.Slot]string),
	}
}

func (s *Store) Load(dirPath string) error {
	return filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() && strings.HasSuffix(path, ".era") {
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to read %q: %w", path, err)
			}
			defer f.Close()
			end, err := f.Seek(0, io.SeekEnd)
			if err != nil {
				return fmt.Errorf("failed to seek era to end: %w", err)
			}
			startSlot, err := SeekState(f, end)
			if err != nil {
				return fmt.Errorf("failed to seek era to state: %w", err)
			}
			s.Files[startSlot] = path
		}
		return nil
	})
}

func (s *Store) Bounds() (min, max common.Slot) {
	min = ^common.Slot(0)
	max = common.Slot(0)
	for k := range s.Files {
		if k < min {
			min = k
		}
		if k > max {
			max = k
		}
	}
	return
}

func (s *Store) openEra(slot common.Slot) (*os.File, error) {
	p, ok := s.Files[slot]
	if !ok {
		return nil, os.ErrNotExist
	}
	return os.Open(p)
}

func (s *Store) State(slot common.Slot, dest common.SSZObj) error {
	if slot%SlotsPerEra != 0 {
		return fmt.Errorf("can only open states at multiples of era size, but got request for %d", slot)
	}
	f, err := s.openEra(slot)
	if err != nil {
		return fmt.Errorf("failed to open era: %w", err)
	}
	defer f.Close()
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("failed to seek era to end: %w", err)
	}
	startSlot, err := SeekState(f, end)
	if err != nil {
		return fmt.Errorf("failed to seek era to state: %w", err)
	}
	if slot != startSlot {
		return fmt.Errorf("sanity check of state starting-slot slot failed: %w", err)
	}
	sr := snappyPool.Get().(*snappy.Reader)
	defer snappyPool.Put(sr)

	buf := stateBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer stateBufPool.Put(buf)
	if err := CoppySnappyEntry(f, buf, sr, CompressedBeaconStateType); err != nil {
		return fmt.Errorf("failed to read compressed beacon state: %w", err)
	}
	dr := codec.NewDecodingReader(buf, uint64(buf.Len()))
	if err := dest.Deserialize(dr); err != nil {
		return fmt.Errorf("failed to deserialize beacon state: %w", err)
	}
	return nil
}

func (s *Store) Block(slot common.Slot, dest common.SSZObj) error {
	eraSlot := slot - (slot % SlotsPerEra) + SlotsPerEra
	f, err := s.openEra(eraSlot)
	if err != nil {
		return fmt.Errorf("failed to open era: %w", err)
	}
	defer f.Close()
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("failed to seek era to end: %w", err)
	}
	err = SeekBlock(f, uint64(slot%SlotsPerEra), end)
	if err != nil {
		return fmt.Errorf("failed to seek era to block: %w", err)
	}
	sr := snappyPool.Get().(*snappy.Reader)
	defer snappyPool.Put(sr)

	buf := blockBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer blockBufPool.Put(buf)
	if err := CoppySnappyEntry(f, buf, sr, CompressedSignedBeaconBlockType); err != nil {
		return fmt.Errorf("failed to read compressed signed beacon block: %w", err)
	}
	dr := codec.NewDecodingReader(buf, uint64(buf.Len()))
	if err := dest.Deserialize(dr); err != nil {
		return fmt.Errorf("failed to deserialize signed beacon block: %w", err)
	}
	return nil
}
