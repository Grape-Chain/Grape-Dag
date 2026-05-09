package run

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"time"

	"github.com/VG-Grape/luna/app"
	config "github.com/VG-Grape/luna/config"
	"github.com/VG-Grape/luna/tx/pb"
	"github.com/VG-Grape/luna/crypto"
	"github.com/enescakir/emoji"
	"google.golang.org/protobuf/proto"
)

type ProcessActivation struct{}

func (p *ProcessActivation) process(c *config.Lunapeer) error {
	if !config.USE_ACTIVATION {
		return nil
	}
	if c.Host.Leader {
		if len(c.Host.Activation) == 0 {
			return fmt.Errorf("in order to run leader node must be activated")
		}
		fd, err := os.OpenFile(c.Host.Activation, os.O_RDONLY, fs.ModeExclusive)
		if err != nil {
			return err
		}
		activation := []byte{}
		buf := make([]byte, 1024)
		for {
			buf = buf[:cap(buf)]
			n, err := fd.Read(buf)
			if err == io.EOF {
				if n > 0 {
					activation = append(activation, buf[:n]...)
				}
				break
			} else if err != nil {
				return err
			}
			activation = append(activation, buf[:n]...)
		}
		act, err := p.validate(activation, c.Host.Secret, c)
		if err != nil {
			return err
		}
		app.GetApp().App_Activation = act
	}
	return nil
}

func (p *ProcessActivation) validate(act []byte, secret string, c *config.Lunapeer) ([]byte, error) {

	dk, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(dk)
	if err != nil {
		panic(err)
	}
	iv := act[:aes.BlockSize]
	verify_stream := cipher.NewCFBDecrypter(block, iv)
	ciphertext := act[aes.BlockSize:]
	// XORKeyStream can work in-place if the two arguments are the same.
	verify_stream.XORKeyStream(ciphertext, ciphertext)

	payload := &pb.Secret{}

	proto.Unmarshal(ciphertext, payload)
	if len(payload.Ico) == 0 || len(payload.Id) == 0 || len(payload.Sign) == 0 {
		return nil, fmt.Errorf("activation failed")
	}
	signature := payload.Sign
	payload.Sign = []byte{}

	payload_pb, _ := proto.Marshal(payload)

	pk, err := luna1crypto.ParsePrivateKey(c.Dag.Privatekey)
	if err != nil {
		if c.Host.Verbose > 2 {
			fmt.Printf("%s\tleader %s private key is invalid %s\n", emoji.StopSign, emoji.Purse, err.Error())
		}
		return nil, fmt.Errorf("activation failed")
	}

	privKey := ed25519.NewKeyFromSeed(pk)
	pubKey := privKey.Public()
	z := pubKey.(ed25519.PublicKey)
	if !ed25519.Verify(z, payload_pb, signature) {
		if c.Host.Verbose > 2 {
			fmt.Printf("Failed to verify the signature %s", emoji.WorriedFace)
		}
		return nil, fmt.Errorf("activation failed")
	}
	if payload.Id != c.Peer.Id {
		return nil, fmt.Errorf("activation failed")
	}
	act_buf := make([]byte, 64)
	binary.LittleEndian.PutUint64(act_buf, 0xa5f0b4c3)
	r := rand.NewSource(time.Now().UnixNano())
	rng := rand.New(r)
	rng.Shuffle(cap(act_buf), func(x, y int) {
		act_buf[x], act_buf[y] = act_buf[y], act_buf[x]
	})

	return act_buf, nil
}

func validateActivation() bool {
	if !config.USE_ACTIVATION {
		return true
	}
	c := config.GetConfig()
	if c.Host.Leader {
		if len(app.GetApp().App_Activation) == 0 {
			return false
		}
		check := app.GetApp().App_Activation
		var z uint64 = 0
		for i := 0; i < 64; i++ {
			x := check[i]
			for y := 0; y < 8; y++ {
				z += uint64(x >> y & 1)
			}
		}
		return z == config.ACTIVATION_OK
	}
	return true
}
