package discovery

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/ledongthuc/goterators"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	kbucket "github.com/libp2p/go-libp2p-kbucket"
	"github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/enescakir/emoji"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	"github.com/multiformats/go-multiaddr"
)

var my_peer host.Host = nil

func PeerDHT(host host.Host, bootstrapPeers []multiaddr.Multiaddr) *dht.IpfsDHT {
	// Connect to the bootstrap nodes. They will tell us about the
	// other nodes in the network via dht. If this is a new bootstrap node,
	// it will have an up to date dht table to serve to other nodes

	options := []dht.Option{
		dht.Mode(dht.ModeServer),
	}

	// Command line arguments override the config file with bootstrap nodes

	var bootstrap_list []multiaddr.Multiaddr
	if len(bootstrapPeers) > 0 {
		bootstrap_list = bootstrapPeers[:]
	} else {
		bootstrap_list = config.LoadBootstrap()
	}

	if len(bootstrap_list) > 0 {
		if err := connectBootstrapPeers(host, bootstrap_list, true); err != nil {
			logger.Warnf("[dht client] %s", err.Error())
		} else {
			// if successfully connected to bs, help DHT keep the accurate routing table
			for _, bs_ma := range bootstrap_list {
				bs_ai, _ := peer.AddrInfoFromP2pAddr(bs_ma)
				options = append(options, dht.BootstrapPeers(*bs_ai))
			}
		}
	}
	ctx := context.Background()
	kdht, err := dht.New(ctx, host, options...)
	if err != nil {
		logger.Error(err)
	}

	kdht.RoutingTable().PeerAdded = rt_PeerAdded
	kdht.RoutingTable().PeerRemoved = rt_PeerRemoved
	// _, err = kdht.RoutingTable().TryAddPeer(host.ID(), true, true)
	// if err != nil {
	// 	logger.Errorf("%s  ~ [O] Failed to add peer %s to DHT: %s", emoji.CrossMark, host.ID(), err.Error())
	// }
	n_peers := kdht.RoutingTable().NearestPeers(kbucket.ConvertPeerID(host.ID()), 5)
	logger.Infof("%s  ~ DHT as seen by %s", emoji.Ledger, host.ID())
	for i, n := range n_peers {
		logger.Infof("%s  ~ [%d] PEER %s -> DHT", emoji.Ledger, i, n.String())
	}
	// Bootstrap the DHT. In the default configuration, this spawns a Background
	// thread that will refresh the peer table every five minutes.
	logger.Debug("Bootstrapping the Client DHT")
	if err := kdht.Bootstrap(ctx); err != nil {
		panic(err)
	}

	return kdht
}

var (
	relayTemplate  string
	myHost         host.Host
	relayBootstrap peer.AddrInfo
	bootstrapReady atomic.Bool
)

