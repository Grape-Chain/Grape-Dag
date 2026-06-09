package graph

import "github.com/Grape-Chain/Grape-Dag/graph/iface"

type DEdge struct {
	from iface.Vertex_ID_type
	to   iface.Vertex_ID_type
}

func NewEdge(f, t iface.Vertex_ID_type) iface.Edge {
	return &DEdge{
		from: f,
		to:   t,
	}
}

func (e *DEdge) From() iface.Vertex_ID_type {
	return e.from
}

func (e *DEdge) To() iface.Vertex_ID_type {
	return e.to
}

func (e *DEdge) ReversedEdge() iface.Edge {
	return &DEdge{
		from: e.to,
		to:   e.from,
	}
}
