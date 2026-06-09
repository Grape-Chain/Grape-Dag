package discovery

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	host_config "github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/enescakir/emoji"
	"github.com/ledongthuc/goterators"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multiaddr"
)

type notifier struct {
	ProtocolID string
	Host       host.Host
}

var Notifiee notifier

// Listen is called when network starts listening on an addr
func (n *notifier) Listen(netw network.Network, ma multiaddr.Multiaddr) {
	logger.Infof("[Peer Notifier] [+] Listinging on %s", ma.String())
}

// ListenClose is called when network stops listening on an addr
func (n *notifier) ListenClose(netw network.Network, ma multiaddr.Multiaddr) {
	logger.Errorf("[Peer Notifier] [-] Stop listening on %s", ma.String())
}

// Connected is called when a connection opened
func (n *notifier) Connected(netw network.Network, conn network.Conn) {
	//retain max 6 connections
	if len(netw.Conns()) > host_config.MAX_CONN_LIMIT {
		conn.Close()
		logger.Errorf("[conn handler] Max connections. New connection refused for peer: %s", conn.RemoteMultiaddr().String())
	}
	peerID := conn.RemotePeer().String()
	NotifyConnected(conn.RemotePeer())

	logger.Infof("%s  [+] CONNECTED %s\n%s\n", emoji.ElectricPlug, peerID, conn.RemoteMultiaddr())
}

// Disconnected is called when a connection closed
func (cn *notifier) Disconnected(netw network.Network, conn network.Conn) {
	remotePeerID := conn.RemotePeer().String()
	logger.Infof("%s  [-] DISCONNECTED %s\n%s\n", emoji.ElectricPlug, remotePeerID, conn.RemoteMultiaddr())
	NotifyDisconnected(conn.RemotePeer())
	if my_peer != nil {
		my_peer.Peerstore().RemovePeer(conn.RemotePeer())
	}
	logger.Infof("[conn handler] [-] Disconnected from %s @ %s", remotePeerID, conn.RemoteMultiaddr().String())
	return
}

// OpenedStream is called when a stream opened
func (cn *notifier) OpenedStream(netw network.Network, s network.Stream) {
	logger.Infof("[conn handler] [+] Opened stream to %s", s.Conn().RemotePeer().String())
}

// ClosedStream is called when a stream was closed
func (cn *notifier) ClosedStream(netw network.Network, s network.Stream) {
	logger.Infof("[conn handler] [-] Stream closed to", s.Conn().RemotePeer().String())
}

type RWStreams struct {
	mux     sync.Mutex
	streams map[string]*bufio.ReadWriter
}

var rwstreams RWStreams = RWStreams{
	streams: make(map[string]*bufio.ReadWriter),
}

// We use this type to keep track of multiple connections from other peers
// especially when we need to deal with relaying connections
type RemoteConnections struct {
	mx          sync.Mutex
	connections map[string][]multiaddr.Multiaddr
}

// We keep track of host.Connect connections to allow different
// types of connections to this host - including relaying connections
var remoteConnections RemoteConnections = RemoteConnections{
	mx:          sync.Mutex{},
	connections: make(map[string][]multiaddr.Multiaddr),
}

func HandleStream(peerID string, stream network.Stream) {
	protID := stream.Protocol()
	remotePeerID := stream.Conn().RemotePeer().String()
	logger.Infof("[discovery] Protocol: %s Connection from: %s %s", stream.Protocol(),
		remotePeerID, stream.Conn().RemoteMultiaddr().String())
	if strings.Compare(string(protID), Notifiee.ProtocolID) != 0 {
		logger.Errorf("[discovery] Unsupported protocol %s", protID)
		return
	}
	// Create a buffer stream for non blocking read and write.
	rw := bufio.NewReadWriter(bufio.NewReader(stream), bufio.NewWriter(stream))

	// peerID, err := strm.HandshakeTo(rw)
	// if err != nil {
	// 	logger.Debug("Handshake on new connection: ", err)
	// }
	// logger.Debugf("[*] Remote peer ID: %s", peerID)

	remoteConnections.mx.Lock()
	addrs, ok := remoteConnections.connections[remotePeerID]
	allowNewStream := true
	if ok {
		// compare addresses
		// if there is no match allow a new stream connection
		allowNewStream = !compareMultiAddrs(addrs, []multiaddr.Multiaddr{stream.Conn().RemoteMultiaddr()})
	}
	remoteConnections.mx.Unlock()
	if allowNewStream {
		remoteConnections.mx.Lock()
		remoteConnections.connections[remotePeerID] = append(remoteConnections.connections[remotePeerID], stream.Conn().RemoteMultiaddr())
		remoteConnections.mx.Unlock()
		logger.Debugf("[discovery] This is a new connection request from %s. Connected!\n", peerID)
		rwstreams.streams[peerID] = rw
		// Extract the peer ID from the multiaddr.
		raddr := stream.Conn().RemoteMultiaddr()
		logger.Debugf("Remote multiaddress for peer %s is %v", peerID, raddr)
		maddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", raddr.String(), peerID))
		if err != nil {
			logger.Error(err)
			return
		}
		info, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			logger.Error(err)
			return
		}
		// We have a peer ID and a targetAddr so we add it to the peerstore
		// so LibP2P knows how to contact it
		Notifiee.Host.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.ConnectedAddrTTL)
		logger.Infof("[discovery] Added to peerstore ID:%s, Addr:%s", info.ID.String(), stream.Conn().RemoteMultiaddr().String())
	} else {
		logger.Warnf("Connection with peer %s has already been established. Skipping the connect request.\n", peerID)
	}
}

func HandlemDns(ctx context.Context, host host.Host, peerChan chan peer.AddrInfo) {
	logger.Info("[mdns] Handle mDns discovery")
	for peerInfo := range peerChan {
		peerID := peerInfo.ID.String()

		// Connect to the newly discovered host
		remoteConnections.mx.Lock()
		_, ok := remoteConnections.connections[peerID]
		remoteConnections.mx.Unlock()
		if ok {
			continue
		}
		goterators.ForEach(peerInfo.Addrs, func(addr multiaddr.Multiaddr) {
			utils.ColorizeInfo(logger, "[mdns] => Found peer %s @ %s", peerID, addr.String())
		})
		con_ctx, con_csl := context.WithTimeout(context.Background(), time.Second*15)
		if err := host.Connect(con_ctx, peerInfo); err != nil {
			logger.Errorf("[mdns] Connection to %s failed: %s", peerInfo.String(), err.Error())
			con_csl()
			continue
		}
		con_csl()
		utils.ColorizeInfo(logger, "[mdns] => Connected to %s @ %s", peerID, peerInfo.String())

		remoteConnections.mx.Lock()
		remoteConnections.connections[peerInfo.ID.String()] = append(remoteConnections.connections[peerInfo.ID.String()], peerInfo.Addrs...)
		remoteConnections.mx.Unlock()
	}
}

func rt_PeerAdded(id peer.ID) {
	logger.Infof("%s  ~ [+] PEER [%s] ADDED -> DHT", emoji.Laptop, id.String())
}

func rt_PeerRemoved(id peer.ID) {
	logger.Infof("%s  ~ [+] PEER [%s] REMOVED -> DHT", emoji.Laptop, id.String())
}
