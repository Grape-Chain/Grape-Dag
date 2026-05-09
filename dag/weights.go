package dag

import (
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/ledongthuc/goterators"
	"github.com/montanaflynn/stats"
)

var _r_ *rand.Rand = nil

func init() {
	_r_ = rand.New(rand.NewSource(time.Hour.Microseconds()))
}

func calculateWeightsMCMC_P(nodes []*Node) []float64 {
	alpha := dagConfig.Alpha
	cumWeights := goterators.Map(nodes, func(node *Node) float64 {
		return node.cumWeight.Load()
	})
	div := goterators.Reduce(cumWeights, 0, func(prev float64, cur float64, idx int, l []float64) float64 {
		return prev + math.Exp(-alpha*float64(cur))
	}) + 0.01
	weights := goterators.Map(cumWeights, func(w float64) float64 {
		return math.Exp(-alpha*w) / div
	})
	return weights
}

func calculateWeightsMCMC_PP(nodes []*Node) []float64 {
	alpha := dagConfig.Alpha
	// reuse weights from random walk - returns a slice of normalized weights
	weights := calculateWeightsRandom(nodes)

	div := goterators.Reduce(weights, 0, func(prev float64, cur float64, idx int, l []float64) float64 {
		return prev + math.Exp(-alpha*float64(cur))
	}) + 0.01
	probs := goterators.Map(weights, func(w float64) float64 {
		return math.Exp(-alpha*w) / div
	})
	sumProbs := goterators.Reduce(probs, 0, func(prev float64, cur float64, idx int, l []float64) float64 {
		return prev + cur
	})
	normProbs := goterators.Map(probs, func(p float64) float64 {
		return p / sumProbs
	})
	return normProbs
}

func calculateWeightsRandom(nodes []*Node) []float64 {
	sumWeights := goterators.Reduce(nodes, 0, func(prev float64, cur *Node, idx int, l []*Node) float64 {
		return prev + cur.txWeight
	})
	normWeights := goterators.Map(nodes, func(node *Node) float64 {
		return node.txWeight / sumWeights
	})
	return normWeights
}

var algox_fn map[string]func([]*Node) []float64 = map[string]func([]*Node) []float64{
	DAG_ALGO_MCMCP.Type():  calculateWeightsMCMC_P,
	DAG_ALGO_MCMCPP.Type(): calculateWeightsMCMC_PP,
	DAG_ALGO_RANDOM.Type(): calculateWeightsRandom,
}

func calculateTxWeights(nodes []*Node) []float64 {
	fn, ok := algox_fn[strings.ToLower(dagConfig.Algorithm)]
	if ok {
		return fn(nodes)
	}
	return nil
}

func weightedMcmc(nodes []*Node, links []Link, alpha float64) []*Node {
	if len(nodes) == 0 {
		return []*Node{}
	}

	//nodes = calculateWeights(nodes, links)
	if len(nodes) == 2 {
		return nodes
	}

	var tipSelection []*Node = make([]*Node, len(nodes))
	var linkSelection []Link = make([]Link, len(links))
	copy(tipSelection, nodes)
	copy(linkSelection, links)

	// there could be no tx after the pinning transaction, in such a case just return
	// a pinning transaction
	if len(tipSelection) == 0 {
		logger.Warn("No tx to link to. This is exceptionally wrong. Fix me!")
		return []*Node{}
	}
	// look for the first tip
	tip1 := weightedRandomWalk(tipSelection, links, links, tipSelection[rand.Int31n(int32(len(tipSelection)))], alpha)
	// exclude tip1 from the next search if it's found
	if tip1 != nil && len(tipSelection) > 1 {
		_, i, err := goterators.Find(tipSelection, func(node *Node) bool {
			return tip1.id.id == node.id.id
		})
		if err == nil {
			tipSelection = append(tipSelection[:i], tipSelection[i+1:]...)
		}
		for {
			_, i, err = goterators.Find(linkSelection, func(link Link) bool {
				return link.source.id.id == tip1.id.id
			})
			if err == nil {
				linkSelection = append(linkSelection[:i], linkSelection[i+1:]...)
			} else {
				break
			}
		}
	}
	tip2 := weightedRandomWalk(tipSelection, linkSelection, links, tipSelection[rand.Int31n(int32(len(tipSelection)))], alpha)
	if tip1 == nil && tip2 == nil {
		return []*Node{}
	} else if tip2 == nil {
		return []*Node{tip1}
	}

	return []*Node{tip1, tip2}
}

