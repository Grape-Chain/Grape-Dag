package graph

import (
	"fmt"
	"os"
	"sync"

	"github.com/VG-Grape/luna/graph/iface"
	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
	"github.com/golang-collections/collections/set"
	"github.com/golang-collections/collections/stack"
	"github.com/google/uuid"
	"github.com/ledongthuc/goterators"
	"golang.org/x/exp/maps"
)

var file_count = 0

type PinTxStack struct {
	pinStack *stack.Stack
	mx       sync.Mutex
}

func (s *PinTxStack) Len() int {
	s.mx.Lock()
	defer s.mx.Unlock()
	return s.pinStack.Len()
}

func (s *PinTxStack) Push(i interface{}) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.pinStack.Push(i)
}

func (s *PinTxStack) Pop() interface{} {
	s.mx.Lock()
	defer s.mx.Unlock()
	return s.pinStack.Pop()
}

func (s *PinTxStack) Peek() interface{} {
	s.mx.Lock()
	defer s.mx.Unlock()
	return s.pinStack.Peek()
}

func (s *PinTxStack) PeekPrev() interface{} {
	s.mx.Lock()
	defer s.mx.Unlock()
	last := s.pinStack.Pop()
	prev := s.pinStack.Peek()
	s.pinStack.Push(last)
	return prev
}

func NewPinStack() *PinTxStack {
	return &PinTxStack{
		pinStack: stack.New(),
	}
}

var pinStack *PinTxStack = NewPinStack()

func GetGraphPinStack() *PinTxStack {
	if pinStack == nil {
		pinStack = NewPinStack()
	}
	return pinStack
}

type DGraph struct {
	vertices map[iface.Vertex_ID_type]iface.Vertex
}

func NewGraph() iface.DirectedBuilder {
	return &DGraph{
		vertices: make(map[iface.Vertex_ID_type]iface.Vertex),
	}
}

func (g *DGraph) Vertex(id iface.Vertex_ID_type) iface.Vertex {
	i := g.vertices[id]
	return i
}

func (g *DGraph) Vertices() []iface.Vertex {
	return maps.Values(g.vertices)
}

func (g *DGraph) From(id iface.Vertex_ID_type) []iface.Vertex {
	return nil
}

func (g *DGraph) HasEdgeBetween(xid, yid iface.Vertex_ID_type) bool {
	return false
}

func (g *DGraph) Edge(uid, vid iface.Vertex_ID_type) iface.Edge {
	return nil
}

func (g *DGraph) HasEdgeFromTo(uid, vid iface.Vertex_ID_type) bool {
	return false
}

func (g *DGraph) To(id iface.Vertex_ID_type) []iface.Vertex {
	return nil
}

func (g *DGraph) NewVertex() iface.Vertex {
	return &DVertex{
		id: iface.Vertex_ID_type(uuid.New()),
	}
}

func (g *DGraph) AddVertex(v iface.Vertex) {
	// Check if there is a collision, if we panic, improve algorithm for uuid2uint64
	if _, ok := g.vertices[v.Id()]; !ok {
		g.vertices[v.Id()] = v
	}
}

func (g *DGraph) visit(ws *stack.Stack, vis *set.Set, tips *set.Set, cur iface.Vertex) {
	vis.Insert(cur.Id())
	es := cur.GetEdges()
	len := len(es)
	if len < 2 {
		tips.Insert(g.Vertex(cur.Id()))
	}
	if len == 0 {
		return
	}
	goterators.ForEach(es, func(e iface.Edge) {
		ws.Push(e.To())
	})
	for ws.Len() > 0 {
		id := ws.Pop().(iface.Vertex_ID_type)
		if !vis.Has(id) {
			g.visit(ws, vis, tips, g.Vertex(id))
		}
	}
}

