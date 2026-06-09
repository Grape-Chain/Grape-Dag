package discovery

import (
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

type Mesh struct {
	nodes map[string][]peer.AddrInfo
	mu    sync.Mutex
}

func (m *Mesh) In(topic string, id peer.ID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.nodes[topic]; ok {
		for _, n := range v {
			if n.ID == id {
				return true
			}
		}
	}
	return false
}

func (m *Mesh) Get(topic string) []peer.AddrInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.nodes[topic]; ok {
		return v
	}
	return nil
}

func (m *Mesh) Add(topic string, p []peer.AddrInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// this is a bulk re-write based on what dutils.FindPeers returns
	m.nodes[topic] = p
}

func GetMesh() *Mesh {
	return topic_discovery
}

var topic_discovery *Mesh = nil

func init() {
	topic_discovery = &Mesh{
		nodes: make(map[string][]peer.AddrInfo),
		mu:    sync.Mutex{},
	}
}