func weightedMcmcPlusPlus(nodes []*Node, links []Link, alpha float64) []*Node {
	if len(nodes) == 0 {
		return []*Node{}
	}

	//nodes = calculateWeights(nodes, links)
	if len(nodes) == 2 {
		return nodes
	}
	var tipSelection []*Node = make([]*Node, len(nodes))
	var linkSelection []Link = make([]Link, len(links))
	copy(tipSelection, nodes)
	copy(linkSelection, links)

	tip1 := weightedRandomWalk(tipSelection, links, links, tipSelection[0], alpha)
	// exclude tip1 from the next search if it's found
	if tip1 != nil && len(tipSelection) > 1 {
		_, i, err := goterators.Find(tipSelection, func(node *Node) bool {
			return tip1.Equal(node)
		})
		if err == nil {
			tipSelection = append(tipSelection[:i], tipSelection[i+1:]...)
		}
		for {
			_, i, err = goterators.Find(linkSelection, func(link Link) bool {
				return link.source.Equal(tip1)
			})
			if err == nil {
				linkSelection = append(linkSelection[:i], linkSelection[i+1:]...)
			} else {
				break
			}
		}
	}
	tip2 := weightedRandomWalk(tipSelection, linkSelection, links, tipSelection[0], alpha)

	if tip1.Equal(tip2) {
		return []*Node{tip1}
	}

	return []*Node{tip1, tip2}
}

// This will be further optimize by parallelizing calculation of weights
func updateCumWeights(nodes []*Node, links []Link) []*Node {

	for i := len(nodes) - 1; i >= 0; i-- {
		nodes[i].cumWeight.Store(goterators.Reduce(nodes[i].sources, 0, func(prev float64, cur *Node, idx int, l []*Node) float64 {
			return prev + cur.cumWeight.Load()
		}) + float64(dagConfig.Cummstep))
	}

	return nodes
}

func weightedRandomWalk(nodes []*Node, linkSelection []Link, links []Link, start *Node, alpha float64) *Node {
	particle := start
	approvetx := int(dagConfig.Approvetx)
	count := len(links)
	for !isNodeTip(links, particle, approvetx) {
		approvers := getApprovers(linkSelection, particle)
		// call based on the algo choice
		rc := weightedSelect(approvers)
		if rc != nil {
			particle = rc
		}
		count--
		if count == 0 {
			count = len(links)
			approvetx++
		}
		if approvetx > 3 {
			return nil
		}
	}
	return particle
}

var weightedSelFn map[string]func([]*Node) *Node = map[string]func([]*Node) *Node{
	DAG_ALGO_MCMCP.Type():  weightedSelectMCMCP,
	DAG_ALGO_MCMCPP.Type(): weightedSelectMCMCPP,
	DAG_ALGO_RANDOM.Type(): weightedSelectRandom,
}

func weightedSelect(approvers []*Node) *Node {
	fn, ok := weightedSelFn[dagConfig.Algorithm]
	if ok {
		return fn(approvers)
	}
	return nil
}

func weightedSelectRandom(approvers []*Node) *Node {
	// need to do process in ascending order
	_sort_(approvers)
	weights := calculateTxWeights(approvers)
	r := _r_.NormFloat64()/12 + 0.5 // -3 - 3 range / 12 gives us +/-0.25 + 0.5
	x := 0.0
	for idx, v := range weights {
		if r >= x && r <= v {
			return approvers[idx]
		}
		x += weights[idx]
	}
	// if we miss, randomly return one of the approver nodes
	return approvers[rand.Intn(len(approvers))]
}

func weightedSelectMCMCP(approvers []*Node) *Node {
	_shuffle_(approvers)
	weights := calculateTxWeights(approvers)
	mean, _ := stats.Mean(weights)
	stdiv, _ := stats.StandardDeviation(weights)
	r := _r_.NormFloat64()*stdiv + mean
	if len(weights) > 0 {
		cumSum := weights[0]
		for i := 1; i < len(approvers); i++ {
			if r < cumSum {
				return approvers[i-1]
			}
			cumSum = weights[i]
		}
	}
	if len(approvers) > 0 {
		return approvers[len(approvers)-1]
	} else {
		return nil
	}
}

func weightedSelectMCMCPP(approvers []*Node) *Node {
	_sort_(approvers)
	weights := calculateTxWeights(approvers)
	mean, _ := stats.Mean(weights)
	stdiv, _ := stats.StandardDeviation(weights)
	rv := _r_.NormFloat64()*stdiv + mean
	x := 0.0
	for idx, v := range weights {
		if rv >= x && rv <= v {
			return approvers[idx]
		}
		x += v
	}
	// did not find a suitable node? pick randomly
	return approvers[rand.Intn(len(approvers))]
}
