package dag

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"

	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
	"github.com/ledongthuc/goterators"
)

func (dag *Dag) visPopulateGraph(g graph.Graph[int, int], vertices []*Node) {
	goterators.ForEach(vertices, func(v *Node) {
		if GetPin().InPin(v) {
			g.AddVertex(int(v.id.idMajor), graph.VertexAttribute("style", "filled"), graph.VertexAttribute("fillcolor", "green"))
		} else {
			if !v.valid {
				g.AddVertex(int(v.id.idMajor), graph.VertexAttribute("style", "filled"), graph.VertexAttribute("fillcolor", "red"))
			} else {
				g.AddVertex(int(v.id.idMajor), graph.VertexAttribute("style", "filled"), graph.VertexAttribute("fillcolor", "yellow"))
			}
		}
	})
	var edges []Link
	goterators.ForEach(vertices, func(vertex *Node) {
		edges = append(edges, goterators.Filter(dag._links_, func(edge Link) bool {
			return edge.source.id.idMajor == vertex.id.idMajor
		})...)
	})
	goterators.ForEach(edges, func(edge Link) {
		if edge.source.tx.GetTransactionType() == tx.SERVICE_PIN {
			g.AddEdge(int(edge.target.id.idMajor),
				int(edge.source.id.idMajor),
				graph.EdgeAttribute("dir", "back"),
				graph.EdgeAttribute("color", "red"),
				graph.EdgeAttribute("style", "dotted"),
				graph.EdgeAttribute("arrowhead", "empty"),
			)
		} else {
			g.AddEdge(int(edge.target.id.idMajor),
				int(edge.source.id.idMajor),
				graph.EdgeAttribute("dir", "back"),
			)
		}
	})
}

// Note: due to a bug in libgv6 limit the numbe of vertices to 90
// and roll over to the next file
func (dag *Dag) visualizeMcmcPlus(peerID string) {
	if dag != nil {
		var (
			fileID int = 0
			//			offset int   = 0
			//			idx    int   = 0
			//			val    *Node = nil
			//			idxPin int   = 0
		)
		var vertices []*Node
		vertices = dag._dag_[:]
		g := graph.New(graph.IntHash, graph.Weighted(), graph.Directed(), graph.Acyclic())
		dag.visPopulateGraph(g, vertices)
		fileID++
		fmt.Printf("Creating file grapepeer.%s.%d.gv", peerID, fileID)
		file, _ := os.Create(fmt.Sprintf("./grapepeer.%s.%d.gv", peerID, fileID))
		_ = draw.DOT(g, file)
	}
}

func (dag *Dag) visualizeMcmcPlusPlus(peerID string) {
	if dag != nil {
		var (
			fileID int   = 0
			offset int   = 0
			idx    int   = 0
			val    *Node = nil
			idxPin int   = 0
		)
		for idx, val = range dag._dag_ {
			var vertices []*Node
			if val.tx.GetTransactionType() == tx.SERVICE_PIN {
				idxPin = idx
				vertices = dag._dag_[offset : idx+1]
				g := graph.New(graph.IntHash, graph.Weighted(), graph.Directed(), graph.Acyclic())
				dag.visPopulateGraph(g, vertices)
				offset = idx
				fileID++
				fmt.Printf("Creating file grapepeer.%s.%d.gv", peerID, fileID)
				file, _ := os.Create(fmt.Sprintf("./grapepeer.%s.%d.gv", peerID, fileID))
				_ = draw.DOT(g, file)
			}
		}
		// Visualize the remainder of the DAG after the last pinning transaction
		if idxPin < idx {
			g := graph.New(graph.IntHash, graph.Weighted(), graph.Directed(), graph.Acyclic())
			vertices := dag._dag_[idxPin:]
			dag.visPopulateGraph(g, vertices)
			fileID++
			fmt.Printf("Creating file grapepeer.%s.%d.gv", peerID, fileID)
			file, _ := os.Create(fmt.Sprintf("./grapepeer.%s.%d.gv", peerID, fileID))
			_ = draw.DOT(g, file)
		}
	}
}

func (dag *Dag) visualizeRandomWalk(peerID string) {
	if dag != nil {
		g := graph.New(graph.IntHash, graph.Weighted(), graph.Directed(), graph.Acyclic())

		goterators.ForEach(dag._dag_, func(node *Node) {
			g.AddVertex(int(node.id.idMajor))
		})

		goterators.ForEach(dag._links_, func(link Link) {
			weight := link.target.txWeight * 6 / (txConfig.Neutrino * float64(txConfig.Maxfuellimit*txConfig.Maxfuelprice))
			g.AddEdge(int(link.target.id.idMajor),
				int(link.source.id.idMajor),
				graph.EdgeWeight(int(link.target.txWeight)),
				graph.EdgeAttribute("dir", "back"),
				graph.EdgeAttribute("label", strconv.FormatFloat(weight, 'f', 4, 64)),
			)
		})

		file, _ := os.Create(fmt.Sprintf("./grapepeer.%s.gv", peerID))
		_ = draw.DOT(g, file)
	}
}

type Signature struct {
	sign []byte
}

const sign_limit = 10

var signHash = func(s Signature) string {
	v := hex.EncodeToString(s.sign)[:sign_limit]
	return v
}

func (d *Dag) visualizePins(id string) {
	g := graph.New(signHash, graph.Directed())
	pinl := len(_pins_.pins)
	for i := 0; i < pinl; i++ {
		sign := _pins_.pins[i].Sign
		g.AddVertex(
			Signature{sign: sign},
			graph.VertexAttribute("style", "filled"),
			graph.VertexAttribute("fillcolor", "green"))
	}

	for i := pinl - 1; i > 0; i-- {
		sign := hex.EncodeToString(_pins_.pins[i].Sign)[:sign_limit]
		prev := hex.EncodeToString(_pins_.pins[i].Prev)[:sign_limit]
		lsites := len(_pins_.pins[i].Sites)
		g.AddEdge(sign, prev,
			graph.EdgeAttribute("dir", "back"),
			graph.EdgeAttribute("label", fmt.Sprintf("%s[%d]... => %s...", sign[:5], lsites, prev[:5])))
	}

	file, _ := os.Create(fmt.Sprintf("./grapepeer.pin.%s.gv", id))
	_ = draw.DOT(g, file)

}

func (dag *Dag) Visualize(peerID string) {
	if dag != nil {
		switch dagAlgorithm() {
		case DAG_ALGO_MCMCP.Type():
			logger.Info("Visualize MCMC+")
			dag.visualizeMcmcPlus(peerID)
		case DAG_ALGO_MCMCPP.Type():
			logger.Info("Visualize MCMC++")
			dag.visualizeMcmcPlusPlus(peerID)
		case DAG_ALGO_RANDOM.Type():
			logger.Info("Visualize Random Walk")
			dag.visualizeRandomWalk(peerID)
		}
		logger.Info("Visualize Pins...")
		dag.visualizePins(peerID)
	}
}
