package lunapeer

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	host_config "github.com/VG-Grape/luna/config"
	discovery "github.com/VG-Grape/luna/discovery"
	utils "github.com/VG-Grape/luna/utils"
	"github.com/enescakir/emoji"
	golog "github.com/ipfs/go-log/v2"
	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	config "github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/metrics"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	"github.com/libp2p/go-libp2p/p2p/protocol/identify"
	tls "github.com/libp2p/go-libp2p/p2p/security/tls"
	"github.com/multiformats/go-multiaddr"
)

var (
	idht        *dht.IpfsDHT
	idht_ready  chan bool         = make(chan bool, 1)
	logger      golog.EventLogger = golog.Logger("host")
	hpService   *holepunch.Service
	addrObsrvCh chan bool
	addrObsrvWg sync.WaitGroup
)

type LunaHost struct {
	host            host.Host
	bandwidthTicker *time.Ticker
	bandwidthDone   chan bool
	relay           *relayv2.Relay
	relayInfo       peer.AddrInfo
}

var lunaHost *LunaHost

type RoutineMgr struct {
	Name   string
	Stop   atomic.Bool
	Status atomic.Bool
	WG     *sync.WaitGroup
	C      chan bool
}

var evtbus_monitor *RoutineMgr = &RoutineMgr{
	Name:   "event bus",
	Stop:   atomic.Bool{},
	Status: atomic.Bool{},
	WG:     &sync.WaitGroup{},
	C:      make(chan bool),
}

func getRelayCandidates(cfg *host_config.HostConfig) []string {
	seen := map[string]struct{}{}
	candidates := make([]string, 0)

	for _, relay := range cfg.Bootstrap_peers {
		relayAddr := relay.String()
		if len(relayAddr) == 0 {
			continue
		}
		if _, ok := seen[relayAddr]; ok {
			continue
		}
		seen[relayAddr] = struct{}{}
		candidates = append(candidates, relayAddr)
	}

	for _, relay := range []string{host_config.RELAY_SRV_1, host_config.RELAY_SRV_2} {
		if _, ok := seen[relay]; ok {
			continue
		}
		seen[relay] = struct{}{}
		candidates = append(candidates, relay)
	}

	return candidates
}

func GetHost() host.Host {
	if lunaHost != nil {
		return lunaHost.host
	}
	return nil
}
func SetIDHT(dht *dht.IpfsDHT) {
	idht = dht
}

func GetIDHT() *dht.IpfsDHT {
	return idht
}

func WaitDHTReady() {
	<-idht_ready
}

func handleRouting(h host.Host) (routing.PeerRouting, error) {
	options := []dht.Option{
		dht.Mode(dht.ModeAuto),
	}
	kdht, err := dht.New(context.Background(), h, options...)
	if err != nil {
		logger.Error(err)
	} else {
		idht = kdht
	}
	idht_ready <- true
	return idht, nil
}

func handleAddr(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	var filtered_addrs []multiaddr.Multiaddr
	r, _ := regexp.Compile(host_config.ADDR_REGEXP)
	for _, addr := range addrs {
		if r.MatchString(addr.String()) {
			continue
		}
		filtered_addrs = append(filtered_addrs, addr)
	}
	return filtered_addrs
}

func prepareHostOptions(prvKey crypto.PrivKey, cfg *host_config.HostConfig, lunaCfg *host_config.Lunapeer) []config.Option {
	if lunaCfg.Peer.Port == 0 {
		utils.ColorizeWarn(logger, "Host will choose an ephemeral port to communicate on.")
	}

	optionsHolePunch := []holepunch.Option{}
	// make sure we have the ability override the config port when launching multiple instances
	// of peer node on the same host
	var port int
	if cfg.Port > 0 {
		port = cfg.Port
	} else {
		port = lunaCfg.Peer.Port
	}

	options := []config.Option{
		libp2p.Identity(prvKey),
		libp2p.Ping(true),
		libp2p.EnableHolePunching(optionsHolePunch...),
	}

	id := fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", lunaCfg.Peer.Host, port)
	logger.Infof("[Host] Listen address option: %s", id)
	options = append(options, libp2p.ListenAddrStrings(id))
	id = fmt.Sprintf("/ip4/%s/tcp/%d", lunaCfg.Peer.Host, port)
	logger.Infof("[Host] Listen address option: %s", id)
	options = append(options, libp2p.ListenAddrStrings(id))
	id = fmt.Sprintf("/ip4/%s/tcp/%d/ws", lunaCfg.Peer.Host, port)
	logger.Infof("[Host] Listen address option: %s", id)
	options = append(options, libp2p.ListenAddrStrings(id))

	// muxers := libp2p.ChainOptions(
	// 	libp2p.Muxer("/yamux/1.0.0", yamux.DefaultTransport),
	// 	libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
	// )

	// options = append(options, muxers)

	options = append(options, libp2p.Security(tls.ID, tls.New))
	options = append(options, libp2p.EnableRelay())
	// options = append(options, libp2p.NATPortMap())
	// options = append(options, libp2p.AddrsFactory(handleAddr))
	//options = append(options, libp2p.ForceReachabilityPublic())
	if len(cfg.Bootstrap_peers) == 0 {
		cfg.Bootstrap_peers = host_config.LoadBootstrap()
	}
	var static_relays []peer.AddrInfo
	for _, relay := range getRelayCandidates(cfg) {
		if v, err := peer.AddrInfoFromString(relay); err == nil {
			static_relays = append(static_relays, *v)
		} else {
			logger.Warnf("Invalid address %s. Skipping...", relay)
		}
	}
	options = append(options, libp2p.EnableAutoRelay(autorelay.WithStaticRelays(static_relays)))
	// Ok: disable dht routing. It could be a double call
	// options = append(options, libp2p.Routing(handleRouting))
	return options
}

