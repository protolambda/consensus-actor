package era

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/golang/snappy"
	"github.com/protolambda/zrnt/eth2/beacon/common"
)

// Era file format
//
// from specs: https://github.com/status-im/nimbus-eth2/blob/stable/docs/e2store.md
//
// entry commons:
//   header := type | length | reserved
//   type := [2]byte
//   length := LE uint32
//   reserved := [2]byte zeroes
//
// Version := header | data
//   type: [0x65, 0x32]
//   length: 0
//   data: []
//
// SlotIndex := header | data
//   type: [0x69, 0x32]
//   data: starting-slot | index | index | index ... | count
//
// era := group+
// group := Version | block* | era-state | other-entries* | slot-index(block)? | slot-index(state)
// block := CompressedSignedBeaconBlock
// era-state := CompressedBeaconState
// slot-index(block) := SlotIndex where count == 8192
// slot-index(state) := SlotIndex where count == 1

const (
	headerSize         = 8
	slotIndexOverhead  = headerSize + 8 + 8 // starting-slot, count
	stateSlotIndexSize = slotIndexOverhead + 8
	SlotsPerEra        = 8192
	blockSlotIndexSize = slotIndexOverhead + 8*SlotsPerEra
)

var ErrNotExist = errors.New("entry does not exist")

func Tell(f io.Seeker) (int64, error) {
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, fmt.Errorf("can't tell current offset: %w", err)
	}
	return offset, nil
}

type EntryType [2]byte

var (
	SlotIndexType                   = EntryType{'i', '2'} // starting-slot | index | index | index ... | count
	VersionType                     = EntryType{'e', '2'} // always 0-length
	CompressedSignedBeaconBlockType = EntryType{1, 0}
	CompressedBeaconStateType       = EntryType{2, 0}
	EmptyType                       = EntryType{0, 0} // may have a length, data should be skipped
)

func CopyEntry(f io.Reader, w io.Writer) (EntryType, error) {
	typ, length, err := ReadHeader(f)
	if err != nil {
		return EmptyType, fmt.Errorf("failed to read entry header: %w", err)
	}
	_, err = io.CopyN(w, f, int64(length))
	return typ, err
}

func CoppySnappyEntry(f io.Reader, w io.Writer, sr *snappy.Reader, expectType EntryType) error {
	typ, length, err := ReadHeader(f)
	if err != nil {
		return fmt.Errorf("failed to read entry header: %w", err)
	}
	if typ != expectType {
		return fmt.Errorf("expected type %x but got type %x", expectType, typ)
	}
	sr.Reset(io.LimitReader(f, int64(length)))
	_, err = io.Copy(w, sr)
	if err != nil {
		return fmt.Errorf("failed to copy snappy output into writer: %w", err)
	}
	return nil
}

func ReadHeader(f io.Reader) (EntryType, uint32, error) {
	var x [8]byte
	if _, err := io.ReadFull(f, x[:]); err != nil {
		return EmptyType, 0, fmt.Errorf("failed to read header: %w", err)
	}
	if x[6] != 0 || x[7] != 0 {
		return EmptyType, 0, fmt.Errorf("reserved value is not 0, got %04x", x[6:])
	}
	return EntryType{x[0], x[1]}, binary.LittleEndian.Uint32(x[2:6]), nil
}

func ReadUint64(f io.Reader) (uint64, error) {
	var x [8]byte
	if _, err := io.ReadFull(f, x[:]); err != nil {
		return 0, fmt.Errorf("failed to read value: %w", err)
	}
	return binary.LittleEndian.Uint64(x[:]), nil
}

func ReadInt64(f io.Reader) (int64, error) {
	x, err := ReadUint64(f)
	return int64(x), err
}

func ReadSlot(f io.Reader) (common.Slot, error) {
	x, err := ReadUint64(f)
	return common.Slot(x), err
}

