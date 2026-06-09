package utils

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/mr-tron/base58/base58"
)

const (
	KEYDIR_PATH  = ".grap3"
	KEYFILE_NAME = "grapepk.json"
)

var logger golog.EventLogger

func init() {
	logger = golog.Logger("p2p-pk")
}

func generatePk() (crypto.PrivKey, crypto.PubKey) {
	//r := mrand.New(mrand.NewSource(time.Now().UnixMicro()))
	//prk, puk, err := crypto.GenerateKeyPairWithReader(crypto.RSA, 2048, r)
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)

	if err != nil {
		return nil, nil
	}
	logger.Debug("Successfully generated peer keys")
	return priv, pub
}

func ManagePK(peerID string) crypto.PrivKey {
	// Load the pk file and find pk that belongs to this peer id
	home_dir, _ := os.UserHomeDir()
	key_path := filepath.Join(home_dir, KEYDIR_PATH)
	if _, err := os.Stat(key_path); err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(key_path, os.ModePerm)
			if err != nil {
				logger.Errorf("Failed to create %s. %s", key_path, err)
			}
		}
	} else {
		// If found, return that key
		prvkey := loadKey(key_path, peerID)
		if prvkey == nil {
			logger.Warnf("Failed to load private key for %s. Will generate a new key pair.", peerID)
		} else {
			return prvkey
		}
	}
	// If not found, generate keys, save and return
	prvkey, _ := generatePk()
	err := saveKey(key_path, peerID, prvkey)
	if err != nil {
		logger.Errorf("Failed to save a new key pair for %s", peerID)
		return nil
	}
	return prvkey
}

func loadKey(key_path, peer_id string) crypto.PrivKey {
	keyfile_path := filepath.Join(key_path, KEYFILE_NAME)
	f, err := os.Open(keyfile_path)
	if err != nil {
		if t, ok := err.(*os.PathError); ok {
			if os.IsNotExist(err) {
				logger.Errorf("Failed to open %s. %s", keyfile_path, t.Error())
				return nil
			}
		}
	}
	defer f.Close()
	rr := bufio.NewReader(f)
	jsondata, err := ioutil.ReadAll(rr)
	if err != nil {
		logger.Errorf("Failed to read in the contents of %s. %s", keyfile_path)
		return nil
	}
	var payload map[string]interface{} = make(map[string]interface{})
	err = json.Unmarshal(jsondata, &payload)
	if err != nil {
		logger.Errorf("Failed to unmarshal the contents of %s. %s", keyfile_path, err)
		return nil
	}
	if v, ok := payload[peer_id]; ok {
		s, ok := v.(string)
		if ok {
			keybytes, err := base58.Decode(s)
			if err != nil {
				logger.Errorf("Failed to decode the contents of %s. %s", keyfile_path, err)
				return nil
			}
			prvkey, err := crypto.UnmarshalPrivateKey(keybytes)
			if err != nil {
				logger.Errorf("Failed to unmarshal the private key for %s. %s", peer_id, err)
				return nil
			}
			logger.Debugf("Successfully loaded the private key for %s", peer_id)
			return prvkey
		} else {
			logger.Error("Failed to cast the key value to type string")
			return nil
		}
	}
	return nil
}

func saveKey(key_path, peer_id string, prvkey crypto.PrivKey) error {
	file_exists := true
	keyfile_path := filepath.Join(key_path, KEYFILE_NAME)
	if _, err := os.Stat(keyfile_path); err != nil {
		if t, ok := err.(*os.PathError); ok {
			if os.IsNotExist(err) {
				logger.Warnf("File %s: %s", keyfile_path, t.Error())
				file_exists = false
			}
		}
	}

	var payload map[string]interface{} = make(map[string]interface{})

	if file_exists {
		keys, err := os.ReadFile(keyfile_path)
		if err != nil {
			logger.Errorf("Failed to read from %s. %s", keyfile_path, err)
			return err
		}
		err = json.Unmarshal(keys, &payload)
		if err != nil {
			logger.Errorf("Failed to unmarshal the file contents. %s", err)
			return err
		}
	}
	keybytes, err := crypto.MarshalPrivateKey(prvkey)
	if err != nil {
		logger.Errorf("Failed to marshal the private key for %s. %s", peer_id, err)
		return err
	}
	payload[peer_id] = base58.Encode(keybytes)
	jsonData, err := json.MarshalIndent(payload, "", " ")
	if err != nil {
		logger.Errorf("Failed to marshal the key array to json. %s", err)
		return err
	}
	f, err := os.OpenFile(keyfile_path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		logger.Errorf("Failed to open/creaate file %s. %s", keyfile_path, err)
		return err
	}
	//Save Json Data  into a json file
	nb, err := f.Write(jsonData)
	if err == nil && nb > 0 {
		logger.Debug("Successfully saved the new key pair")
	}
	return err
}
