package forwardstrategy

import (
	"github.com/ipfs/go-cid"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

// STUB FOR COMPATIBILITY WITH THE RAWA-BITSWAP TESTPLAN

type ForwardStrategy interface {
	SelectPeer(peers map[peer.ID]struct{}, c cid.Cid) peer.ID
}

type RandomForward struct{}

func NewRandomForward() *RandomForward {
	return &RandomForward{}
}

func (rf *RandomForward) SelectPeer(peers map[peer.ID]struct{}, c cid.Cid) peer.ID {
	return peer.ID("")
}

type CloseCidForward struct{}

func NewCloseCidForward() *CloseCidForward {
	return &CloseCidForward{}
}

func (ccf *CloseCidForward) SelectPeer(peers map[peer.ID]struct{}, c cid.Cid) peer.ID {
	return peer.ID("")
}
