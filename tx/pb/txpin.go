package pb

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/Grape-Chain/Grape-Dag/types"
	"github.com/Grape-Chain/Grape-Dag/utils"
	grape_wallet "github.com/Grape-Chain/Grape-Dag/crypto"
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

func (t *TxPin) MarshalBinary() ([]byte, error) {
	return proto.Marshal(t)
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
	buf, err := proto.Marshal(t)
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

func (t *TxPin) VerifyTx() error {
	sz := len(t.Sign)
	if sz < 64 {
		return errors.Errorf("Invalid pin tx signature of length %d", sz)
	}
	// We assume that signature is empty when calculating hash, so, make a copy and set it to empty
	// before calculating the hash value
	sigbuf := make([]byte, sz)
	copy(sigbuf, t.Sign)
	t.Sign = []byte{} // reset the sig value before calculating hash
	payload, err := t.Hash(crypto.SHA256)
	if err != nil {
		return errors.Errorf("Pin tx marshal binary error: %s", err.Error())
	}
	// restore signature value for the transaction
	t.Sign = make([]byte, sz)
	copy(t.Sign, sigbuf)

	valid := grape_wallet.NewDSA().Verify(t.Pk, t.Sign, payload)
	if !valid {
		return errors.Errorf("Cannot verify pin tx: %s", t.String())
	}
	return nil
}

func (t *TxPin) generateSignature(pk *grape_wallet.PrivateKey) []byte {
	// generate tx hash
	payload, err := t.Hash(crypto.SHA256)
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
