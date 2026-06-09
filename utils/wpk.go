package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Grape-Chain/Grape-Dag/crypto"
)

func checkFsRequirements() (string, error) {
	// Load the pk file and find pk that belongs to this peer id
	home_dir, _ := os.UserHomeDir()
	key_path := filepath.Join(home_dir, KEYDIR_PATH)
	if _, err := os.Stat(key_path); err != nil {
		logger.Errorf("checking file system requirements err:%s", err.Error())
		if os.IsNotExist(err) {
			err = os.MkdirAll(key_path, os.ModePerm)
			if err != nil {
				return key_path, fmt.Errorf("Failed to create %s. %s", key_path, err)
			}
		}
	}
	return key_path, nil
}

func LoadWalletKey(wallet string) (grape1crypto.PrivateKey, grape1crypto.PublicKey) {
	key_path, err := checkFsRequirements()
	if err != nil {
		logger.Errorf("Load wallet %s key err: %s", err.Error())
		return nil, nil
	}
	keyfile_path := filepath.Join(key_path, KEYFILE_NAME)
	f, err := os.Open(keyfile_path)
	if err != nil {
		if t, ok := err.(*os.PathError); ok {
			if os.IsNotExist(err) {
				logger.Errorf("Failed to open %s. %s", keyfile_path, t.Error())
				return nil, nil
			}
		}
	}
	defer f.Close()
	rr := bufio.NewReader(f)
	jsondata, err := ioutil.ReadAll(rr)
	if err != nil {
		logger.Errorf("Failed to read in the contents of %s. %s", keyfile_path)
		return nil, nil
	}
	var payload map[string]interface{} = make(map[string]interface{})
	err = json.Unmarshal(jsondata, &payload)
	if err != nil {
		logger.Errorf("Failed to unmarshal the contents of %s. %s", keyfile_path, err)
		return nil, nil
	}
	if v, ok := payload[wallet]; ok {
		s, ok := v.(string)
		if ok {
			keys := strings.Split(s, "|")
			if len(keys) == 2 {
				logger.Debugf("Successfully loaded the private key [%s] for wallet %s", s, wallet)
				privkey, err := grape1crypto.ParsePrivateKey(keys[0])
				if err != nil {
					logger.Errorf("Private key string to byte array. err: %s", err.Error())
					return nil, nil
				}
				pubkey, err := grape1crypto.ParsePublicKey(keys[1])
				if err != nil {
					logger.Errorf("Public key string to byte array. err: %s", err.Error())
					return nil, nil
				}
				return privkey, pubkey
			}
			logger.Errorf("Failed to extract private/public keys from %s", s)
		} else {
			logger.Error("Failed to cast the key value to type string")
			return nil, nil
		}
	}
	return nil, nil
}

func SaveWalletKey(wallet string, prvkey string, pubkey string) error {
	key_path, err := checkFsRequirements()
	if err != nil {
		logger.Errorf("Save wallet %s key err: %s", err.Error())
		return nil
	}
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
	payload[wallet] = prvkey + "|" + pubkey
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
		logger.Debugf("Successfully saved the new key for wallet %s", wallet)
	}
	return err
}