// ReadBlockOffset reads the file offset of block i (slot relative to group slot-index).
// Seeker f must be positioned at the end of a group.
// Returns offset relative to start of file.
func ReadBlockOffset(f io.ReadSeeker, i uint64) (int64, error) {
	// find start of block slot-index entry, then skip header (8) and starting-slot (8) to find indices.
	x := 8 + 8 + 8*int64(i)
	n, err := f.Seek(-stateSlotIndexSize-blockSlotIndexSize+x, io.SeekCurrent)
	if err != nil {
		return 0, fmt.Errorf("failed to lookup first block offset: %w", err)
	}
	offset, err := ReadInt64(f)
	if err != nil {
		return 0, fmt.Errorf("failed to read offset: %w", err)
	}
	result := n - x + offset
	if result == 0 {
		return 0, ErrNotExist
	}
	return result, nil
}

// ReadStateOffsetAndSlot reads the file offset of the state.
// Seeker f must be positioned at the end of a group.
// Returns offset relative to start of file.
// Seeker f will be positioned where it started after a successful read.
// The slot of the state is returned as well.
func ReadStateOffsetAndSlot(f io.ReadSeeker) (offset int64, slot common.Slot, err error) {
	n, err := f.Seek(-stateSlotIndexSize, io.SeekCurrent)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to lookup state slot-index: %w", err)
	}

	typ, length, err := ReadHeader(f)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read state slot-index header: %w", err)
	}
	if typ != SlotIndexType {
		return 0, 0, fmt.Errorf("expected state slot-index type: %w", err)
	}
	if length != 8*3 {
		return 0, 0, fmt.Errorf("unexpected state slot-index size: %d", length)
	}

	slot, err = ReadSlot(f)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read starting slot: %w", err)
	}

	offset, err = ReadInt64(f)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read offset: %w", err)
	}

	count, err := ReadUint64(f)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read count: %w", err)
	}
	if count != 1 {
		return 0, 0, fmt.Errorf("unexpected number of states: %w", err)
	}

	return n + offset, slot, nil
}

func SeekGroupStart(f io.ReadSeeker, groupEnd int64) error {
	_, err := f.Seek(groupEnd, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to end of group: %w", err)
	}
	for i := uint64(0); i < SlotsPerEra; i++ {
		if err := SeekBlock(f, i, groupEnd); err == ErrNotExist {
			continue
		} else if err != nil {
			return fmt.Errorf("failed to seek to block %d to find group start: %w", i, err)
		}
		if _, err := f.Seek(-8, io.SeekCurrent); err != nil {
			return fmt.Errorf("failed to skip version part before first block entry (%d): %w", i, err)
		}
		return nil
	}
	if _, err := SeekState(f, groupEnd); err != nil {
		return fmt.Errorf("failed to seek to state to find group start: %w", err)
	}
	_, err = f.Seek(-8, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to skip version part before state: %w", err)
	}
	return nil
}

func SeekBlock(f io.ReadSeeker, i uint64, groupEnd int64) error {
	if _, err := f.Seek(groupEnd, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to end of group: %w", err)
	}
	offset, err := ReadBlockOffset(f, i)
	if offset == 0 {
		return ErrNotExist
	}
	if err != nil {
		return fmt.Errorf("failed to read block %d offset: %w", i, err)
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to block %d at offset %d: %w", i, offset, err)
	}
	return nil
}

func SeekState(f io.ReadSeeker, groupEnd int64) (slot common.Slot, err error) {
	if _, err := f.Seek(groupEnd, io.SeekStart); err != nil {
		return 0, fmt.Errorf("failed to seek to end of group: %w", err)
	}
	offset, v, err := ReadStateOffsetAndSlot(f)
	if err != nil {
		return 0, fmt.Errorf("failed to read state offset: %w", err)
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, fmt.Errorf("failed to seek to state at offset %d: %w", offset, err)
	}
	return v, nil
}
