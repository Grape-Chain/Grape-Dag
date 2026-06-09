package stream

import (
	"bufio"
	"fmt"
	"strings"

	golog "github.com/ipfs/go-log/v2"
)

var logger golog.EventLogger = golog.Logger("p2p:stream")

func HandshakeTo(rw *bufio.ReadWriter) (string, error) {
	_, err := rw.WriteString("REQUEST_PEER_ID>")
	if err != nil {
		logger.Error("Error writing to buffer:", err)
		return "", err
	}
	err = rw.Flush()
	if err != nil {
		logger.Error("Error flushing buffer:", err)
		return "", err
	}
	str, err := rw.ReadString('<')
	if err != nil {
		logger.Info("Error reading from buffer:", err)
		return "", err
	}
	_, err = rw.WriteString("RESPONSE_PEER_ID:ACK>")
	if err != nil {
		logger.Info("Error writing to buffer:", err)
		return "", err
	}
	err = rw.Flush()
	if err != nil {
		logger.Error("Error flushing buffer:", err)
		return "", err
	}
	if strings.Contains(str, "RESPONSE_PEER_ID:") {
		payload := strings.SplitAfter(str, ":")
		payload = strings.Split(payload[1], "<")
		str = payload[0][:]
	} else {
		logger.Errorf("Wrong handshake response %s", str)
		return "", fmt.Errorf("wrong handshake response %s", str)
	}
	logger.Infof("HandshakeTo with peer %s SUCCESS", str)
	return str, nil
}

func HandshakeFrom(rw *bufio.ReadWriter, peerID string) (string, error) {
	str, err := rw.ReadString('>')
	if err != nil {
		logger.Error("Error reading from buffer:", err)
		return "", err
	}
	if strings.Compare(str, "REQUEST_PEER_ID>") == 0 {
		_, err := rw.WriteString(fmt.Sprintf("RESPONSE_PEER_ID:%s<", peerID))
		if err != nil {
			logger.Error("Error writing to buffer:", err)
			return "", err
		}
		err = rw.Flush()
		if err != nil {
			logger.Error("Error flushing buffer:", err)
			return "", err
		}
		str, err = rw.ReadString('>')
		if err != nil {
			logger.Error("Error reading from buffer:", err)
			return "", err
		}
	}
	return str, nil
}
