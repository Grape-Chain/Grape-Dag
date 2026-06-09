package network

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	cfg "github.com/Grape-Chain/Grape-Dag/config"
	grapepeer "github.com/Grape-Chain/Grape-Dag/peer"
	"github.com/Grape-Chain/Grape-Dag/utils"
	golog "github.com/ipfs/go-log/v2"
	"github.com/ledongthuc/goterators"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/host"
	nat "github.com/libp2p/go-nat"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

var (
	logger golog.EventLogger
)

func init() {
	logger = golog.Logger("netw")
}

var (
	maddr_options = []string{
		"/%s/%s/udp/%d/quic",
		"/%s/%s/tcp/%d",
		"/%s/%s/tcp/%d/ws",
	}
)

func ListNetworkInterfaces(h host.Host, port int, home bool) {
	// Enumerate all network interfaces this host can listen on
	ifaceAddrs, _ := h.Network().InterfaceListenAddresses()
	// First, let's find all interesting private addresses
	pAddrs := multiaddr.FilterAddrs(ifaceAddrs, manet.IsPrivateAddr)
	r, _ := regexp.Compile(cfg.ADDR_REGEXP)
	for _, privAddr := range pAddrs {
		// We are skipping over 127. and 172. addresses
		if r.MatchString(privAddr.String()) {
			continue
		}
		utils.ColorizeInfo(logger, "\nHost ID: %s/p2p/%s\n", privAddr.String(), h.ID().String())
		// [Private] /ip4/198.51.100.10/udp/33331/quic/p2p/QmS1L76o6q2CVG2AAtRC2PgSS4jznQce7k5ZtZT2LMbq89
		utils.ColorizeInfo(logger, "[Private] %s", privAddr.String())
		// if cfg.Bootstrap {
		// }
	}
	// Second, let's see if we have any public addresses assigned to us
	pubAddrs := multiaddr.FilterAddrs(ifaceAddrs, manet.IsPublicAddr)
	for _, pubAddr := range pubAddrs {
		utils.ColorizeInfo(logger, "\nHost ID: %s/p2p/%s\n", pubAddr.String(), h.ID().String())
		utils.ColorizeInfo(logger, "[Public] %s", pubAddr.String())
	}
	// utils.ColorizeInfo(logger, "Preparing the network interfaces/addresses...")
	// if home {
	// 	addrs, err := PrepareExternalAddresses(port)
	// 	if err != nil {
	// 		utils.ColorizePrint("Cannot find suitable gateway device. err: %s", err.Error())
	// 	} else {
	// 		for id, pm := range addrs {
	// 			utils.ColorizePrint("Port Forwarding Rule [%s] %s:%d -> %s:%d",
	// 				id, pm.iaddr.String(), pm.iport, pm.eaddr.String(), pm.eport,
	// 			)
	// 			var ipv string
	// 			if pm.iaddr.To4() == nil {
	// 				ipv = "ip4"
	// 			} else {
	// 				ipv = "ip6"
	// 			}
	// 			for _, ma := range maddr_options {
	// 				conn_str := fmt.Sprintf(ma, ipv, pm.eaddr.String(), pm.eport)
	// 				utils.ColorizePrint("\nHost ID: %s/%s\n", conn_str, h.ID().Pretty())
	// 				utils.ColorizeInfo(logger, "[Public] %s/%s", conn_str, h.ID().Pretty())
	// 			}
	// 		}
	// 	}
	// }
}

func prepareLocalAddresses(port int) ([]string, []int, error) {
	addrs := []string{}
	ports := []int{}
	ifaces, e := net.InterfaceAddrs()
	if e != nil {
		return addrs, ports, e
	}
	r, _ := regexp.Compile(cfg.ADDR_REGEXP)
	for _, i := range ifaces {
		addr, _, _ := net.ParseCIDR(i.String())

		if r.MatchString(addr.String()) {
			continue
		}

		utils.ColorizeInfo(logger, "[Private] %s:%d", addr, port)
		addrs = append(addrs, addr.String())
		ports = append(ports, port)
	}

	// Enumerate all network interfaces this host can listen on
	// ifaceAddrs, _ := h.Network().InterfaceListenAddresses()
	// // First, let's find all interesting private addresses
	// pAddrs := multiaddr.FilterAddrs(ifaceAddrs, manet.IsPrivateAddr)
	// r, _ := regexp.Compile(grapepeer.ADDR_REGEXP)
	// for _, privAddr := range pAddrs {
	// 	// We are skipping over 127. and 172. addresses
	// 	if r.MatchString(privAddr.String()) {
	// 		continue
	// 	}
	// 	// [Private] /ip4/198.51.100.10/udp/33331/quic/p2p/QmS1L76o6q2CVG2AAtRC2PgSS4jznQce7k5ZtZT2LMbq89
	// 	utils.ColorizeInfo(logger, "[Private] %s", privAddr.String())
	// 	options = append(options, libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/udp/%d/quic", privAddr.String(), port)))
	// 	options = append(options, libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/tcp/%d", privAddr.String(), port)))
	// 	options = append(options, libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/tcp/%d/ws", privAddr.String(), port)))
	// 	// if cfg.Bootstrap {
	// 	// 	fmt.Printf("\nHost ID: %s/p2p/%s\n", privAddr.String(), grapepeer.GetHost().ID().Pretty())
	// 	// }
	// }
	// // Second, let's see if we have any public addresses assigned to us
	// pubAddrs := multiaddr.FilterAddrs(ifaceAddrs, manet.IsPublicAddr)
	// for _, pubAddr := range pubAddrs {
	// 	utils.ColorizeInfo(logger, "[Public] %s", pubAddr.String())
	// 	options = append(options, libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/udp/%d/quic", pubAddr.String(), port)))
	// 	options = append(options, libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/tcp/%d", pubAddr.String(), port)))
	// 	options = append(options, libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/tcp/%d/ws", pubAddr.String(), port)))
	// }
	return addrs, ports, nil
}

