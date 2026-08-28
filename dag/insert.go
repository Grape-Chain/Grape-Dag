package dag

import (
	"time"

	"github.com/Grape-Chain/Grape-Dag/tx"
	"github.com/Grape-Chain/Grape-Dag/utils"
	"github.com/google/uuid"
	"github.com/ledongthuc/goterators"
	"github.com/pkg/errors"
)

func (d *Dag) InsertIfNotExist(vertices []*Node) error {
	goterators.ForEach(vertices, func(v *Node) {
		if x := d.getById(v.id.id, false); x == nil {
			err := d.insertMissing(v)
			if err != nil {
				logger.Errorf("Failed to insert a missing site: %s", err.Error())
			}
		}
	})
	return nil
}

func (d *Dag) unsafe_bypassInsert(vertices []*Node) (*Dag, error) {
	var err error
	for _, n := range vertices {
		if d, err = d.addToDag(n, nil); err != nil {
			return d, errors.Errorf("Failed to bypass insert Tx %s into dag. %s", n.id.id.String(), err.Error())
		}
	}
	return d, nil
}

// InsertTxDag - insert a new site(vertex[aka node]) into dag, given its relative location
// based on the ids of the sites this site approves in other nodes; Hence, this site
// should locate the approvees (this site in the original node) that were approved by this site
// in the original node
func (dag *Dag) InsertTxDag(
	inVertex *Node,
	txId uuid.UUID,
	idMajor uint64,
	idMinor uint32,
	ids ...tx.UuidSlice, // this is a slice of ids for the sites that this site(vertex) has approved elsewhere
) error {
	dag.mux.Lock()
	defer dag.mux.Unlock()

	// When creating a site/vertex from a transaction, the versioning information does not get preserved
	// restore it here
	inVertex.id.id = txId
	inVertex.id.idMajor = idMajor
	inVertex.id.idMinor = idMinor
	inVertex.time = time.Now()
	// Check if this node is already in our dag
	// if v := dag.getById(inVertex.id.id, true); v == nil {
	// 	// if cache lookup failed, do a more thorough search
	// 	if _, i, err := goterators.Find(dag._dag_, func(n *Node) bool {
	// 		return n.Equal(inVertex)
	// 	}); err == nil && i >= 0 {
	// 		// this node is already in our dag - Ignore
	// 		return nil
	// 	}
	// } else {
	// 	// this vertex is already in our version of dag; skip out
	// 	return nil
	// }

	if v := dag.getById(inVertex.id.id, true); v != nil {
		return nil
	}

	// Vertex(node) to insert may be missing sites in the current version of the dag
	// this is the result of this node missing previous tx (out of sync)
	// wait till the next pin tx, and check if the tx after it can be
	// linked to either dag or pin tx
	candidates := []*Node{}
	settledIds := []uuid.UUID{}
	idsNotFound := []uuid.UUID{}
	for _, id := range ids {
		// first, check dag cache (and more thorough search if required)
		// we are looking for the sites/vertices that this site/vertex has
		// already approved elsewhere
		if n := dag.getById(id.Id, true); n != nil {
			candidates = append(candidates, n)
		} else if _, ok := settledSite(id.Id); ok {
			// The approved site has been settled by a commit transaction. It is
			// resolved - not missing - but it gets no edge: keeping one would
			// pull a settled site back into the live graph and stop it, and its
			// neighbours, from ever being collected.
			settledIds = append(settledIds, id.Id)
		} else if n = _pins_.getById(id.Id); n != nil {
			candidates = append(candidates, n)
		} else {
			idsNotFound = append(idsNotFound, id.Id)
		}
	}
	if len(settledIds) > 0 {
		inVertex.slicedTargets = append(inVertex.slicedTargets, settledIds...)
	}
	// let's see if we have the approvees [aka candidates for local approval]
	if len(candidates)+len(settledIds) != len(ids) {
		// In order to relink later, need to preserve target Ids
		// store in dag without trying to approve target sites it references
		// this node
		// so, the current version of dag does not have the sites referenced by inVertex site
		goterators.ForEach(idsNotFound, func(id uuid.UUID) {
			if inVertex.missingTargets == nil {
				inVertex.missingTargets = make(map[string]bool)
			}
			inVertex.missingTargets[id.String()] = true
			logger.Warnf("Tx %s is missing from this DAG. Requesting...", id.String())
		})
		if _, err := dag.addToDag(inVertex, candidates); err != nil {
			return errors.Errorf("Failed to insert Tx %s into dag. %s", inVertex.id.id.String(), err.Error())
		}
		logger.Warn("Local DAG is out of sync. Synchronization is underway...")

		// @TODO: add to queue until a chain can be built and inserted
		return errors.Errorf("Tx %s is missing source Txs: %q", inVertex.id.id.String(), idsNotFound)
	}

	// Find versions and adjust this inVertex version if required
	var nextToNode *Node = nil
	if dagConfig.Versioncollision {
		// look at the candidates for approval
		approvers := []*Node{}
		// candidates - the sites/vertices that this inVertex approved elsewhere
		goterators.ForEach(candidates, func(targetNode *Node) {
			// for the candidates get all approvers that approve them
			approvers = append(approvers, getApprovers(dag._links_, targetNode)...)
		})
		goterators.ForEach(approvers, func(candidateNode *Node) {
			// only makes sense when two vertises are different
			if candidateNode.id.id != inVertex.id.id {
				if candidateNode.id.idMajor == inVertex.id.idMajor {
					if candidateNode.id.idMinor >= inVertex.id.idMinor {
						idMajor = candidateNode.id.idMajor
						idMinor = candidateNode.id.idMinor + 1
						nextToNode = candidateNode
					}
				}
			}
		})
	}
	// if there is conflicting versioning, update this vertex's versioning
	if nextToNode != nil {
		if nextToNode.id.id != inVertex.id.id {
			return errors.Errorf("Tx %s is missing from this DAG. @TODO: Add to queue. Error for now", inVertex.id.id.String())
		}
		utils.ColorizeInfo(logger, "[*] Resolving DAG node versioning conflict: v.%d.%d", idMajor, idMinor)
		// Update the sourceNode's ids
		inVertex.id.idMajor = idMajor
		inVertex.id.idMinor = idMinor
		logger.Warnf("[*] Update uuid:%s idMajor:%d, idMinor:%d",
			inVertex.id.id.String(), inVertex.id.idMajor, inVertex.id.idMinor)

		_, idx, err := goterators.Find(dag._dag_, func(candidate *Node) bool {
			return candidate.id.id == nextToNode.id.id
		})
		if err == nil {
			// insert if there is conflict
			if idx < len(dag._dag_)-1 {
				dag._dag_ = append(dag._dag_[:idx+1], dag._dag_[idx:]...)
				dag._dag_[idx+1] = inVertex
			} else {
				_, err := dag.addToDag(inVertex, candidates)
				if err != nil {
					return err
				}
			}
		} else {
			_, err := dag.addToDag(inVertex, candidates)
			if err != nil {
				return err
			}
		}
	} else {
		_, err := dag.addToDag(inVertex, candidates)
		if err != nil {
			return err
		}
	}
	// after the new node has been added to dag, update links
	goterators.ForEach(candidates, func(targetNode *Node) {
		// link to these candidates - there should be at least one or ideally, two candidates
		dag._links_ = append(dag._links_, Link{source: inVertex, target: targetNode})
		logger.Debugf("[SUB LINK] %s|%d|%d ==> %s|%d|%d",
			inVertex.id.id.String(), inVertex.id.idMajor, inVertex.id.idMinor,
			targetNode.id.id.String(), targetNode.id.idMajor, targetNode.id.idMinor,
		)
	})
	// Whoever built this site claimed it, and the claim is checkable: the
	// signature covers the site's id, its transaction and its approvals. Checked
	// here, after the links are established, because the approvals are part of
	// what was signed.
	//
	// A site with no attribution at all is accepted - it came from a node
	// predating attribution, and it simply earns nobody a fee. A site whose
	// attribution does not verify is kept as well, but stripped: the site is
	// still a valid part of the graph and refusing it would let a bad claim
	// deny the network a transaction, whereas keeping the claim would let the
	// liar be paid.
	if err := verifyProcessor(inVertex); err != nil {
		if !errors.Is(err, ErrNoProcessorAttribution) {
			logger.Warnf("[attribution] Site %s carries an unusable claim, dropping it: %s",
				inVertex.id.id.String(), err.Error())
			clearProcessor(inVertex)
		}
	}

	// Cumulative weights are no longer recomputed here either; see dag/weights.go.
	// dag.dag = updateFwdWeights(dag.dag, dag.links)
	if traceSites {
		dag.logLast("INSERT:", inVertex, 1)
	}
	return nil
}
