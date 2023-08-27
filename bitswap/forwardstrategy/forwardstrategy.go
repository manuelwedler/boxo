package forwardstrategy

import (
	"math/rand"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	qpeerset "github.com/libp2p/go-libp2p-kad-dht/qpeerset"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

var log = logging.Logger("bs:fstrat")

// Interface for implementing different forwarding strategies.
type ForwardStrategy interface {
	// Selects one peer of the set according to the strategy.
	SelectPeer(peers map[peer.ID]struct{}, c cid.Cid) peer.ID
}

type RandomForward struct{}

func NewRandomForward() *RandomForward {
	return &RandomForward{}
}

func (rf *RandomForward) SelectPeer(peers map[peer.ID]struct{}, c cid.Cid) peer.ID {
	rand.Seed(time.Now().UnixNano())
	perm := rand.Perm(len(peers))

	available := make([]peer.ID, 0, len(peers))
	for p := range peers {
		available = append(available, p)
	}
	log.Debugw("rf SelectPeer", "peers", available, "cid", c)

	if len(available) == 0 {
		return peer.ID("")
	}

	return available[perm[0]]
}

type CloseCidForward struct{}

func NewCloseCidForward() *CloseCidForward {
	return &CloseCidForward{}
}

func (ccf *CloseCidForward) SelectPeer(peers map[peer.ID]struct{}, c cid.Cid) peer.ID {
	log.Debugw("ccf SelectPeer", "peers", peers, "cid", c)
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
