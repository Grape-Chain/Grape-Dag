package tx

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	pb "github.com/VG-Grape/luna/tx/pb"
	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/crypto"
	proto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const SyncEnvelopeDomain = "lunaone-tx-record"

// Register in https://github.com/multiformats/multicodec/blob/master/table.csv
var SyncEnvelopePayloadType = []byte{0xFF, 0x44}

type SyncVersionType uint8

const (
	STXV0 SyncVersionType = iota
	STVX1
)

type SyncType uint8

const (
	INITIAL SyncType = iota
	SITE
	CURRENT
	GENESIS
	ALL
	LATEST
	MISSING
)

func (e SyncType) String() string {
	return []string{"INITIAL", "SITE", "CURRENT", "GENESIS", "ALL", "LATEST", "MISSING"}[e]
}

type SyncMsgType uint8

const (
	STX_REQUEST SyncMsgType = iota
	STX_RESPONSE
	STX_VOTE_REQUEST
	STX_VOTE_RESPONSE
	STX_ANNOUNCE
	STX_UPDATE
	STX_STOP
	STX_PIN_REQUEST
	STX_PIN_RESPONSE
	STX_SITE_REQUEST
	STX_SITE_RESPONSE
	STX_SNAPSHOT_BALANCE_REQUEST
	STX_SNAPSHOT_BALANCE_RESPONSE
	STX_PIN_DOWNLOAD_REQUEST
	STX_PIN_DOWNLOAD_RESPONSE
)

func (e SyncMsgType) String() string {
	return []string{
		"STX_REQUEST",
		"STX_RESPONSE",
		"STX_VOTE_REQUEST",
		"STX_VOTE_RESPONSE",
		"STX_ANNOUNCE",
		"STX_UPDATE",
		"STX_STOP",
		"STX_PIN_REQUEST",
		"STX_PIN_RESPONSE",
		"STX_SITE_REQUEST",
		"STX_SITE_RESPONSE",
		"STX_SNAPSHOT_BALANCE_REQUEST",
		"STX_SNAPSHOT_BALANCE_RESPONSE",
		"STX_PIN_DOWNLOAD_REQUEST",
		"STX_PIN_DOWNLOAD_RESPONSE",
	}[e]
}

type Syncv1 struct {
	Ver_type    SyncVersionType
	Sync_Type   SyncType
	Msg_Type    SyncMsgType
	Tracking_Id uuid.UUID
	Sender_Pubk []byte // sender's public key: secp256k1
	Timestamp   time.Time
	Data        []byte // optional for payments; mandatory for contracts
	Details     []*anypb.Any
	SyncHash    []byte // Merkel hash
	Signature   []byte // Signature of the serialized transaction bytes by user’s private key
}

func NewSyncv1() *Syncv1 {
	return &Syncv1{
		Tracking_Id: uuid.New(),
		Timestamp:   time.Now(),
	}
}

func NewSyncv1Stop() *Syncv1 {
	return &Syncv1{
		Tracking_Id: uuid.New(),
		Timestamp:   time.Now(),
		Msg_Type:    STX_STOP,
	}
}

func (t *Syncv1) Size() uint32 {
	x := reflect.ValueOf(*t).Type().Size()
	return uint32(x)
}

func (t *Syncv1) MarshalBinary() *pb.Syncv1 {
	tpb := &pb.Syncv1{}
	tpb.Ver = pb.SyncVersionType(t.Ver_type)
	tpb.SyncType = pb.SyncType(t.Sync_Type)
	tpb.MsgType = pb.SyncMsgType(t.Msg_Type)
	trackid, _ := t.Tracking_Id.MarshalBinary()
	tpb.TrackingId = trackid
	tpb.SenderPubk = t.Sender_Pubk
	tpb.Timestamp = timestamppb.New(t.Timestamp)
	tpb.Data = t.Data
	tpb.Details = t.Details
	tpb.SyncHash = t.SyncHash
	tpb.Signature = t.Signature
	return tpb
}

func uuidToProtobuf(id uuid.UUID) []byte {
	bid, err := id.MarshalBinary()
	if err != nil {
		logger.Fatalf("Failed to marshal tx id: %v+")
	}
	return bid
}

func (t *Syncv1) UnmarshalBinary(tpb *pb.Syncv1) {
	t.Ver_type = SyncVersionType(tpb.Ver)
	t.Sync_Type = SyncType(tpb.SyncType)
	t.Msg_Type = SyncMsgType(tpb.MsgType)
	trackid := uuid.UUID{}
	trackid.UnmarshalBinary(tpb.TrackingId)
	t.Tracking_Id = trackid
	t.Sender_Pubk = tpb.SenderPubk
	t.Timestamp = tpb.GetTimestamp().AsTime()
	t.Data = tpb.Data
	t.Details = tpb.Details
	t.SyncHash = tpb.SyncHash
	t.Signature = tpb.Signature
}

