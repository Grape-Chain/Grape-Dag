package tx

import (
	"fmt"
	"sync"
	"time"

	pb "github.com/VG-Grape/luna/tx/pb"
	"github.com/google/uuid"
	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/record"
	"github.com/mr-tron/base58/base58"
	ma "github.com/multiformats/go-multiaddr"
	proto "google.golang.org/protobuf/proto"
)

type LunaTx struct {
	PeerID      peer.ID
	Seq         uint64
	Tx          string
	Addrs       []ma.Multiaddr
	Version     VersionType
	Ids         Ids
	Transaction Transaction
	TxIdMajor   uint64
	TxIdMinor   uint32
	TxIdId      uuid.UUID
}

var logger golog.EventLogger = golog.Logger("p2p-lunatx")
var chaintype = PRIVATE_TESTNET

func init() {
	record.RegisterType(&LunaTx{})
	//chaintype = ChainType(config.GetLunaPeerFromConfig().Peer.Network)
}

const LunaTxEnvelopeDomain = "lunaone-tx-record"

// Register in https://github.com/multiformats/multicodec/blob/master/table.csv
var LunaTxEnvelopePayloadType = []byte{0xFF, 0x43}

// NewPeerRecord returns a PeerRecord with a timestamp-based sequence number.
// The returned record is otherwise empty and should be populated by the caller.
func NewLunaTx(host host.Host) *LunaTx {
	return &LunaTx{
		PeerID:      host.ID(),
		Addrs:       host.Network().ListenAddresses(),
		Seq:         TimestampSeq(),
		Version:     VersionType(TVX1),
		Ids:         Ids{},
		Transaction: NewTxv1(chaintype),
		TxIdMajor:   0,
		TxIdMinor:   0,
		TxIdId:      uuid.UUID{},
	}
}

func ConvertToLunaTx(host host.Host, txvx Transaction) *LunaTx {
	rec := NewLunaTx(host)
	rec.Transaction = txvx
	return rec
}

// PeerRecordFromAddrInfo creates a PeerRecord from an AddrInfo struct.
// The returned record will have a timestamp-based sequence number.
func LunaTxFromAddrInfo(info peer.AddrInfo) *LunaTx {
	rec := &LunaTx{Seq: TimestampSeq()}
	rec.PeerID = info.ID
	rec.Addrs = info.Addrs
	return rec
}

// PeerRecordFromProtobuf creates a PeerRecord from a protobuf PeerRecord
// struct.
func LunaTxFromProtobuf(msg *pb.LunaTxRecord) (*LunaTx, error) {
	record := &LunaTx{}

	var id peer.ID
	// if err := id.UnmarshalBinary(msg.PeerId); err != nil {
	// 	return nil, err
	// }
	_ = id.UnmarshalText(msg.PeerId[:])
	record.PeerID = id
	record.Addrs = addrsFromProtobuf(msg.Addresses)
	record.Seq = msg.Seq
	payload, _ := base58.Decode(string(msg.Tx[:]))
	record.Tx = string(payload[:])
	record.Version = VersionType(msg.Version)
	if record.Version == TVX1 {
		t := UnmarshalBinary(msg.GetTxv1())
		record.Transaction = t
	} else {
		logger.Errorf("Unsupported version [%d] of transaction protocol", record.Version)
	}
	record.Ids.UnmarshalBinary(msg.Ids)
	record.TxIdMajor = msg.Txidmajor
	record.TxIdMinor = msg.Txidminor
	record.TxIdId = uuid.UUID{}
	record.TxIdId.UnmarshalBinary(msg.Txidid)
	return record, nil
}

var (
	lastTimestampMu sync.Mutex
	lastTimestamp   uint64
)

// TimestampSeq generates a timestamp-based sequence number for a LunaTx
func TimestampSeq() uint64 {
	now := uint64(time.Now().UnixNano())
	lastTimestampMu.Lock()
	defer lastTimestampMu.Unlock()
	// Need these sequence numbers to be strictly increasing
	if now <= lastTimestamp {
		now = lastTimestamp + 1
	}
	lastTimestamp = now
	return now
}

