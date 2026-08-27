package dag

import (
	"runtime"
	"sort"

	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/ledongthuc/goterators"
)

func __noop__() {
	runtime.Gosched()
}

func (dag *Dag) GetNodeVer(node *Node) (uint64, uint32) {
	if dag != nil {
		if node != nil {
			return node.id.idMajor, node.id.idMinor
		}
	}
	return 0, 0
}

func (thisLink *Link) Equal(thatLink *Link) bool {
	switch {
	case thisLink == thatLink:
		return true
	case !thisLink.source.Equal(thatLink.source):
		return false
	case !thisLink.target.Equal(thatLink.target):
		return false
	default:
		return true
	}
}

func (thisNodeID *NodeID) Equal(thatNodeID *NodeID) bool {
	switch {
	case thisNodeID == thatNodeID:
		return true
	case thisNodeID.id != thatNodeID.id:
		return false
	case thisNodeID.idMajor != thatNodeID.idMajor:
		return false
	case thisNodeID.idMinor != thatNodeID.idMinor:
		return false
	default:
		return true
	}
}

func (thisNode *Node) Equal(thatNode *Node) bool {
	if thisNode != nil && thatNode != nil {
		return thisNode.id.id == thatNode.id.id
	}
	return false
	// switch {
	// case thisNode == thatNode:
	// 	return true
	// case !thisNode.id.Equal(&thatNode.id):
	// 	return false
	// case thisNode.time != thatNode.time:
	// 	return false
	// case !thisNode.tx.Equal(&thatNode.tx):
	// 	return false
	// default:
	// 	return true
	// }
}

func topologicalSort(nodes []*Node, links []Link) []*Node {
	// get a list of children for the slice of nodes/links
	childrenLists := getTargetLists(nodes, links)
	unvisited := make([]*Node, len(nodes))
	copy(unvisited, nodes)
	result := []*Node{}
	var visit func(*Node)
	visit = func(node1 *Node) {
		// find this node in unvisited, if not found, return
		_, _, err := goterators.Find(unvisited, func(node2 *Node) bool {
			return node1.Equal(node2)
		})
		// is the node found?
		if err != nil {
			return
		}
		// Obtain a list of children for node1
		if len(childrenLists) > 0 {
			targetNodes, ok := childrenLists[node1.id.idMajor]
			if ok {
				// if has children, visit them
				for _, target := range targetNodes {
					visit(target)
				}
			}
		}
		// previous calls to visit may have removed this node from unvisited
		// re-find this node again, if found - append
		if _, i, err := goterators.Find(unvisited, func(node2 *Node) bool {
			return node1.Equal(node2)
		}); err == nil {
			// shrink unvisited by 1, since we visited yet another node from unvisited
			if len(unvisited) > i {
				unvisited = append(unvisited[:i], unvisited[i+1:]...)
			}
			// all visited nodes get moved to result in order
			result = append(result, node1)
		}
	}
	// visit all nodes one by one
	for len(unvisited) > 0 {
		node := unvisited[len(unvisited)-1]
		visit(node)
	}
	// need to reverse the order
	// ReverseSlice(result)
	return result
}

func ReverseSlice[T any](s []T) {
	sort.SliceStable(s, func(i, j int) bool {
		return i > j
	})
}

func (dag *Dag) printDag(pref string) {
	goterators.ForEach(dag._dag_, func(n *Node) {
		utils.ColorizePrint("[%s]  %s|%d|%d\n", pref, n.id.id.String(), n.id.idMajor, n.id.idMinor)
	})
}