func connectToBootstrap(myhost host.Host, peerAddr multiaddr.Multiaddr, wg *sync.WaitGroup, relays bool) {
	defer wg.Done()
	peerinfo, err := peer.AddrInfoFromP2pAddr(peerAddr)
	if err != nil {
		e := fmt.Errorf("Error parsing address %s. err: %s", peerAddr.String(), err.Error())
		logger.Error(e.Error())
		panic(e.Error())
	}
	remoteConnections.mx.Lock()
	_, ok := remoteConnections.connections[peerinfo.ID.String()]
	if ok {
		remoteConnections.mx.Unlock()
		logger.Infof("Already connected to %s.", peerinfo.String())
		return
	}
	remoteConnections.mx.Unlock()
	logger.Infof("[dht] Connecting to %s...", peerinfo.String())
	// Initial connection to the boostrap nodes
	for retries := 0; ; retries++ {
		con_ctx, con_csl := context.WithTimeout(context.Background(), time.Second*config.NET_CONN_TIMEOUT)
		err := myhost.Connect(con_ctx, *peerinfo)
		con_csl()
		if err != nil {
			logger.Warnf("[dht] Failed to connect to a boostrap node %s : %s", peerinfo.String(), err.Error())
		} else {
			break
		}
		time.Sleep(time.Second * 5)
		if retries == 10 {
			logger.Errorf("Failed to connect to a bootstrap node %s after %d retries. Giving up...", peerinfo.String(), retries)
			return
		}
	}
	// Make sure we do not double connect to another host as it may affect pubsub
	remoteConnections.mx.Lock()
	remoteConnections.connections[peerinfo.ID.String()] = append(remoteConnections.connections[peerinfo.ID.String()], peerAddr)
	remoteConnections.mx.Unlock()
	logger.Infof("[conn bootstrap] Connection established with bootstrap node: %s", peerinfo.String())

	if relays {
		logger.Infof("[conn bootstrap] Making relay reservations: %s ...", peerinfo.String())
		res, err := client.Reserve(context.Background(), myhost, *peerinfo)
		if err != nil {
			logger.Errorf("[conn bootstrap] Failed to make relay reservation: %s", err.Error())
		} else {
			for _, addr := range res.Addrs {
				logger.Infof("[conn bootstrap] Relay addr: %s", addr.String())
			}
			logger.Infof("[conn bootstrap] Success reserving relay circiut through: %s", res.Voucher.Relay.String())
			relayaddr, _ := multiaddr.NewMultiaddr("/p2p/" + peerinfo.ID.String() + "/p2p-circuit/p2p/" + myhost.ID().String())
			logger.Infof("[conn bootstrap] Our relay address: %s", relayaddr.String())
			relayTemplate = "/p2p/" + peerinfo.ID.String() + "/p2p-circuit/p2p/"
			relayBootstrap = peer.AddrInfo{
				ID:    peerinfo.ID,
				Addrs: []multiaddr.Multiaddr{relayaddr},
			}
			bootstrapReady.Store(true)
		}
	}
}

func connectBootstrapPeers(myhost host.Host, bootstrapPeers []multiaddr.Multiaddr, make_relay_reservations bool) error {
	var err error = nil
	// Connect to the bootstrap nodes. They will tell us about the
	// other nodes in the network via dht. If this is a new bootstrap node,
	// it will have an up to date dht table to serve to other nodes
	if myHost == nil {
		myHost = myhost
	}
	wg := sync.WaitGroup{}
	for _, peerAddr := range bootstrapPeers {
		wg.Add(1)
		logger.Infof("%s  ~ Connecting to bootstrap: %s ...", emoji.ElectricPlug, peerAddr.String())
		go connectToBootstrap(myhost, peerAddr, &wg, make_relay_reservations)
	}
	logger.Infof("%s  ~ Waiting for all bootstrap connections to complete...", emoji.HourglassNotDone)
	wg.Wait()
	return err
}

func compareMultiAddrs(addrs1 []multiaddr.Multiaddr, addrs2 []multiaddr.Multiaddr) bool {
	if len(addrs1) == len(addrs2) && len(addrs1) == 0 {
		return true
	}
	for _, addr1 := range addrs1 {
		_, _, err := goterators.Find(addrs2, func(addr2 multiaddr.Multiaddr) bool {
			return addr1.Equal(addr2)
		})
		if err != nil {
			return false
		}
	}
	return true
}

func connectRemoteHost(host host.Host, peerInfo peer.AddrInfo) error {
	var err error = nil
	con_ctx, con_csl := context.WithTimeout(context.Background(), time.Second*config.NET_CONN_TIMEOUT)
	defer con_csl()
	if err = host.Connect(con_ctx, peerInfo); err != nil {
		logger.Errorf("[dht peer discovery] Stream Handler [connect] connection establishment with %s failed: %v", peerInfo.String(), err)
		host.Network().(*swarm.Swarm).Backoff().Clear(peerInfo.ID)
		logger.Infof("Attempting to connect to hosts %s via relay...", peerInfo.ID)
		// otherwise, let's construct a relay address and try to connect
		relayID := relayBootstrap.ID
		if relayID == "" {
			staticRelayInfo, parseErr := peer.AddrInfoFromP2pAddr(multiaddr.StringCast(config.RELAY_SRV_1))
			if parseErr != nil {
				logger.Errorf("Failed to parse static relay address %s: %s", config.RELAY_SRV_1, parseErr.Error())
				return err
			}
			relayID = staticRelayInfo.ID
		}

		peer_relay_ma_str := fmt.Sprintf("/p2p/%s/p2p-circuit/p2p/%s", relayID, peerInfo.ID)
		peer_relay_addr, _ := multiaddr.NewMultiaddr(peer_relay_ma_str)
		logger.Infof("Peer %s relay address: %s", peerInfo.ID, peer_relay_addr)
		relayinfo := peer.AddrInfo{
			ID:    peerInfo.ID,
			Addrs: []multiaddr.Multiaddr{peer_relay_addr},
		}
		if err = host.Connect(con_ctx, relayinfo); err != nil {
			logger.Errorf("%s  Failed to connect via relay %s. %s", emoji.CrossMarkButton, relayinfo, err.Error())
		} else {
			logger.Infof("%s  Successfully connected via relay %s", emoji.CheckBoxWithCheck, relayinfo)
		}
	} else {
		logger.Infof("%s  Successfully connected to %s", emoji.CheckBoxWithCheck, peerInfo)
	}
	return err
}

