package dag

import (
	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/golang-collections/collections/set"
	"github.com/golang-collections/collections/stack"
	"github.com/google/uuid"
	"github.com/ledongthuc/goterators"
)

func (d *Dag) visit(visited *set.Set, workset *stack.Stack, id uuid.UUID) {
	visited.Insert(id)
	vertex := d.Vertex(id)
	// stop at the latest pinning transaction or genesis
	if vertex.tx.GetTransactionType() == tx.SERVICE_PIN {
		return
	}
	ids := d.mapped_edges[id] // for the given source, get target ids
	goterators.ForEach(ids, func(id uuid.UUID) {
		workset.Push(id)
	})
	// visiting target vertices
	for workset.Len() > 0 {
		id := workset.Pop().(uuid.UUID)
		if !visited.Has(id) {
			if vertex := d.Vertex(id); vertex != nil && vertex.tx.GetTransactionType() != tx.SERVICE_PIN {
				d.visit(visited, workset, id)
			}
		}
	}
}

func (d *Dag) traverseDagSlice(ids []uuid.UUID) []uuid.UUID {
	visited := set.New()
	workset := stack.New()
	d.updateMappedVertices()
	d.updateMappedEdges()
	goterators.ForEach(ids, func(id uuid.UUID) {
		d.visit(visited, workset, id)
	})
	txs := []uuid.UUID{}
	visited.Do(func(i interface{}) {
		txs = append(txs, i.(uuid.UUID))
	})
	return txs
}

func NodeToVertex(node *Node) *tx.Vertex {
	vertex := &tx.Vertex{
		Id: tx.VertexId{
			Id:      node.id.id,
			IdMajor: node.id.idMajor,
			IdMinor: node.id.idMinor,
		},
		CumWeight: node.cumWeight,
		TxWeight:  node.txWeight,
		Timestamp: node.time,
		Tx:        node.tx,
	}
	return vertex
}

func (d *Dag) getFromLastPinTx() ([]tx.Vertex, []tx.Edge) {
	d.mux.Lock()
	defer d.mux.Unlock()
	// get the last pin tx
	lastPinTx := d.pins[len(d.pins)-1]
	edges := []tx.Edge{}
	// d._links_ | target Tx <- source Tx
	//recursive function to traverse the dag from the current pin tx to the tips
	var f func(*set.Set, *stack.Stack, *tx.Vertex)
	f = func(vis *set.Set, ws *stack.Stack, vertex *tx.Vertex) {
		vis.Insert(vertex)
		// for this vertex let's find the edges that link to it
		// note: the link direction is from tips to roots
		// need to reverse the order - from roots to tips
		flag := false
		edge_ids := []uuid.UUID{}
		goterators.ForEach(d._links_, func(link Link) {
			// looking for links where the vertex is the target
			if link.target.id.id == vertex.Id.Id {
				ws.Push(NodeToVertex(link.source))
				edge_ids = append(edge_ids, link.source.id.id)
				flag = true
			}
		})
		if !flag {
			return
		}
		edges = append(edges, tx.Edge{
			Vertex: vertex.Id.Id,
			Edges:  edge_ids,
		})
		for ws.Len() > 0 {
			v := ws.Pop().(*tx.Vertex)
			if !vis.Has(v) {
				f(vis, ws, v)
			}
		}
	}
	vis := set.New()
	ws := stack.New()
	f(vis, ws, NodeToVertex(lastPinTx))
	vertices := []tx.Vertex{}
	vis.Do(func(i interface{}) {
		vertices = append(vertices, *i.(*tx.Vertex))
	})
	return vertices, edges
}

func (d *Dag) getLatestDagSlice() ([]tx.Vertex, []tx.Edge) {
	tipVertices := _graph_.GetTipVertices()
	tipIds := pinTxIDs(tipVertices)
	vertices := []tx.Vertex{}
	edges := []tx.Edge{}
	// traverse to the latest slice root
	sliceTxs := d.traverseDagSlice(tipIds)

	goterators.ForEach(sliceTxs, func(id uuid.UUID) {
		v := d.Vertex(id)
		vertex := tx.Vertex{
			Id: tx.VertexId{
				Id:      v.id.id,
				IdMajor: v.id.idMajor,
				IdMinor: v.id.idMinor,
			},
			CumWeight: v.cumWeight,
			TxWeight:  v.txWeight,
			Timestamp: v.time,
			Tx:        v.tx,
		}
		vertices = append(vertices, vertex)
		es := tx.Edge{
			Vertex: id,
			Edges:  d.mapped_edges[id],
		}
		edges = append(edges, es)
	})
	return vertices, edges
}
