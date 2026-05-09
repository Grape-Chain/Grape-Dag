package lunapeer

import (
	"time"

	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
)

type PeerHolepunchTracer struct {
}

func NewPeerHolepunchTracer() *PeerHolepunchTracer {
	return &PeerHolepunchTracer{}
}

func (tr *PeerHolepunchTracer) Trace(evt *holepunch.Event) {
	logger.Infof("[H->P] Holepunch: %s %s->%s %s", time.Unix(evt.Timestamp, 0).Local().String(), evt.Peer.String(), evt.Remote.String(), evt.Type)
}
