package yolo

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/protolambda/zrnt/eth2/beacon/altair"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/beacon/phase0"
	"github.com/protolambda/ztyp/codec"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	// KeyBlock is a:
	// 3 byte prefix for keying beacon block roots, followed by:
	// 8 byte big-endian slot number
	//
	// The value is a SSZ-encoded beacon block (no compression).
	KeyBlock string = "blk"

	// KeyBlockRoot is a:
	// 3 byte prefix for keying beacon block roots, followed by:
	// 8 byte big-endian slot number
	//
	// The value is the canonical beacon block root, bytes32
	KeyBlockRoot string = "blr"

	// KeyRandaoMix is a:
	// 3 byte prefix for keying randao mixes, followed by:
	// 8 byte big-endian epoch number
	//
	// The value is a 32 byte mix, XOR'ing all past randao inputs together, as defined in the consensus spec.
	KeyRandaoMix string = "rnd"
)

type BlockData struct {
	Slot         common.Slot
	Attestations phase0.Attestations
	RandaoReveal common.BLSSignature
}

var ErrBlockNotFound = errors.New("block not found")

func (s *Server) getBlock(slot common.Slot) (*BlockData, error) {
	var key [3 + 8]byte
	copy(key[:3], KeyBlock)
	binary.BigEndian.PutUint64(key[3:], uint64(slot))
	data, err := s.blocks.Get(key[:], nil)
	if err != nil {
		if err == leveldb.ErrNotFound {
			return nil, ErrBlockNotFound
		}
		return nil, err
	}
	if uint64(s.spec.ALTAIR_FORK_EPOCH)*uint64(s.spec.SLOTS_PER_EPOCH) <= uint64(slot) {
		var dest altair.SignedBeaconBlock
		if err := dest.Deserialize(s.spec, codec.NewDecodingReader(bytes.NewReader(data), uint64(len(data)))); err != nil {
			return nil, err
		}
		return &BlockData{
			Slot:         slot,
			Attestations: dest.Message.Body.Attestations,
			RandaoReveal: dest.Message.Body.RandaoReveal,
		}, nil
	} else {
		var dest phase0.SignedBeaconBlock
		if err := dest.Deserialize(s.spec, codec.NewDecodingReader(bytes.NewReader(data), uint64(len(data)))); err != nil {
			return nil, err
		}
		return &BlockData{
			Slot:         slot,
			Attestations: dest.Message.Body.Attestations,
			RandaoReveal: dest.Message.Body.RandaoReveal,
		}, nil
	}
}

func (s *Server) getBlockRoot(slot common.Slot) (common.Root, error) {
	var key [3 + 8]byte
	copy(key[:3], KeyBlockRoot)
	binary.BigEndian.PutUint64(key[3:], uint64(slot))
	root, err := s.blocks.Get(key[:], nil)
	if err != nil {
		return [32]byte{}, err
	}
	if len(root) != 32 {
		return [32]byte{}, fmt.Errorf("unexpected block root value length (%d): %x", len(root), root)
	}
	return *(*[32]byte)(root), nil
}

func (s *Server) importNextBlock() error {
	if s.lastHeader == nil {
		// TODO try load last block from DB
		// If not in DB, load genesis from API
	}
	// TODO import N+1

	// TODO: on a reorg we want to reset the perf and tiles db
	return nil
}
