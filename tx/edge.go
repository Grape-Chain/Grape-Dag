package tx

import (
	"bytes"
	"encoding/json"
	"fmt"

	pb "github.com/VG-Grape/luna/tx/pb"
	"github.com/google/uuid"
	"github.com/ledongthuc/goterators"
)

type Edge struct {
	Vertex uuid.UUID
	Edges  []uuid.UUID
}

func (edge *Edge) MarshalBinary() *pb.Edge {
	binTxid, _ := edge.Vertex.MarshalBinary()
	pbEdge := &pb.Edge{
		Tx: binTxid,
	}
	goterators.ForEach(edge.Edges, func(id uuid.UUID) {
		binTxid, _ := id.MarshalBinary()
		pbEdge.Edges = append(pbEdge.Edges, binTxid)
	})
	return pbEdge
}

func (edge *Edge) UnmarshalBinary(pbEdge *pb.Edge) {
	edge.Vertex.UnmarshalBinary(pbEdge.Tx)
	goterators.ForEach(pbEdge.Edges, func(e []byte) {
		id, _ := uuid.FromBytes(e)
		edge.Edges = append(edge.Edges, id)
	})
}

func (edge *Edge) String() string {
	strBuf := bytes.Buffer{}
	strBuf.WriteString(fmt.Sprintf("\nTx: %s \n", edge.Vertex.String()))
	goterators.ForEach(edge.Edges, func(id uuid.UUID) {
		strBuf.WriteString(fmt.Sprintf("\tedge: %s\n", id.String()))
	})
	return strBuf.String()
}

func (e *Edge) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Vertex uuid.UUID   `json:"vertex"`
		Edges  []uuid.UUID `json:"edges"`
	}{
		Vertex: e.Vertex,
		Edges:  e.Edges,
	})
}

func (edge *Edge) UnmarshalJSON(data []byte) error {
	v := &struct {
		Vertex *uuid.UUID   `json:"vertex"`
		Edges  *[]uuid.UUID `json:"edges"`
	}{
		Vertex: &edge.Vertex,
		Edges:  &edge.Edges,
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	return nil
}
