package graph

import (
	"bytes"
	"fmt"

	"github.com/VG-Grape/luna/graph/iface"
	"github.com/ledongthuc/goterators"
)

type DVertex struct {
	id    iface.Vertex_ID_type
	major uint64
	minor uint32
	depth uint64
	edges []iface.Edge
}

func NewVertex(id iface.Vertex_ID_type, major uint64, minor uint32) iface.Vertex {
	return &DVertex{
		id:    id,
		major: major,
		minor: minor,
		depth: 0,
		edges: []iface.Edge{},
	}
}

func (v *DVertex) Id() iface.Vertex_ID_type {
	return v.id
}

func (v *DVertex) Major() uint64 {
	return v.major
}

func (v *DVertex) Minor() uint32 {
	return v.minor
}

func (v *DVertex) SetDepth(d uint64) {
	v.depth = d
}

func (v *DVertex) GetDepth() uint64 {
	return v.depth
}

func (v *DVertex) GetEdges() []iface.Edge {
	return v.edges
}

func (v *DVertex) AddEdge(e iface.Edge) {
	v.edges = append(v.edges, e)
}

func (v *DVertex) String() string {
	var b bytes.Buffer = bytes.Buffer{}
	b.WriteString(fmt.Sprintf("ID: %d\n", v.id))
	b.WriteString(fmt.Sprintf("Depth: %d\n", v.depth))
	e := v.GetEdges()
	goterators.ForEach(e, func(edge iface.Edge) {
		b.WriteString(fmt.Sprintf("Edge: %d\n", edge.To()))
	})

	return b.String()
}
