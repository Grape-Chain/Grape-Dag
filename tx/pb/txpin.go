package pb

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
	"github.com/Grape-Chain/Grape-Dag/types"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	proto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func NewTxPin(prev []byte) *TxPin {
	if prev == nil {
		prev = []byte{}
	}
	return &TxPin{
		Prev:  prev,
		Ts:    timestamppb.Now(),
		Sites: []*SiteID{},
		Balance: &Balance{
			Balance: map[string][]byte{},
		},
		Pk:   []byte{},
		Sign: []byte{},
	}
}

func (s *SiteID) MarshalJSON() ([]byte, error) {
	id, _ := uuid.FromBytes(s.Id)
	return json.Marshal(struct {
		Id      string `json:"id"`
		Address string `json:"address"`
		IdMajor uint64 `json:"major"`
		IdMinor uint32 `json:"minor"`
	}{
		Id:      id.String(),
		Address: s.Address,
		IdMajor: s.IdMajor,
		IdMinor: s.IdMinor,
	})
}

func (b *Balance) MarshalJSON() ([]byte, error) {
	buf := bytes.Buffer{}
	buf.WriteString("[")
	for k, v := range b.Balance {
		buf.WriteString(fmt.Sprintf("\t{%s : %s},\n", k, big.NewInt(0).SetBytes(v).String()))
	}
	buf.WriteString("],\n")
	return buf.Bytes(), nil
}

func (b *Balance) MarshalJSONShort() ([]byte, error) {
	buf := bytes.Buffer{}
	buf.WriteString("[")
	for k, v := range b.Balance {
		buf.WriteString(fmt.Sprintf("{%s... : %s},", k[:16], big.NewInt(0).SetBytes(v).String()))
	}
	buf.WriteString("],")
	return buf.Bytes(), nil
}

func (t *TxPin) MarshalJSON() ([]byte, error) {
	buf := bytes.Buffer{}
	buf.WriteString("[")
	for _, v := range t.Sites {
		b, e := v.MarshalJSON()
		if e == nil {
			buf.WriteString(string(b))
		}
	}
	buf.WriteString("],")
	sites := buf.String()
	x, _ := t.Balance.MarshalJSON()
	balance := string(x)
	return json.Marshal(struct {
		Prev      string    `json:"prev_tx_signature"`
		Timestamp time.Time `json:"timestamp"`
		Sites     string    `json:"sites"`
		Balances  string    `json:"balances"`
		Pk        string    `json:"public_key"`
		Sign      string    `json:"signature"`
	}{
		Prev:      hex.EncodeToString(t.Prev),
		Timestamp: t.Ts.AsTime(),
		Sites:     sites,
		Balances:  balance,
		Pk:        string(t.Pk),
		Sign:      hex.EncodeToString(t.Sign),
	})
}

func (t *TxPin) MarshalJSONShort() ([]byte, error) {
	x, _ := t.Balance.MarshalJSONShort()
	balance := string(x)
	return json.MarshalIndent(struct {
		Prev      string    `json:"prev_tx_signature"`
		Timestamp time.Time `json:"timestamp"`
		Balances  string    `json:"balances"`
		Sign      string    `json:"signature"`
	}{
		Prev:      hex.EncodeToString(t.Prev),
		Timestamp: t.Ts.AsTime(),
		Balances:  balance,
		Sign:      hex.EncodeToString(t.Sign),
	}, "", "  ")
}

// deterministicMarshal - protobuf output with map entries in a defined order.
//
// The default is deliberately non-deterministic: the Go implementation
// randomises map iteration so that nobody comes to depend on the order. A
// commit transaction carries two maps - the balance map, and missingTargets on
// every site it names - so its encoding, and therefore any hash taken over it,
// varied from one call to the next. That made the signature over a commit
// transaction unverifiable in practice: correctly signed, it failed
// verification four times out of five, and it only went unnoticed because
// nothing verified. It also meant the eth RPC reported a different block hash
// for the same commit transaction on successive calls.
func deterministicMarshal(m proto.Message) ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(m)
}

func (t *TxPin) MarshalBinary() ([]byte, error) {
	return deterministicMarshal(t)
}

func (t *TxPin) UnmarshalBinary(data []byte) error {
	return proto.Unmarshal(data, t)
}

func (t *TxPin) GetHash() types.Hex {
	hash, err := t.Hash(crypto.SHA256)
	if err != nil {
		panic(err)
	}
	return hash
}

func (t *TxPin) Hash(algo crypto.Hash) ([]byte, error) {
	// Get wire format byte sequence from the transaction
	buf, err := deterministicMarshal(t)
	if err != nil {
		return nil, err
	}
	return utils.GetBuilder().Build(crypto.SHA256).Hash(buf), nil
}

// PrototypeHash - the hash a signature over this commit transaction is taken
// over: everything the commit transaction says, and none of the evidence that
// anybody agreed to it.
//
// Excluding the signature is obvious. Excluding the public key is what lets more
// than one signer sign the same bytes, which is the whole point once a quorum of
// validators has to agree rather than a single leader asserting. Substituting a
// key is not a way in: verification checks the signature against whichever key
// is presented, so a swapped key simply fails.
//
// Works on a copy. The version this replaces blanked the signature on the
// receiver, hashed, and put it back, which mutates a commit transaction that
// other goroutines may be reading.
func (t *TxPin) PrototypeHash() ([]byte, error) {
	c, ok := proto.Clone(t).(*TxPin)
	if !ok {
		return nil, errors.New("cannot copy the commit transaction to hash it")
	}
	c.Sign = nil
	c.Pk = nil
	buf, err := deterministicMarshal(c)
	if err != nil {
		return nil, err
	}
	return utils.GetBuilder().Build(crypto.SHA256).Hash(buf), nil
}

func (t *TxPin) SignTx(w *grape_wallet.Wallet) {
	t.Sign = []byte{}
	t.Pk = *w.PublicKey()
	t.generateSignature(w.PrivateKey())
}

// SignerOf - the public key that signed this commit transaction, if it carries
// a signature at all.
func (t *TxPin) SignerOf() []byte { return t.Pk }

func (t *TxPin) VerifyTx() error {
	sz := len(t.Sign)
	if sz < 64 {
		return errors.Errorf("Invalid pin tx signature of length %d", sz)
	}
	payload, err := t.PrototypeHash()
	if err != nil {
		return errors.Errorf("Pin tx marshal binary error: %s", err.Error())
	}
	if !grape_wallet.NewDSA().Verify(t.Pk, t.Sign, payload) {
		return errors.Errorf("Cannot verify pin tx pin=%d", t.PinNumber)
	}
	return nil
}

func (t *TxPin) generateSignature(pk *grape_wallet.PrivateKey) []byte {
	// The same payload the verifier will reconstruct: see PrototypeHash.
	payload, err := t.PrototypeHash()
	if err != nil {
		return nil
	}
	// Sign the hash
	t.Sign = grape_wallet.NewDSA().Sign(*pk, payload)
	if err != nil {
		return nil
	}
	// Get the signature
	return t.Sign
}