func (g *DGraph) GetTipVertices() *set.Set {
	// Get the genesis vertex
	v := g.Vertex(iface.Vertex_ID_type(uuid.Nil))
	workset := stack.New()
	visited := set.New()
	tips := set.New()
	// Walk the tree from the very beginning
	// Remember: working with the reverse edges, from genesis to tips
	g.visit(workset, visited, tips, v)
	return tips
}

func (g *DGraph) NewEdge(from, to iface.Vertex_ID_type) iface.Edge {
	return &DEdge{
		from: from,
		to:   to,
	}
}

func (g *DGraph) RemoveVertex(id iface.Vertex_ID_type) {
	delete(g.vertices, id)
}

func (g *DGraph) addVisVertices(grph graph.Graph[int, int]) {
	// Visualize from previous pinning tx to the latest
	prevPinTxId := pinStack.PeekPrev().(uuid.UUID)
	lastPinTxId := pinStack.Peek().(uuid.UUID)
	fromVertex := g.vertices[iface.Vertex_ID_type(prevPinTxId)]
	ws := stack.New()
	vis := set.New()
	var fvis func(*stack.Stack, *set.Set, iface.Vertex)
	fvis = func(ws *stack.Stack, vis *set.Set, cur iface.Vertex) {
		vis.Insert(cur.Id())
		if cur.Id() == iface.Vertex_ID_type(prevPinTxId) || cur.Id() == iface.Vertex_ID_type(lastPinTxId) {
			grph.AddVertex(int(cur.Major()), graph.VertexAttribute("style", "filled"), graph.VertexAttribute("fillcolor", "red"))
		} else {
			grph.AddVertex(int(cur.Major()), graph.VertexAttribute("bgcolor", "lightgrey"))
		}
		es := cur.GetEdges()
		len := len(es)
		if len == 0 {
			return // we should return when we reach the latest pin tx
		}
		goterators.ForEach(es, func(e iface.Edge) {
			ws.Push(e.To())
		})
		for ws.Len() > 0 {
			id := ws.Pop().(iface.Vertex_ID_type)
			if !vis.Has(id) {
				fvis(ws, vis, g.Vertex(id))
			}
		}
	}
	fvis(ws, vis, fromVertex)
}

func (g *DGraph) addVisEdges(grph graph.Graph[int, int]) {
	prevPinTxId := pinStack.PeekPrev().(uuid.UUID)
	fromVertex := g.vertices[iface.Vertex_ID_type(prevPinTxId)]
	ws := stack.New()
	vis := set.New()
	var fvis func(*stack.Stack, *set.Set, iface.Vertex)
	fvis = func(ws *stack.Stack, vis *set.Set, cur iface.Vertex) {
		vis.Insert(cur.Id())
		vertex := g.vertices[cur.Id()]
		es := cur.GetEdges()
		len := len(es)
		if len == 0 {
			return // we should return when we reach the latest pin tx
		}
		goterators.ForEach(es, func(e iface.Edge) {
			fromVertex := g.vertices[e.From()]
			toVertex := g.vertices[e.To()]
			grph.AddEdge(
				int(fromVertex.Major()),
				int(toVertex.Major()),
				graph.EdgeAttribute("label", fmt.Sprintf("%d", int(vertex.GetDepth()))),
			)
			ws.Push(e.To())
		})
		for ws.Len() > 0 {
			id := ws.Pop().(iface.Vertex_ID_type)
			if !vis.Has(id) {
				fvis(ws, vis, g.Vertex(id))
			}
		}
	}
	fvis(ws, vis, fromVertex)
}

func (g *DGraph) visPopulateGraph(grph graph.Graph[int, int]) {
	g.addVisVertices(grph)
	g.addVisEdges(grph)
}

func (g *DGraph) Visualize() {
	grph := graph.New(graph.IntHash, graph.Weighted(), graph.Directed(), graph.Acyclic())
	g.visPopulateGraph(grph)
	file_count++
	file, _ := os.Create(fmt.Sprintf("./dag.graph.%d.gv", file_count))
	_ = draw.DOT(grph, file)
}
