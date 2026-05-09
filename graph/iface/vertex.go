package iface

type Vertex interface {
	VertexSetter
	VertexGetter
	Id() Vertex_ID_type
	Major() uint64
	Minor() uint32
	SetDepth(uint64)
	GetDepth() uint64
	String() string
}

type VertexAdder interface {
	NewVertex() Vertex
	AddVertex(Vertex)
}

type VertexSetter interface {
	AddEdge(Edge)
}

type VertexGetter interface {
	GetEdges() []Edge
}
type VertexRemover interface {
	RemoveVertex(id Vertex_ID_type)
}
