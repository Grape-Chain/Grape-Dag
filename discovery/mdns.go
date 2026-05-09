package discovery

import (
	"github.com/VG-Grape/luna/utils"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	golog "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

var logger golog.EventLogger = golog.Logger("disc")

type discoveryNotifee struct {
	PeerChan chan peer.AddrInfo
}

// interface to be called when new  peer is found
func (n *discoveryNotifee) HandlePeerFound(pi peer.AddrInfo) {
	utils.ColorizeInfo(logger, "[mdns] Peer found: %s\n", pi.ID.String())
	n.PeerChan <- pi
}

// Initialize the MDNS service
func InitMDNS(peerhost host.Host, rendezvous string) (chan peer.AddrInfo, error) {
	// register with service so that we get notified about peer discovery
	n := &discoveryNotifee{}
	n.PeerChan = make(chan peer.AddrInfo)

	ser := mdns.NewMdnsService(peerhost, rendezvous, n)
	if err := ser.Start(); err != nil {
		logger.Error("Starting mDns service faild. ", err)
		close(n.PeerChan)
		return n.PeerChan, err
	}

	return n.PeerChan, nil
}
