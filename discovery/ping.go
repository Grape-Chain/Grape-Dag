package discovery

import (
	"context"
	"sync"
	"time"

	"github.com/enescakir/emoji"
	dht "github.com/libp2p/go-libp2p-kad-dht"
)

func ping_pong(ctx context.Context, kdht *dht.IpfsDHT, wg *sync.WaitGroup, done <-chan bool) {
	defer wg.Done()
	logger.Infof("%s  ~ PingPong service is running %s ->", emoji.PingPong, emoji.PersonRunning)
	t := time.NewTicker(time.Second * 10)
	defer t.Stop()
	for {
		select {
		case <-done:
			logger.Infof("%s  ~ %s PingPort service stopped", emoji.StopSign, emoji.PingPong)
			return
		case <-t.C:
			for i, p := range kdht.RoutingTable().ListPeers() {
				logger.Infof("%s  ~ [%d] Ping -> %s", emoji.PingPong, i, p.String())
				ping_ctx, ping_csl := context.WithTimeout(ctx, time.Millisecond*500)
				err := kdht.Ping(ping_ctx, p)
				ping_csl()
				if err != nil {
					logger.Errorf("%s  ~ [%d] Pong -> %s | %s", emoji.PingPong, i, p.String(), err.Error())
				} else {
					logger.Infof("%s  ~ [%d] Pong -> %s", emoji.PingPong, i, p.String())
				}
			}
		}
	}
}