func reportBandwidth(reporter *metrics.BandwidthCounter) (*time.Ticker, chan bool) {
	ticker := time.NewTicker(5 * time.Minute)
	done := make(chan bool, 1)
	go func() {
		stop := false
		for !stop {
			select {
			case <-ticker.C:
				stats := reporter.GetBandwidthTotals()
				utils.ColorizeInfo(logger,
					"[Bandwidth]: Rate In:%.3fB/s, Out:%.3fB/s, Total In:%dB, Out:%dB",
					stats.RateIn, stats.RateOut, stats.TotalIn, stats.TotalOut)
			case <-done:
				logger.Info("Bandwidth reporter is stopping...")
				stop = true
			}
		}
		logger.Info("Bandwidth reporter has stoppped")
	}()
	return ticker, done
}

func NewHost(prvKey crypto.PrivKey, cfg *host_config.HostConfig, lunaCfg *host_config.Lunapeer) error {
	// 0.0.0.0 will listen on any interface device.
	// libp2p.New constructs a new libp2p Host.
	// Other options can be added here.
	lunaHost = new(LunaHost)
	options := prepareHostOptions(prvKey, cfg, lunaCfg)
	var reporter *metrics.BandwidthCounter = nil
	if lunaCfg.Peer.Bandwidth > 0 {
		reporter = metrics.NewBandwidthCounter()
		options = append(options, libp2p.BandwidthReporter(reporter))
		lunaHost.bandwidthTicker, lunaHost.bandwidthDone = reportBandwidth(reporter)
	}

	host, err := libp2p.New(options...)
	if err != nil {
		logger.Error(err)
		return err
	}
	host.Peerstore().AddPrivKey(host.ID(), prvKey)
	host.Peerstore().AddPubKey(host.ID(), prvKey.GetPublic())

	if !cfg.Bootstrap {
		for _, relay := range getRelayCandidates(cfg) {
			if _, err := makeRelayReservation(host, relay); err == nil {
				break
			}
		}
	}

	idService, err := identify.NewIDService(host)
	if err != nil {
		logger.Warnf("Creating an identity sevice err: %s. Will continue...", err.Error())
	}

	// // Launch address observer
	// addrObsrvCh = make(chan bool)
	// addrObsrvWg = sync.WaitGroup{}
	// addrObsrvWg.Add(1)
	// stop := false
	// go func() {
	// 	defer addrObsrvWg.Done()
	// 	t := time.NewTicker(time.Second * 30)
	// 	for !stop {
	// 		select {
	// 		case <-addrObsrvCh:
	// 			stop = true
	// 		case <-t.C:
	// 			ma := idService.OwnObservedAddrs()
	// 			for _, a := range ma {
	// 				oma := idService.ObservedAddrsFor(a)
	// 				for _, oa := range oma {
	// 					logger.Infof("\t[%s] Observed address: %s", time.Now().Local().String(), oa.String())
	// 				}
	// 			}
	// 		}
	// 	}
	// 	t.Stop()
	// }()

	hpOptions := []holepunch.Option{holepunch.WithTracer(NewPeerHolepunchTracer())}
	hpService, err = holepunch.NewService(host, idService, host.Addrs, hpOptions...)
	if err != nil {
		logger.Warnf("Launching a holepunching sevice err: %s. Will continue...", err.Error())
	}

	host.ConnManager().TagPeer(host.ID(), lunaCfg.Peer.Id, 5)
	if cfg.Bootstrap {
		lunaHost.relay, err = relayv2.New(host, relayv2.WithLimit(nil))
		if err != nil {
			logger.Errorf("[RELAY] Failed to enable a new relay service: %v+", err)
		}
		lunaHost.relayInfo = peer.AddrInfo{
			ID:    host.ID(),
			Addrs: host.Addrs(),
		}
		logger.Info("/p2p/" + host.ID().String() + "/p2p-circuit/p2p/")
	}
	discovery.Notifiee.ProtocolID = cfg.ProtocolID
	discovery.Notifiee.Host = host
	host.Network().Notify(&discovery.Notifiee)

	cSub, err := host.EventBus().Subscribe(event.WildcardSubscription)
	if err != nil {
		logger.Errorf("%s  ~ [HOST EVT] %s", emoji.Notebook, err.Error())
	} else {
		go peerEvent(cSub)
	}

	// Set a function as stream handler.
	// This function is called when a peer initiates a connection and starts a stream with this peer.
	// Ok: Sat 5 - disable stream creation. Experiment
	//host.SetStreamHandler(protocol.ID(cfg.ProtocolID), discovery.HandleStream)
	// an_options := []autonat.Option{}
	// lunaHost.dialback, err = libp2p.New(libp2p.NoListenAddrs)
	// if err != nil {
	// 	logger.Errorf("Dialback host error %s", err.Error())
	// } else {
	// 	an_options = append(an_options, autonat.EnableService(lunaHost.dialback.Network()))
	// 	an_nat, err := autonat.New(host, an_options...)
	// 	if err != nil {
	// 		logger.Errorf("[AutoNAT] Failed to create a NAT service: %s", err.Error())
	// 	} else {
	// 		nat_monitor.WG.Add(1)
	// 		go func() {
	// 			logger.Infof("%s  ~ [AutoNAT] service is running %s ->", emoji.Laptop, emoji.PersonRunning)
	// 			defer nat_monitor.WG.Done()
	// 			t := time.NewTicker(time.Second * 10)
	// 			defer t.Stop()
	// 			defer an_nat.Close()
	// 			nat_monitor.Status.Store(true)
	// 			status := an_nat.Status()
	// 			logger.Infof("%s  ~ [AutoNAT] Network reachability %s", emoji.Laptop, status)
	// 			for !nat_monitor.Stop.Load() {
	// 				select {
	// 				case <-nat_monitor.C:
	// 					nat_monitor.Status.Store(false)
	// 					logger.Info("%s  ~ [AutoNAT] service stopped", emoji.StopSign)
	// 					return
	// 				case <-t.C:
	// 					new_status := an_nat.Status()
	// 					if new_status != status {
	// 						logger.Infof("%s  ~ [AutoNAT] Network reachability changed from % to %s", emoji.Laptop, status, new_status)
	// 						status = new_status
	// 						ma, err := an_nat.PublicAddr()
	// 						if err == nil {
	// 							logger.Infof("%s  ~ [AutoNAT] Public IP: %s", emoji.GlobeShowingEuropeAfrica, ma)
	// 						}
	// 					}
	// 				}
	// 			}
	// 			nat_monitor.Status.Store(false)
	// 			logger.Info("%s  ~ AutoNAT service stopped", emoji.StopSign)
	// 		}()
	// 	}
	// }
	lunaHost.host = host
	return nil
}

