package lunapeer

import (
	"context"

	"github.com/enescakir/emoji"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	ma "github.com/multiformats/go-multiaddr"
)

func makeRelayReservation(host host.Host, conn_str string) (*client.Reservation, error) {
	var res *client.Reservation = nil
	relayHost_info, err := peer.AddrInfoFromString(conn_str)
	if err != nil {
		logger.Errorf("Error parsing relay p2p address: %s", err.Error())
		return nil, err
	}
	logger.Infof("%s  MAKE RELAY RESERVATION %s", emoji.Ledger, relayHost_info)
	logger.Infof("%s  Connecting to RELAY %s", emoji.ElectricPlug, relayHost_info.ID)
	if err := host.Connect(context.Background(), *relayHost_info); err != nil {
		logger.Errorf("%s  Failed to connect to %s %s", emoji.StopSign, relayHost_info.ID, err.Error())
		return nil, err
	} else {
		logger.Infof("%s  Successfully connected to relay %s. Reserving...", emoji.CheckBoxWithCheck, relayHost_info.ID)
		res, err = client.Reserve(context.Background(), host, *relayHost_info)
		if err != nil {
			logger.Errorf("%s Failed to make a reservation with %s %s", emoji.StopSign, relayHost_info.ID, err.Error())
			return nil, err
		} else {
			logger.Infof("%s [+]SUCCESSFULY RELAY RESERVATION %s", emoji.CheckBoxWithCheck, relayHost_info)
			host_relayaddr, _ := ma.NewMultiaddr("/p2p/" + relayHost_info.ID.String() + "/p2p-circuit/p2p/" + host.ID().String())
			logger.Infof("%s  Our relay address: %s", emoji.Laptop, host_relayaddr)
			host.Peerstore().SetAddr(host.ID(), host_relayaddr, 1<<63-1)
		}
	}
	return res, nil
}