func nsSpecificDiscovery(ctx context.Context, host host.Host, routingDiscovery discovery.Discovery, rendezvous string, options ...discovery.Option) {
	peerInfos, err := dutil.FindPeers(ctx, routingDiscovery, rendezvous, options...)
	if err != nil {
		logger.Errorf("%s  ~ %s Failed to discover peers. err: %s", emoji.CrossMarkButton, emoji.GlobeWithMeridians, err.Error())
		return
	}
	if len(peerInfos) == 0 {
		logger.Warnf("%s  ~ %s No peers found for topic: %s", emoji.Warning, emoji.GlobeWithMeridians, rendezvous)
		return
	}

	GetMesh().Add(rendezvous, peerInfos)

	var wg sync.WaitGroup
	for _, peerInfo := range peerInfos {
		// if its ourselves, do nothing
		if len(peerInfo.Addrs) == 0 || peerInfo.ID == host.ID() {
			continue
		}
		// we may have been connected already by the pubsub handlers
		connectedness := host.Network().Connectedness(peerInfo.ID)
		switch connectedness {
		case network.Connected:
			// already connected
			peer_discovered.Store(true)
			continue
		case network.CannotConnect:
			// no point trying to connect again
			continue
		case network.CanConnect:
		case network.NotConnected:
			logger.Infof("%s  ~ [DISCOVERY] T:%s New peer %s found", emoji.GlobeWithMeridians, rendezvous, peerInfo)
			wg.Add(1)
			go func(peerInfo peer.AddrInfo) {
				defer wg.Done()
				if err := connectRemoteHost(host, peerInfo); err != nil {
					logger.Errorf("%s  ~ [DISCOVERY] Connection to %s failed: %s", emoji.StopSign, peerInfo.ID, peerInfo)
				} else {
					peer_discovered.Store(true)
					logger.Infof("%s  ~ [DISCOVERY] Successfully connected to %s", emoji.CheckBoxWithCheck, peerInfo)
				}
			}(peerInfo)
		}
	}
	// wait for all the goroutines to process new connections
	wg.Wait()
}

func peerDiscovery(ctx context.Context, host host.Host, routingDiscovery discovery.Discovery, rendezvous []string, options ...discovery.Option) {
	logger.Debug("[dht peer discovery] -> Running NS Specific Discovery...")
	goterators.ForEach(rendezvous, func(topic string) {
		nsSpecificDiscovery(ctx, host, routingDiscovery, topic, options...)
	})
}

func HandleDht(
	ctx context.Context,
	host host.Host,
	kdht *dht.IpfsDHT,
	rendezvous []string) (chan bool, *drouting.RoutingDiscovery, *sync.WaitGroup) {
	my_peer = host

	ch := kdht.ForceRefresh()
	<-ch
	logger.Infof("%s  ~ DHT Refresh requested", emoji.Abacus)

	// start maintaining live connections
	done := make(chan bool, 2)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go ping_pong(ctx, kdht, wg, done)
	wg.Add(1)
	go peer_discovery(ctx, wg, done, host, kdht, rendezvous)
	routingDiscovery := drouting.NewRoutingDiscovery(kdht)
	return done, routingDiscovery, wg
}
