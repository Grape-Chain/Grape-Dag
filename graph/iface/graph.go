package iface

import (
	"github.com/golang-collections/collections/set"
)

type Graph interface {
	Vertex(id Vertex_ID_type) Vertex
	From(id Vertex_ID_type) []Vertex
	HasEdgeBetween(xid, yid Vertex_ID_type) bool
	Edge(uid, vid Vertex_ID_type) Edge
}

type VertexOp interface {
	GetTipVertices() *set.Set
	NewEdge(from, to Vertex_ID_type) Edge
}

type Directed interface {
	Graph
	VertexOp
	HasEdgeFromTo(uid, vid Vertex_ID_type) bool
	To(id Vertex_ID_type) []Vertex
}

type Visualizer interface {
	Visualize()
}

type Builder interface {
	VertexAdder
	VertexRemover
}

type DirectedBuilder interface {
	Directed
	Builder
	Visualizer
}
