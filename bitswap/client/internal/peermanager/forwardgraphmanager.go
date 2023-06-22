package peermanager

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	qpeerset "github.com/libp2p/go-libp2p-kad-dht/qpeerset"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

const (
	protectionTag            = "bs-fgw"
	reconstructGraphDelay    = time.Minute * 9  // todo / what makes sense?
	initialConstructionDelay = time.Second * 10 // todo / evaluate different values here
)

// PeerTagger is an interface for tagging peers with metadata
type PeerTagger interface {
	Protect(peer.ID, string)
	Unprotect(peer.ID, string) bool
}

// forwardGraphManager establishes a privacy subgraph by
// keeping record of logical connections derived from the
// main graph.
// It approximates a directed regular graph with a specified
// degree.
type forwardGraphManager struct {
	// Degree of the approximated regular graph.
	degree uint64

	// Strategy to select a successor when forwarding.
	strategy ForwardStrategy

	// Set of all connected peers
	connectedPeers map[peer.ID]struct{}

	// Set of successors of this peer.
	successors map[peer.ID]struct{}

	peerTagger            PeerTagger
	reconstructGraphTimer *time.Timer

	lk sync.RWMutex
}

// newForwardGraphManager creates a new forwardGraphManager
// with a specified degree for the privacy subgraph.
func newForwardGraphManager(
	ctx context.Context,
	degree uint64,
	forwardStrategy ForwardStrategy,
	peerTagger PeerTagger) *forwardGraphManager {
	fgm := &forwardGraphManager{
		degree:         degree,
		strategy:       forwardStrategy,
		connectedPeers: make(map[peer.ID]struct{}),
		successors:     make(map[peer.ID]struct{}),
		peerTagger:     peerTagger,
	}

	go fgm.run(ctx)

	return fgm
}

func (fgm *forwardGraphManager) run(ctx context.Context) {
	fgm.reconstructGraphTimer = time.NewTimer(initialConstructionDelay)

	for {
		select {
		case <-fgm.reconstructGraphTimer.C:
			fgm.SelectNewSuccessors()
		case <-ctx.Done():
			fgm.reconstructGraphTimer.Stop()
			return
		}
	}
}

func (fgm *forwardGraphManager) useCompleteGraph() bool {
	return fgm.degree == math.MaxInt64
}

func (fgm *forwardGraphManager) successorTarget() int {
	return int(math.Floor(float64(fgm.degree) / 2.0))
}

// Returns a successor chosen by the used strategy.
func (fgm *forwardGraphManager) GetSuccessorByStrategy(c cid.Cid) peer.ID {
	fgm.lk.RLock()
	defer fgm.lk.RUnlock()

	var successors map[peer.ID]struct{}
	if fgm.useCompleteGraph() {
		successors = fgm.connectedPeers
	} else {
		successors = fgm.successors
	}

	return fgm.strategy.selectPeer(successors, c)
}

// Replaces all successors with newly chosen ones.
func (fgm *forwardGraphManager) SelectNewSuccessors() {
	fgm.lk.Lock()
	defer fgm.lk.Unlock()

	if fgm.useCompleteGraph() {
		return
	}
	for s := range fgm.successors {
		fgm.removeSuccessor(s)
	}
	foundNew := true
	for foundNew == true && len(fgm.successors) < fgm.successorTarget() {
		foundNew = fgm.findOneNewSuccessor()
	}

	if !fgm.reconstructGraphTimer.Stop() {
		<-fgm.reconstructGraphTimer.C
	}
	fgm.reconstructGraphTimer.Reset(reconstructGraphDelay)
}

// Randomly selects one peer among the connected to be a successor.
func (fgm *forwardGraphManager) findOneNewSuccessor() bool {
	if fgm.useCompleteGraph() {
		return false
	}
	available := make([]peer.ID, 0)
	for p := range fgm.connectedPeers {
		if _, isSuccessor := fgm.successors[p]; !isSuccessor {
			available = append(available, p)
		}
	}
	if len(available) == 0 {
		return false
	}
	rand.Seed(time.Now().UnixNano())
	perm := rand.Perm(len(available))
	chosen := available[perm[0]]
	fgm.addSuccessor(chosen)
	return true
}

// Called when we connect to a new peer.
func (fgm *forwardGraphManager) AddPeer(p peer.ID) {
	fgm.lk.Lock()
	defer fgm.lk.Unlock()

	fgm.connectedPeers[p] = struct{}{}
	if fgm.useCompleteGraph() {
		return
	}
	if len(fgm.successors) < fgm.successorTarget() {
		fgm.findOneNewSuccessor()
	}
}

// Called when we disconnect from a peer.
func (fgm *forwardGraphManager) RemovePeer(p peer.ID) {
	fgm.lk.Lock()
	defer fgm.lk.Unlock()

	delete(fgm.connectedPeers, p)
	if fgm.useCompleteGraph() {
		return
	}
	if _, isSuccessor := fgm.successors[p]; isSuccessor {
		fgm.removeSuccessor(p)
		fgm.findOneNewSuccessor()
	}
}

func (fgm *forwardGraphManager) addSuccessor(p peer.ID) {
	fgm.peerTagger.Protect(p, protectionTag)
	fgm.successors[p] = struct{}{}
}

func (fgm *forwardGraphManager) removeSuccessor(p peer.ID) {
	fgm.peerTagger.Unprotect(p, protectionTag)
	delete(fgm.successors, p)
}

// Interface for implementing different forwarding strategies.
type ForwardStrategy interface {
	// Selects one peer of the set according to the strategy.
	selectPeer(peers map[peer.ID]struct{}, c cid.Cid) peer.ID
}

type RandomForward struct{}

func NewRandomForward() *RandomForward {
	return &RandomForward{}
}

func (rf *RandomForward) selectPeer(peers map[peer.ID]struct{}, c cid.Cid) peer.ID {
	rand.Seed(time.Now().UnixNano())
	perm := rand.Perm(len(peers))

	available := make([]peer.ID, 0, len(peers))
	for p := range peers {
		available = append(available, p)
	}

	if len(available) == 0 {
		return peer.ID("")
	}

	return available[perm[0]]
}

type CloseCidForward struct{}

func NewCloseCidForward() *CloseCidForward {
	return &CloseCidForward{}
}

func (ccf *CloseCidForward) selectPeer(peers map[peer.ID]struct{}, c cid.Cid) peer.ID {
	set := qpeerset.NewQueryPeerset(c.String())
	for p := range peers {
		set.TryAdd(p, peer.ID(""))
	}
	result := set.GetClosestNInStates(1, qpeerset.PeerHeard)

	if len(result) == 0 {
		return peer.ID("")
	}

	return result[0]
}
