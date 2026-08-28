package pb

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
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
	// And the quorum certificate, for the same reason: the validators are
	// signing this hash, so it cannot depend on how many of them have signed
	// yet.
	c.Quorum = nil
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

// QuorumSigners - the distinct validators whose signatures over this commit
// transaction's prototype hash check out, restricted to the given set.
//
// Returns the signers rather than a count, so a caller can say which validators
// agreed and not merely how many - which is what makes a disagreement
// diagnosable rather than just a number that is too small.
func (t *TxPin) QuorumSigners(validators map[string]struct{}) ([]string, error) {
	cert := t.GetQuorum()
	if cert == nil {
		return nil, errors.Errorf("commit transaction pin=%d carries no quorum certificate", t.PinNumber)
	}
	if cert.PinNumber != t.PinNumber {
		return nil, errors.Errorf("quorum certificate is for pin=%d but the commit transaction is pin=%d",
			cert.PinNumber, t.PinNumber)
	}
	payload, err := t.PrototypeHash()
	if err != nil {
		return nil, errors.Errorf("cannot hash commit transaction pin=%d: %s", t.PinNumber, err.Error())
	}
	if len(cert.PinHash) == 0 || !bytes.Equal(cert.PinHash, payload) {
		// The certificate names the bytes the validators agreed to. If they are
		// not the bytes in hand, the signatures are evidence about a different
		// commit transaction.
		//
		// Defence in depth rather than the load-bearing check: each signature is
		// verified against the recomputed hash below, so a certificate lifted
		// from another commit transaction fails there too. This one turns that
		// into a clear diagnosis instead of "not enough signatures".
		return nil, errors.Errorf("quorum certificate for pin=%d certifies a different commit transaction", t.PinNumber)
	}

	dsa := grape_wallet.NewDSA()
	seen := make(map[string]struct{}, len(cert.Signatures))
	signers := make([]string, 0, len(cert.Signatures))
	for _, sig := range cert.Signatures {
		if sig == nil || len(sig.Pk) == 0 || len(sig.Sign) < 64 {
			continue
		}
		key := strings.ToLower(hex.EncodeToString(sig.Pk))
		if _, dup := seen[key]; dup {
			// One validator signing twice is one validator.
			continue
		}
		if _, isValidator := validators[key]; !isValidator {
			continue
		}
		if !dsa.Verify(grape_wallet.PublicKey(sig.Pk), sig.Sign, payload) {
			continue
		}
		seen[key] = struct{}{}
		signers = append(signers, key)
	}
	return signers, nil
}

// VerifyQuorum - does this commit transaction carry agreement from at least
// `quorum` distinct members of the validator set?
func (t *TxPin) VerifyQuorum(validators map[string]struct{}, quorum int) error {
	if quorum < 1 {
		return errors.New("a quorum of fewer than one signature is not a quorum")
	}
	signers, err := t.QuorumSigners(validators)
	if err != nil {
		return err
	}
	if len(signers) < quorum {
		return errors.Errorf("commit transaction pin=%d carries %d valid validator signature(s), quorum is %d",
			t.PinNumber, len(signers), quorum)
	}
	return nil
}

// SignAsValidator - add this wallet's agreement to the commit transaction's
// certificate, replacing any signature it had already contributed.
func (t *TxPin) SignAsValidator(w *grape_wallet.Wallet, round uint32) error {
	payload, err := t.PrototypeHash()
	if err != nil {
		return errors.Errorf("cannot hash commit transaction pin=%d: %s", t.PinNumber, err.Error())
	}
	if t.Quorum == nil {
		t.Quorum = &QuorumCert{PinNumber: t.PinNumber, PinHash: payload, Round: round}
	}
	// The certificate has to name the same bytes every signer signed.
	t.Quorum.PinNumber = t.PinNumber
	t.Quorum.PinHash = payload
	pk := *w.PublicKey()
	kept := t.Quorum.Signatures[:0]
	for _, sig := range t.Quorum.Signatures {
		if sig != nil && !bytes.Equal(sig.Pk, pk) {
			kept = append(kept, sig)
		}
	}
	t.Quorum.Signatures = append(kept, &ValidatorSignature{
		Pk:   pk,
		Sign: grape_wallet.NewDSA().Sign(*w.PrivateKey(), payload),
	})
	return nil
}
