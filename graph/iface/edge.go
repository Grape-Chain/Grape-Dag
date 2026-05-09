package iface

import "github.com/google/uuid"

type Vertex_ID_type uuid.UUID

type Edge interface {
	From() Vertex_ID_type
	To() Vertex_ID_type
	ReversedEdge() Edge
}

type EdgeAdder interface {
	AddEdge(from, to Vertex_ID_type)
}
type EdgeRemover interface {
	RemoveEdge(fid, tid Vertex_ID_type)
}