type NatPortMappingNotifee func(pmr *PortMapping)

type PortMapping struct {
	iaddr net.IP
	iport int
	eaddr net.IP
	eport int
	prot  string
}

func (pm *PortMapping) String() string {
	return fmt.Sprintf("[%s] %s:%d -> %s:%d", pm.prot, pm.iaddr.String(), pm.iport, pm.eaddr.String(), pm.eport)
}

var (
	myNat         nat.NAT        = nil
	pm_mu         sync.Mutex     = sync.Mutex{}
	port_mappings                = make(map[string]PortMapping)
	stop_mappings atomic.Bool    = atomic.Bool{}
	wg            sync.WaitGroup = sync.WaitGroup{}
	notifees      []NatPortMappingNotifee
	notifees_mu   sync.Mutex = sync.Mutex{}
)

func Init() {
	// After the host has been created, list its interfaces
	ListNetworkInterfaces(grapepeer.GetHost(), cfg.GetConfig().Host.Port, cfg.GetConfig().Host.Home)

	if cfg.GetConfig().Host.Home {
		// There might be a dependency on host being created first before we start port mappings
		// if upnp is requested
		AddPortMappingNotifee(portMappingAdded)
		NatPortMappings(cfg.GetConfig().Host.Port)
	}
}

func AddPortMappingNotifee(n NatPortMappingNotifee) {
	notifees_mu.Lock()
	defer notifees_mu.Unlock()
	notifees = append(notifees, n)
}

func notifyAddPortMapping(pm *PortMapping) {
	notifees_mu.Lock()
	defer notifees_mu.Unlock()
	goterators.ForEach(notifees, func(f NatPortMappingNotifee) {
		f(pm)
	})
}

func TerminatePortMappings() {
	logger.Info("Terminating port mappings")
	stop_mappings.Store(true)
	wg.Wait()
}

func natPortMappings(port int) {
	chNATs := nat.DiscoverNATs(context.Background())
	for {
		myNAT := <-chNATs
		go func(myNat nat.NAT) {
			if myNat == nil {
				return
			}
			logger.Infof("[UPnP] [+] Discovered UPnP nat type: %s", myNat.Type())

			daddr, err := myNat.GetDeviceAddress()
			if err != nil {
				logger.Errorf("[UPnP] [E] Get device address: %s", err.Error())
				return
			}
			logger.Infof("[UPnP] [+] NAT device address: %s", daddr.String())

			iaddr, err := myNat.GetInternalAddress()
			if err != nil {
				logger.Errorf("[UPnP] [E] Get internal address: %s", err.Error())
				return
			}
			logger.Infof("[UPnP] NAT device internal address: %s", iaddr.String())

			eaddr, err := myNat.GetExternalAddress()
			if err != nil {
				logger.Errorf("[UPnP] [E] Get external address: %s", err.Error())
				return
			}
			logger.Infof("[UPnP] NAT device external address: %s", eaddr.String())

			id := fmt.Sprintf("grapeone-tcp-%s:%d", iaddr.String(), port)
			eport, err := myNat.AddPortMapping(context.Background(), "tcp", port, id, time.Second*300)
			if err != nil {
				logger.Errorf("[UPnP] [E] Get device address: %s", err.Error())
				return
			}
			logger.Infof("[UPnP] [+] [TCP] %s:%d => %s:%d", iaddr.String(), port, eaddr.String(), eport)
			rule_tcp := PortMapping{
				iaddr: iaddr,
				iport: port,
				eaddr: eaddr,
				eport: eport,
				prot:  "tcp",
			}
			pm_mu.Lock()
			port_mappings[id] = rule_tcp
			pm_mu.Unlock()
			notifyAddPortMapping(&rule_tcp)
			id = fmt.Sprintf("grapeone-udp-%s:%d", iaddr.String(), port)
			eport, err = myNat.AddPortMapping(context.Background(), "udp", port, id, time.Second*60)
			if err != nil {
				logger.Errorf("[UPnP] [E] Add port mapping: %s", err.Error())
				return
			}
			logger.Infof("[UPnP] [+] [UDP] %s:%d => %s:%d", iaddr.String(), port, eaddr.String(), eport)
			rule_udp := PortMapping{
				iaddr: iaddr,
				iport: port,
				eaddr: eaddr,
				eport: eport,
				prot:  "udp",
			}
			pm_mu.Lock()
			port_mappings[id] = rule_udp
			pm_mu.Unlock()
			notifyAddPortMapping(&rule_udp)
		}(myNAT)
	}
}