func (t *Syncv1) GenerateSignature(pk crypto.PrivKey) []byte {
	// we send messages across pubsub signed, there is no need to
	// generate another signature for the entire record,
	// but it might make sense to sign the payload - the actual transaction
	// as the outter record may undergo some changes
	// t.Signature = []byte{}
	// // get protobuf sync tx
	// pbT := t.MarshalBinary()
	// // create binary payload
	// payload, err := proto.Marshal(pbT)
	// if err != nil {
	// 	logger.Errorf("Syncv1 marshalling error. %s", err.Error())
	// 	return nil
	// }

	// hash of payload
	hashed := sha256.Sum256(t.Data)

	// get as rsa private key
	//rsakey := pk.(*crypto.RsaPrivateKey)

	// generate signature
	var err error
	//	t.Signature, err = rsakey.Sign(hashed[:])
	t.Signature, err = pk.Sign(hashed[:])
	if err != nil {
		logger.Errorf("Failed to sign transaction. %v", err)
		return nil
	}
	// if len(t.Sender_Pubk) > 0 {
	// 	pubkey, err := crypto.UnmarshalPublicKey(t.Sender_Pubk)
	// 	if err != nil {
	// 		logger.Errorf("Failed to unmarshal public key. err: %s", err.Error())
	// 		return nil
	// 	}
	// 	prsakey := pubkey.(*crypto.RsaPublicKey)
	// 	valid, err := prsakey.Verify(hashed[:], t.Signature)
	// 	if err != nil || !valid {
	// 		logger.Errorf("Failed to validate transaction")
	// 		return nil
	// 	}

	// 	valid, err = t.VerifySignature(pubkey)
	// }
	return t.Signature
}

func (t *Syncv1) VerifySignature(pk crypto.PubKey) (bool, error) {
	// sig := make([]byte, len(t.Signature))
	// n := copy(sig, t.Signature)
	// if n != len(sig) && n != len(t.Signature) {
	// 	err := fmt.Errorf("%s  ~ %s cannot copy signature %s", emoji.RoundPushpin, emoji.Warning, emoji.Locked)
	// 	logger.Error(err.Error())
	// 	return false, err
	// }
	// // reset the original signature
	// t.Signature = []byte{}
	// defer copy(t.Signature, sig)
	// // to protobuf
	// pbT := t.MarshalBinary()
	// // to binary payload
	// payload, err := proto.Marshal(pbT)
	// if err != nil {
	// 	return false, fmt.Errorf("Syncv1 marshalling error. %s", err.Error())
	//}
	// hash of payload
	hashed := sha256.Sum256(t.Data)
	// to rsa pub key
	// rsakey := pk.(*crypto.RsaPublicKey)
	// verify sig
	//flag, err := rsakey.Verify(hashed[:], t.Signature)
	flag, err := pk.Verify(hashed[:], t.Signature)
	if err != nil {
		return false, fmt.Errorf("failed to verify transaction. %s", err.Error())
	}
	return flag, nil
}

func (t *Syncv1) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		VerType    uint8     `json:"ver_type"`
		SyncType   uint8     `json:"sync_type"`
		MsgType    uint8     `json:"msg_type"`
		TrackingId uuid.UUID `json:"tracking_id"`
		SenderPubk []byte    `json:"sender_pubk"`
		Timestamp  time.Time `json:"timestamp"`
		Data       []byte    `json:"data"`
	}{
		VerType:    uint8(t.Ver_type),
		SyncType:   uint8(t.Sync_Type),
		MsgType:    uint8(t.Msg_Type),
		TrackingId: t.Tracking_Id,
		SenderPubk: t.Sender_Pubk,
		Timestamp:  t.Timestamp,
		Data:       t.Data,
	})
}

func (t *Syncv1) MarshalJSONShort() ([]byte, error) {
	sndpubkey := hex.EncodeToString(t.Sender_Pubk)
	l := func(l int) int {
		if l > 33 {
			return 33
		}
		return l
	}(len(sndpubkey))
	return json.MarshalIndent(struct {
		VerType    uint8     `json:"ver_type"`
		SyncType   uint8     `json:"sync_type"`
		MsgType    uint8     `json:"msg_type"`
		TrackingId uuid.UUID `json:"tracking_id"`
		SenderPubk string    `json:"sender_pubk"`
		Timestamp  string    `json:"timestamp"`
		Data       bool      `json:"data_present"`
		Details    bool      `json:"details_present"`
	}{
		VerType:    uint8(t.Ver_type),
		SyncType:   uint8(t.Sync_Type),
		MsgType:    uint8(t.Msg_Type),
		TrackingId: t.Tracking_Id,
		SenderPubk: sndpubkey[:l] + "...",
		Timestamp:  t.Timestamp.String(),
		Data:       t.Data != nil,
		Details:    t.Details != nil,
	}, "", "  ")
}