// Domain is used when signing and validating PeerRecords contained in Envelopes.
// It is constant for all PeerRecord instances.
func (r *LunaTx) Domain() string {
	return LunaTxEnvelopeDomain
}

// Codec is a binary identifier for the LunaTx type
func (r *LunaTx) Codec() []byte {
	return LunaTxEnvelopePayloadType
}

// UnmarshalRecord parses a LunaTx from a byte slice.
// Note: This method is called automatically when consuming a record.Envelope
// whose PayloadType indicates that it contains a LunaTx.
// Warning: do not call this method directly
func (r *LunaTx) UnmarshalRecord(bytes []byte) (err error) {
	if r == nil {
		return fmt.Errorf("cannot unmarshal LunaTx to nil LunaTx receiver")
	}

	defer func() { HandlePanic(recover(), &err, "LunaTx transaction unmarshal") }()

	var msg pb.LunaTxRecord
	err = proto.Unmarshal(bytes, &msg)
	if err != nil {
		return err
	}

	rPtr, err := LunaTxFromProtobuf(&msg)
	if err != nil {
		return err
	}
	*r = *rPtr

	return nil
}

func HandlePanic(rerr interface{}, err *error, where string) {
	if rerr != nil {
		*err = fmt.Errorf("panic in %s: %s", where, rerr)
	}
}

// MarshalRecord serializes a LunaTx to a byte slice.
// Note: This method is called automatically when constructing a routing.Envelope
// using Seal or PeerRecord.Sign.
func (r *LunaTx) MarshalRecord() (res []byte, err error) {
	defer func() { HandlePanic(recover(), &err, "LunaTx transaction marshal") }()

	msg, err := r.ToProtobuf()
	if err != nil {
		return nil, err
	}
	return proto.Marshal(msg)
}

// Equal returns true if the other LunaTx is identical to this one.
func (r *LunaTx) Equal(other *LunaTx) bool {
	if other == nil {
		return r == nil
	}
	if r.PeerID != other.PeerID {
		return false
	}
	if r.Seq != other.Seq {
		return false
	}
	if len(r.Addrs) != len(other.Addrs) {
		return false
	}
	for i := range r.Addrs {
		if !r.Addrs[i].Equal(other.Addrs[i]) {
			return false
		}
	}
	return true
}

// ToProtobuf returns the equivalent Protocol Buffer struct object of a LunaTx
func (r *LunaTx) ToProtobuf() (*pb.LunaTxRecord, error) {
	idBytes, err := r.PeerID.MarshalBinary()
	if err != nil {
		return nil, err
	}
	payload := base58.Encode([]byte(r.Tx))
	ids := r.Ids.MarshalBinary()
	idid, err := r.TxIdId.MarshalBinary()
	if err != nil {
		logger.Fatalf("Failed to marshal LunaTx.TxIdId %v+", err)
		return nil, err
	}
	return &pb.LunaTxRecord{
		PeerId:      idBytes,
		Tx:          []byte(payload),
		Addresses:   addrsToProtobuf(r.Addrs),
		Seq:         r.Seq,
		Version:     pb.VersionType(r.Version),
		Ids:         ids,
		Transaction: &pb.LunaTxRecord_Txv1{Txv1: r.Transaction.MarshalBinary()},
		Txidmajor:   r.TxIdMajor,
		Txidminor:   r.TxIdMinor,
		Txidid:      idid,
	}, nil
}

func addrsFromProtobuf(addrs []*pb.LunaTxRecord_AddressInfo) []ma.Multiaddr {
	var out []ma.Multiaddr
	for _, addr := range addrs {
		a, err := ma.NewMultiaddrBytes(addr.Multiaddr)
		if err != nil {
			continue
		}
		out = append(out, a)
	}
	return out
}

func addrsToProtobuf(addrs []ma.Multiaddr) []*pb.LunaTxRecord_AddressInfo {
	var out []*pb.LunaTxRecord_AddressInfo
	for _, addr := range addrs {
		out = append(out, &pb.LunaTxRecord_AddressInfo{Multiaddr: addr.Bytes()})
	}
	return out
}