func Terminate() {
	if lunaHost.bandwidthTicker != nil {
		logger.Info("Stopping bandwidth estimation timers/tickers")
		lunaHost.bandwidthDone <- true
		close(lunaHost.bandwidthDone)
		lunaHost.bandwidthTicker.Stop()
	}
	// stopping the autonat monitor
	if evtbus_monitor.Status.Load() {
		logger.Info("Stopping event bus monitor")
		evtbus_monitor.Stop.Store(true)
		evtbus_monitor.C <- true
		logger.Infof("%s  ~ Waiting for event bus monitor to stop", emoji.HourglassNotDone)
		evtbus_monitor.WG.Wait()
		close(evtbus_monitor.C)
	}
	// // stopping the address observer routine
	// logger.Info("Stopping the network address observer")
	// addrObsrvCh <- true
	// addrObsrvWg.Wait()
	// close(addrObsrvCh)
	// logger.Info("The network address observer has been successfully stopped.")
	//
	if lunaHost.relay != nil {
		logger.Info("Stopping relay")
		lunaHost.relay.Close()
	}
	if hpService != nil {
		logger.Info("Stopping holepunching service")
		hpService.Close()
	}
	logger.Info("Stopping communication manager")
	lunaHost.host.ConnManager().Close()
	logger.Infof("Stopping "+host_config.APP_NAME+" %s", lunaHost.host.ID().String())

	logger.Info("Purging peer store")
	peers := lunaHost.host.Peerstore().Peers()
	for _, p := range peers {
		logger.Infof("Removing peer %s", p.String())
		lunaHost.host.Peerstore().ClearAddrs(p)
		lunaHost.host.Peerstore().RemovePeer(p)
	}
	logger.Infof("Terminating peer %s", lunaHost.host.ID().String())
	lunaHost.host.Close()
	logger.Infof(host_config.APP_NAME+" %s terminated.", lunaHost.host.ID().String())
}