func (t *Syncv1) String() string {
	var out bytes.Buffer
	var in []byte
	in, err := t.MarshalJSONShort()
	if err != nil {
		logger.Errorf("Failed to marshal tx to json. %v", err)
		return ""
	}
	err = json.Indent(&out, in, "", "\t")
	if err != nil {
		logger.Errorf("Failed to indent json payload. %v", err)
		return ""
	}
	return out.String()
}

func (t *Syncv1) UnmarshalJSON(data []byte) error {
	var ver_type uint8
	var sync_type uint8
	var msg_type uint8
	v := &struct {
		VerType    *uint8       `json:"ver_type"`
		SyncType   *uint8       `json:"sync_type"`
		MsgType    *uint8       `json:"msg_type"`
		TrackingId *uuid.UUID   `json:"tracking_id"`
		SenderPubk *[]byte      `json:"sender_pubk"`
		Timestamp  *time.Time   `json:"timestamp"`
		Data       *[]byte      `json:"data"`
		SyncHash   *[]byte      `json:"sync_hash"`
		TxIDs      *[]uuid.UUID `json:"tx_ids"`
		Vertices   *[]Vertex    `json:"vertices"`
		Edges      *[]Edge      `json:"edges"`
	}{
		VerType:    &ver_type,
		SyncType:   &sync_type,
		MsgType:    &msg_type,
		TrackingId: &t.Tracking_Id,
		SenderPubk: &t.Sender_Pubk,
		Timestamp:  &t.Timestamp,
		Data:       &t.Data,
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	t.Ver_type = SyncVersionType(ver_type)
	t.Sync_Type = SyncType(sync_type)
	t.Msg_Type = SyncMsgType(msg_type)
	return nil
}

// Domain is used when signing and validating PeerRecords contained in Envelopes.
// It is constant for all PeerRecord instances.
func (r *Syncv1) Domain() string {
	return SyncEnvelopeDomain
}

// Codec is a binary identifier for the Syncv1 type
func (r *Syncv1) Codec() []byte {
	return SyncEnvelopePayloadType
}

// UnmarshalRecord parses a Synv1 from a byte slice.
// Note: This method is called automatically when consuming a record.Envelope
// whose PayloadType indicates that it contains a Syncv1.
// Warning: do not call this method directly
func (r *Syncv1) UnmarshalRecord(bytes []byte) (err error) {
	if r == nil {
		return fmt.Errorf("cannot unmarshal Syncv1 to nil Syncv1 receiver")
	}

	defer func() { SyncHandlePanic(recover(), &err, "Syncv1 transaction unmarshal") }()

	var msg pb.Syncv1
	err = proto.Unmarshal(bytes, &msg)
	if err != nil {
		return err
	}

	rPtr, err := Syncv1FromProtobuf(&msg)
	if err != nil {
		return err
	}
	*r = *rPtr

	return nil
}

func SyncHandlePanic(rerr interface{}, err *error, where string) {
	if rerr != nil {
		*err = fmt.Errorf("panic in %s: %s", where, rerr)
	}
}

// MarshalRecord serializes a Syncv1 to a byte slice.
// Note: This method is called automatically when constructing a routing.Envelope
// using Seal or PeerRecord.Sign.
func (r *Syncv1) MarshalRecord() (res []byte, err error) {
	defer func() { SyncHandlePanic(recover(), &err, "Syncv1 transaction marshal") }()

	msg, err := r.ToProtobuf()
	if err != nil {
		return nil, err
	}
	return proto.Marshal(msg)
}

// ToProtobuf returns the equivalent Protocol Buffer struct object of a Syncv1
func (r *Syncv1) ToProtobuf() (*pb.Syncv1, error) {
	return &pb.Syncv1{
		Ver:        pb.SyncVersionType(r.Ver_type),
		SyncType:   pb.SyncType(r.Sync_Type),
		MsgType:    pb.SyncMsgType(r.Msg_Type),
		TrackingId: uuidToProtobuf(r.Tracking_Id),
		SenderPubk: r.Sender_Pubk,
		Timestamp:  timestamppb.New(r.Timestamp),
		Data:       r.Data,
		Details:    r.Details,
		Signature:  r.Signature,
	}, nil
}

func Syncv1FromProtobuf(msg *pb.Syncv1) (*Syncv1, error) {
	record := &Syncv1{}

	record.Ver_type = SyncVersionType(msg.Ver)
	record.Sync_Type = SyncType(msg.SyncType)
	record.Msg_Type = SyncMsgType(msg.MsgType)
	trackid := uuid.UUID{}
	trackid.UnmarshalBinary(msg.TrackingId)
	record.Tracking_Id = trackid
	record.Sender_Pubk = msg.SenderPubk
	record.Timestamp = msg.GetTimestamp().AsTime()
	record.Data = msg.Data
	record.Details = msg.Details
	record.Signature = msg.Signature

	return record, nil
}
