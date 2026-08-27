package dag

import (
	"math/rand"

	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/google/uuid"
	"github.com/ledongthuc/goterators"
)

// Note:
// 	other vertices (nodes) this vertex references in DAG are targets for this node
//	and all other vertices (nodes) that reference this vertex (node) in DAG are sources
//  in regard to this node which is their target
//  vertex (node) <<--[source]-- vertex (current node) <<-- [target] -- vertex (after)

func (dag *Dag) AddTxDag(node *Node) ([]uuid.UUID, map[string][]byte, error) {
	dag.mux.Lock()
	defer dag.mux.Unlock()
	var uuIDs []uuid.UUID = []uuid.UUID{}
	var signatures map[string][]byte = map[string][]byte{}
	var tips []*Node
	if node.tx.GetTransactionType() == tx.PAYMENT {
		// How far the ledger has come, not how much of it is resident: slicing
		// shrinks the live graph, and measuring that would drop the node back
		// into the genesis-fanout phase and link new sites to genesis again.
		if dag.sitesAdded.Load() < uint64(dag.width) {
			tips = append(tips, dag.getGenesis())
		} else {
			// list of nodes in dag that came before this node

			candidates := tipCache().getTips()

			candidateLinks := []Link{}

			for _, v := range candidates {
				for _, t := range v.targets {
					candidateLinks = append(candidateLinks, Link{
						source: v,
						target: t,
					})
				}
			}

			rand.Shuffle(len(candidates), func(i, j int) {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			})
			if dagConfig.Algorithm != DAG_ALGO_RANDOM.Type() {
				tips = weightedMcmc(candidates, candidateLinks, dagConfig.Alpha)
			} else {
				c1 := rand.Int31n(int32(len(candidates)))
				tips = append(tips, candidates[c1])
				if len(candidates) > 1 {
					var c2 int32 = 0
					for {
						c2 = rand.Int31n(int32(len(candidates)))
						if c1 != c2 {
							break
						}
					}
					tips = append(tips, candidates[c2])
				}
			}
		}
	}
	if dag.approveTx(tips) {
		height := Height{0, 0}
		// Set this node's version
		//dag.updateNodeVer(node)
		// add links from the current (new) node to the one/two tips in the dag
		if len(tips) > 0 {
			// MODS
			goterators.ForEach(tips, func(tip *Node) {
				signatures[tip.id.id.String()] = tip.tx.GetSignature()
				dag._links_ = append(dag._links_, Link{source: node, target: tip})
				uuIDs = append(uuIDs, tip.id.id)
				if tip.height.maxheight > height.maxheight {
					height.maxheight = tip.height.maxheight + 1
				}
				if tip.height.minheight > height.minheight {
					height.minheight = tip.height.minheight + 1
				}
			})
		}
		node.height = height
		if _, err := dag.addToDag(node, tips); err != nil {
			return nil, nil, err
		}
	}

	// To speed things up, ignore cumWeights when working with MCMCPP
	if dagConfig.Algorithm == DAG_ALGO_MCMCP.Type() {
		// ReverseSlice(dag._dag_)
		dag._dag_ = updateCumWeights(dag._dag_, dag._links_)
		// ReverseSlice(dag._dag_)
	}

	//dag.dag = updateFwdWeights(dag.dag, dag.links)
	if peerConfig.Console > 0 {
		dag.logLast("ADD:", node, 1)
	}
	return uuIDs, signatures, nil
}

func vertexToNode(vertex *tx.Vertex) *Node {
	return &Node{
		id: NodeID{
			id:      vertex.Id.Id,
			idMajor: vertex.Id.IdMajor,
			idMinor: vertex.Id.IdMinor,
		},
		cumWeight: vertex.CumWeight,
		txWeight:  vertex.TxWeight,
		time:      vertex.Timestamp,
		tx:        vertex.Tx,
	}
}
