package grapepeer

import (
	"reflect"
	"sync/atomic"
	"time"

	"github.com/enescakir/emoji"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/network"
)

type EvtHandler struct{}

func (e *EvtHandler) WaitForEvent(evt reflect.Type) {
	evtName := evt.Name()
	switch evtName {
	case reflect.TypeOf(event.EvtLocalProtocolsUpdated{}).Name():
		for {
			if seen_EvtLocalProtocolsUpdated.Load() {
				break
			}
			t := time.NewTimer(time.Second * 5)
			<-t.C
			t.Stop()
		}
	}
}

var seen_EvtLocalProtocolsUpdated atomic.Bool = atomic.Bool{}

func peerEvent(evtSub event.Subscription) {
	evtbus_monitor.Status.Store(true)
	defer logger.Infof("%s  ~ event bus monitor stopped", emoji.StopSign)
	defer evtSub.Close()
	defer evtbus_monitor.Status.Store(false)
	for !evtbus_monitor.Stop.Load() {
		select {
		case stat := <-evtSub.Out():
			switch stat.(type) {
			case event.AddrAction:
				logger.Infof("%s [HOST EVT] AddrAction", emoji.PartyPopper)
			case event.EvtPeerConnectednessChanged:
				evt := stat.(event.EvtPeerConnectednessChanged)
				logger.Infof("%s  [HOST EVT] %T -> %s", emoji.PartyPopper, evt, evt.Peer.String())
				switch evt.Connectedness {
				case network.NotConnected:
					logger.Infof("\t[-] %s NOT CONNECTED", evt.Peer.String())
				case network.Connected:
					logger.Infof("\t[+] %s CONNECTED", evt.Peer.String())
				case network.CanConnect:
					logger.Infof("\t[#] %s CAN CONNECT", evt.Peer.String())
				case network.CannotConnect:
					logger.Infof("\t[o] %s CANNOT CONNECT", evt.Peer.String())
				}
			case event.EvtLocalProtocolsUpdated:
				evt := stat.(event.EvtLocalProtocolsUpdated)
				for _, a := range evt.Added {
					logger.Infof("%s  [HOST EVT] %T [+]%s", emoji.PartyPopper, evt, a)
				}
				for _, r := range evt.Removed {
					logger.Infof("%s  [HOST EVT] %T [-]%s", emoji.PartyPopper, evt, r)
				}
				seen_EvtLocalProtocolsUpdated.Store(true)
			case event.EvtLocalAddressesUpdated:
				evt := stat.(event.EvtLocalAddressesUpdated)
				logger.Infof("%s  [HOST EVT] %T ->", emoji.PartyPopper, evt)
				if evt.Diffs {
					for i, a := range evt.Removed {
						logger.Infof("\t[-] %d. Removed address: %s", i, a.Address)
					}
					for i, a := range evt.Current {
						logger.Infof("\t[+] %d. Current address: %s", i, a.Address)
					}
				}
			default:
				logger.Infof("%s [HOST EVT] Unhandled %T -> %s", emoji.PartyPopper, stat, stat)
			}
		case <-evtbus_monitor.C:
			logger.Infof("%s  ~ Peer Event Bus monitor stopped", emoji.StopSign)
			return
		}
	}
}
