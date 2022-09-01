package yolo

import (
	"github.com/protolambda/zrnt/eth2/beacon/altair"
	"github.com/protolambda/zrnt/eth2/beacon/common"
	"github.com/protolambda/zrnt/eth2/beacon/phase0"
	"github.com/protolambda/ztyp/codec"
	"github.com/protolambda/ztyp/tree"
)

type SignedBeaconBlockLH struct {
	Message   BeaconBlockLH       `json:"message" yaml:"message"`
	Signature common.BLSSignature `json:"signature" yaml:"signature"`
}

var _ common.EnvelopeBuilder = (*SignedBeaconBlockLH)(nil)

func (b *SignedBeaconBlockLH) Envelope(spec *common.Spec, digest common.ForkDigest) *common.BeaconBlockEnvelope {
	header := b.Message.Header(spec)
	return &common.BeaconBlockEnvelope{
		ForkDigest:        digest,
		BeaconBlockHeader: *header,
		Body:              &b.Message.Body,
		BlockRoot:         header.HashTreeRoot(tree.GetHashFn()),
		Signature:         b.Signature,
	}
}

func (b *SignedBeaconBlockLH) Deserialize(spec *common.Spec, dr *codec.DecodingReader) error {
	return dr.Container(spec.Wrap(&b.Message), &b.Signature)
}

func (b *SignedBeaconBlockLH) Serialize(spec *common.Spec, w *codec.EncodingWriter) error {
	return w.Container(spec.Wrap(&b.Message), &b.Signature)
}

func (b *SignedBeaconBlockLH) ByteLength(spec *common.Spec) uint64 {
	return codec.ContainerLength(spec.Wrap(&b.Message), &b.Signature)
}

func (b *SignedBeaconBlockLH) FixedLength(*common.Spec) uint64 {
	return 0
}

func (b *SignedBeaconBlockLH) HashTreeRoot(spec *common.Spec, hFn tree.HashFn) common.Root {
	return hFn.HashTreeRoot(spec.Wrap(&b.Message), b.Signature)
}

func (b *SignedBeaconBlockLH) SignedHeader(spec *common.Spec) *common.SignedBeaconBlockHeader {
	return &common.SignedBeaconBlockHeader{
		Message:   *b.Message.Header(spec),
		Signature: b.Signature,
	}
}

type BeaconBlockLH struct {
	Slot          common.Slot           `json:"slot" yaml:"slot"`
	ProposerIndex common.ValidatorIndex `json:"proposer_index" yaml:"proposer_index"`
	ParentRoot    common.Root           `json:"parent_root" yaml:"parent_root"`
	StateRoot     common.Root           `json:"state_root" yaml:"state_root"`
	Body          BeaconBlockBodyLH     `json:"body" yaml:"body"`
}

func (b *BeaconBlockLH) Deserialize(spec *common.Spec, dr *codec.DecodingReader) error {
	return dr.Container(&b.Slot, &b.ProposerIndex, &b.ParentRoot, &b.StateRoot, spec.Wrap(&b.Body))
}

func (b *BeaconBlockLH) Serialize(spec *common.Spec, w *codec.EncodingWriter) error {
	return w.Container(&b.Slot, &b.ProposerIndex, &b.ParentRoot, &b.StateRoot, spec.Wrap(&b.Body))
}

func (b *BeaconBlockLH) ByteLength(spec *common.Spec) uint64 {
	return codec.ContainerLength(&b.Slot, &b.ProposerIndex, &b.ParentRoot, &b.StateRoot, spec.Wrap(&b.Body))
}

func (b *BeaconBlockLH) FixedLength(*common.Spec) uint64 {
	return 0
}

func (b *BeaconBlockLH) HashTreeRoot(spec *common.Spec, hFn tree.HashFn) common.Root {
	return hFn.HashTreeRoot(b.Slot, b.ProposerIndex, b.ParentRoot, b.StateRoot, spec.Wrap(&b.Body))
}

