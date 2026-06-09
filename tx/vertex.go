package tx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	pb "github.com/Grape-Chain/Grape-Dag/tx/pb"
	"github.com/google/uuid"
	"go.uber.org/atomic"
)

type VertexId struct {
	Id      uuid.UUID
	Address string
	IdMajor uint64
	IdMinor uint32
}

type Vertex struct {
	Id        VertexId
	CumWeight atomic.Float64
	TxWeight  float64
	Timestamp time.Time
	Tx        Transaction
}

func (verId *VertexId) MarshalBinary() *pb.Vertex_VertexId {
	pbVerId := &pb.Vertex_VertexId{}
	pbId, _ := verId.Id.MarshalBinary()
	pbVerId.Id = pbId
	pbVerId.Address = verId.Address
	pbVerId.IdMajor = verId.IdMajor
	pbVerId.IdMinor = verId.IdMinor
	return pbVerId
}

func (verId *VertexId) UnmarshalBinary(pbVerId *pb.Vertex_VertexId) {
	verId.Id, _ = uuid.FromBytes(pbVerId.Id)
	verId.Address = pbVerId.Address
	verId.IdMajor = pbVerId.IdMajor
	verId.IdMinor = pbVerId.IdMinor
}

func (verId *VertexId) String() string {
	buf := bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("Tx[%s]v%d.%d", verId.Address, verId.IdMajor, verId.IdMinor))
	return buf.String()
}

func (verId *VertexId) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Id      uuid.UUID `json:"id"`
		Address string    `json:"address"`
		IdMajor uint64    `json:"id_major"`
		IdMinor uint32    `json:"id_minor"`
	}{
		Id:      verId.Id,
		Address: verId.Address,
		IdMajor: verId.IdMajor,
		IdMinor: verId.IdMinor,
	})
}

func (verId *VertexId) UnmarshalJSON(data []byte) error {
	v := &struct {
		Id      *uuid.UUID `json:"id"`
		Address *string    `json:"address"`
		IdMajor *uint64    `json:"id_major"`
		IdMinor *uint32    `json:"id_minor"`
	}{
		Id:      &verId.Id,
		Address: &verId.Address,
		IdMajor: &verId.IdMajor,
		IdMinor: &verId.IdMinor,
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	return nil

}

func (vertex *Vertex) MarshalBinary() *pb.Vertex {
	return &pb.Vertex{
		VertexId:  vertex.Id.MarshalBinary(),
		CumWeight: vertex.CumWeight.Load(),
		TxWeight:  vertex.TxWeight,
		Tx:        vertex.Tx.MarshalBinary(),
	}
}

func (vertex *Vertex) UnmarshalBinary(pbVer *pb.Vertex) {
	vertex.Id.UnmarshalBinary(pbVer.VertexId)
	vertex.CumWeight.Store(pbVer.CumWeight)
	vertex.TxWeight = pbVer.TxWeight
	vertex.Tx.UnmarshalBinary(pbVer.Tx)
}

func (vertex *Vertex) String() string {
	buf := bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("%s cumW:%f txW:%f Tx:%s",
		vertex.Id.String(), vertex.CumWeight.Load(), vertex.TxWeight, vertex.Tx.String(),
	))
	return buf.String()
}

func (vertex *Vertex) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Id        VertexId `json:"vertex"`
		CumWeight float64  `json:"cum_weight"`
		TxWeight  float64  `json:"tx_weight"`
		Tx        string   `json:"tx"`
	}{
		Id:        vertex.Id,
		CumWeight: vertex.CumWeight.Load(),
		TxWeight:  vertex.TxWeight,
		Tx:        vertex.Tx.String(),
	})
}

func (vertex *Vertex) UnmarshalJSON(data []byte) error {
	var cumWeight float64 = 0.
	v := &struct {
		Id        *VertexId `json:"id"`
		CumWeight *float64  `json:"cum_weight"`
		TxWeight  *float64  `json:"tx_weight"`
		Tx        string    `json:"tx"`
	}{
		Id:        &vertex.Id,
		CumWeight: &cumWeight,
		TxWeight:  &vertex.TxWeight,
		Tx:        vertex.Tx.String(),
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	vertex.CumWeight = *atomic.NewFloat64(cumWeight)
	return nil
}