func prepareUPnPAddresses(port int) (map[string]PortMapping, error) {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	logger.Infof("Discover network gateways...[may take time]")

	myNat, err = nat.DiscoverGateway(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	cancel()
	logger.Infof("[UPnP] [+] Discovered UPnP nat type: %s", myNat.Type())

	daddr, err := myNat.GetDeviceAddress()
	if err != nil {
		return nil, err
	}
	logger.Infof("[UPnP] [+] NAT device address: %s", daddr.String())

	iaddr, err := myNat.GetInternalAddress()
	if err != nil {
		return nil, err
	}
	logger.Infof("[UPnP] NAT device internal address: %s", iaddr.String())

	eaddr, err := myNat.GetExternalAddress()
	if err != nil {
		return nil, err
	}
	logger.Infof("[UPnP] NAT device external address: %s", eaddr.String())
	id := fmt.Sprintf("grapeone-tcp-%d", port)
	eport, err := myNat.AddPortMapping(context.Background(), "tcp", port, id, time.Second*300)
	if err != nil {
		return nil, err
	}
	logger.Infof("[UPnP] [+] [TCP] %s:%d => %s:%d", iaddr.String(), eport, eaddr.String(), eport)
	port_mappings[id] = PortMapping{
		iaddr: iaddr,
		iport: port,
		eaddr: eaddr,
		eport: eport,
		prot:  "tcp",
	}
	id = fmt.Sprintf("grapeone-udp-%d", port)
	eport, err = myNat.AddPortMapping(context.Background(), "udp", port, id, time.Second*60)
	if err != nil {
		return nil, err
	}
	logger.Infof("[UPnP] [+] [UDP] %s:%d => %s:%d", iaddr.String(), eport, eaddr.String(), eport)
	port_mappings[id] = PortMapping{
		iaddr: iaddr,
		iport: port,
		eaddr: eaddr,
		eport: eport,
		prot:  "udp",
	}

	// Keep the port mappings alive - as NAT device will not keep them forever for us
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(time.Second * 15)
		for !stop_mappings.Load() {
			<-t.C
			// @TODO: Re-discovery device gateway here....

			for id, pm := range port_mappings {
				eport, err := myNat.AddPortMapping(context.Background(), pm.prot, pm.iport, id, time.Second*300)
				if err != nil {
					logger.Errorf("Refresh port mapping %s:%d -> %s:%d error %s",
						pm.iaddr.String(), pm.iport, pm.eaddr.String(), eport, err.Error(),
					)
				} else {
					if eport != pm.eport {
						logger.Warnf("[/] Rule: %s Port mapped %s:%d -> %s:%d to %s:%d",
							pm.iaddr.String(), pm.iport, pm.eaddr.String(), pm.eport, pm.eaddr.String(), eport,
						)
						npm := port_mappings[id]
						npm.eport = eport
						port_mappings[id] = npm
					}
				}
			}
		}
		t.Stop()
		for _, pm := range port_mappings {
			logger.Infof("[UPnP] [-] Delete port mapping: %s:%d => %s:%d", pm.iaddr.String(), pm.iport, pm.eaddr.String(), pm.eport)
			myNat.DeletePortMapping(context.Background(), pm.prot, pm.iport)
		}
	}()

	return port_mappings, nil
}

func prepareAddrOptions(addrs []string, ports []int) ([]config.Option, error) {
	options := []config.Option{}
	var nt string
	for i, a := range addrs {
		if net.ParseIP(a).To4() != nil {
			nt = "ip4"
		} else {
			nt = "ip6"
		}
		for _, ma := range maddr_options {
			las := fmt.Sprintf(ma, nt, a, ports[i])
			utils.ColorizeInfo(logger, "[Multiaddress] %s", las)
			options = append(options, libp2p.ListenAddrStrings(las))
		}
	}
	return options, nil
}

type ExternalAddress struct {
	Addr string
	Port int
}

func PrepareExternalAddresses(port int) (map[string]PortMapping, error) {
	uaddrs, err := prepareUPnPAddresses(port)
	if err != nil || uaddrs == nil {
		// this error may be expected
		logger.Warnf("[UPnP] Port mappings err: %s", err.Error())
	}

	return uaddrs, err
}

func NatPortMappings(port int) {
	go natPortMappings(port)
}
