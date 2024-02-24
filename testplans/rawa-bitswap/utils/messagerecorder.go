package utils

import (
	"context"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/peer"
	bsmsg "github.com/manuelwedler/boxo/bitswap/message"
	bsnet "github.com/manuelwedler/boxo/bitswap/network"
)

type MessageRecorder struct {
	lk sync.RWMutex

	Spys map[peer.ID]struct{}

	ObservedCids    map[cid.Cid]struct{}
	FirstCidForPeer map[peer.ID]cid.Cid
}

func NewMessageRecorder(spys map[peer.ID]struct{}) *MessageRecorder {
	recorder := &MessageRecorder{
		Spys: spys,

		ObservedCids:    make(map[cid.Cid]struct{}),
		FirstCidForPeer: make(map[peer.ID]cid.Cid),
	}

	return recorder
}

func (mr *MessageRecorder) ReceiveMessage(_ context.Context, sender peer.ID, incoming bsmsg.BitSwapMessage) {
	mr.lk.Lock()
	defer mr.lk.Unlock()

	// ignore messages from spys
	if _, ok := mr.Spys[sender]; ok {
		return
	}

	for _, e := range incoming.Wantlist() {
		mr.observed(sender, e.Cid)
	}
	for _, e := range incoming.Forwardlist() {
		mr.observed(sender, e.Cid)
	}
}

func (mr *MessageRecorder) observed(sender peer.ID, c cid.Cid) {
	mr.ObservedCids[c] = struct{}{}
	if _, ok := mr.FirstCidForPeer[sender]; !ok {
		mr.FirstCidForPeer[sender] = c
	}
}

func (mr *MessageRecorder) ReceiveError(error) {}

func (mr *MessageRecorder) PeerConnected(peer.ID) {}

func (mr *MessageRecorder) PeerDisconnected(peer.ID) {}

// ensure MessageRecorder satisfies interface
var _ bsnet.Receiver = &MessageRecorder{}