func (b *BeaconBlockLH) Header(spec *common.Spec) *common.BeaconBlockHeader {
	return &common.BeaconBlockHeader{
		Slot:          b.Slot,
		ProposerIndex: b.ProposerIndex,
		ParentRoot:    b.ParentRoot,
		StateRoot:     b.StateRoot,
		BodyRoot:      b.Body.HashTreeRoot(spec, tree.GetHashFn()),
	}
}

type BeaconBlockBodyLH struct {
	RandaoReveal common.BLSSignature `json:"randao_reveal" yaml:"randao_reveal"`
	Eth1Data     common.Eth1Data     `json:"eth1_data" yaml:"eth1_data"`
	Graffiti     common.Root         `json:"graffiti" yaml:"graffiti"`

	ProposerSlashings phase0.ProposerSlashings `json:"proposer_slashings" yaml:"proposer_slashings"`
	AttesterSlashings phase0.AttesterSlashings `json:"attester_slashings" yaml:"attester_slashings"`
	Attestations      phase0.Attestations      `json:"attestations" yaml:"attestations"`
	Deposits          phase0.Deposits          `json:"deposits" yaml:"deposits"`
	VoluntaryExits    phase0.VoluntaryExits    `json:"voluntary_exits" yaml:"voluntary_exits"`

	SyncAggregate altair.SyncAggregate `json:"sync_aggregate" yaml:"sync_aggregate"`

	// header only
	ExecutionPayload common.ExecutionPayloadHeader `json:"execution_payload" yaml:"execution_payload"`
}

func (b *BeaconBlockBodyLH) Deserialize(spec *common.Spec, dr *codec.DecodingReader) error {
	return dr.Container(
		&b.RandaoReveal, &b.Eth1Data,
		&b.Graffiti, spec.Wrap(&b.ProposerSlashings),
		spec.Wrap(&b.AttesterSlashings), spec.Wrap(&b.Attestations),
		spec.Wrap(&b.Deposits), spec.Wrap(&b.VoluntaryExits),
		spec.Wrap(&b.SyncAggregate), &b.ExecutionPayload,
	)
}

func (b *BeaconBlockBodyLH) Serialize(spec *common.Spec, w *codec.EncodingWriter) error {
	return w.Container(
		&b.RandaoReveal, &b.Eth1Data,
		&b.Graffiti, spec.Wrap(&b.ProposerSlashings),
		spec.Wrap(&b.AttesterSlashings), spec.Wrap(&b.Attestations),
		spec.Wrap(&b.Deposits), spec.Wrap(&b.VoluntaryExits),
		spec.Wrap(&b.SyncAggregate), &b.ExecutionPayload,
	)
}

func (b *BeaconBlockBodyLH) ByteLength(spec *common.Spec) uint64 {
	return codec.ContainerLength(
		&b.RandaoReveal, &b.Eth1Data,
		&b.Graffiti, spec.Wrap(&b.ProposerSlashings),
		spec.Wrap(&b.AttesterSlashings), spec.Wrap(&b.Attestations),
		spec.Wrap(&b.Deposits), spec.Wrap(&b.VoluntaryExits),
		spec.Wrap(&b.SyncAggregate), &b.ExecutionPayload,
	)
}

func (b *BeaconBlockBodyLH) FixedLength(*common.Spec) uint64 {
	return 0
}

func (b *BeaconBlockBodyLH) HashTreeRoot(spec *common.Spec, hFn tree.HashFn) common.Root {
	return hFn.HashTreeRoot(
		b.RandaoReveal, &b.Eth1Data,
		b.Graffiti, spec.Wrap(&b.ProposerSlashings),
		spec.Wrap(&b.AttesterSlashings), spec.Wrap(&b.Attestations),
		spec.Wrap(&b.Deposits), spec.Wrap(&b.VoluntaryExits),
		spec.Wrap(&b.SyncAggregate), &b.ExecutionPayload,
	)
}
