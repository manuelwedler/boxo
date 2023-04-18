package relaysession

import (
	"context"
	"sync"

	cid "github.com/ipfs/go-cid"
	exchange "github.com/ipfs/go-ipfs-exchange-interface"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

var log = logging.Logger("relaysession")

type RelayRegistry struct {
	r  map[cid.Cid]map[peer.ID]bool
	lk sync.RWMutex
}

type RelaySession struct {
	Session  exchange.Fetcher
	Registry *RelayRegistry
	// We could add here the PeerBlockRegistry to start making decisions
	// also with the information tracked from want messages.
	// PeerBlockRegistry
}

func NewRelaySession() *RelaySession {
	return &RelaySession{
		Session: nil,
		Registry: &RelayRegistry{
			r: make(map[cid.Cid]map[peer.ID]bool, 0),
		},
	}
}

// BlockSeen removes interest in relaySession if a peer interested have sent
// already the block to prevent from forwarding to it again.
func (rs *RelaySession) BlockSeen(c cid.Cid, p peer.ID) {
	// RemoveInterest from this peer becaues he has sent the block for that CID (avoid resending)
	rs.RemoveInterest(c, p)
}

// RemoveInterest removes interest for a CID from a peer from the registry.
func (rs *RelaySession) RemoveInterest(c cid.Cid, p peer.ID) {
	rs.Registry.lk.Lock()
	defer rs.Registry.lk.Unlock()

	delete(rs.Registry.r[c], p)
	if len(rs.Registry.r[c]) == 0 {
		delete(rs.Registry.r, c)
	}
}

func (rs *RelaySession) UpdateSession(ctx context.Context, kt *keyTracker) {
	rs.Registry.lk.Lock()
	defer rs.Registry.lk.Unlock()
	sessionBlks := make([]cid.Cid, 0)

	for _, c := range kt.T {
		if rs.Registry.r[c] == nil {
			// We need to start a new search because the CID is not active.
			sessionBlks = append(sessionBlks, c)
			// Add to the registry
			rs.Registry.r[c] = make(map[peer.ID]bool, 1)
		}
		rs.Registry.r[c][kt.Peer] = true
	}
	log.Debugf("Started new block search for keys: %v", sessionBlks)
	rs.Session.GetBlocks(ctx, sessionBlks) // TODO GetBlocks should get called for every request to have a random walk.
}

// InterestedPeers returns peer looking for a cid.
func (rs *RelaySession) InterestedPeers(c cid.Cid) map[peer.ID]bool {
	rs.Registry.lk.RLock()
	defer rs.Registry.lk.RUnlock()
	// Create brand new map to avoid data races.
	res := make(map[peer.ID]bool)
	for k, v := range rs.Registry.r[c] {
		res[k] = v
	}
	return res
}

type keyTracker struct {
	Peer peer.ID
	T    []cid.Cid
}

func (kt *keyTracker) UpdateTracker(c cid.Cid) {
	kt.T = append(kt.T, c)
}

func NewKeyTracker(p peer.ID) *keyTracker {
	return &keyTracker{
		T:    make([]cid.Cid, 0),
		Peer: p,
	}
}
