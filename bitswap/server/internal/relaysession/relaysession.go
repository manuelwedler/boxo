package relaysession

import (
	"context"
	"sync"

	cid "github.com/ipfs/go-cid"
	exchange "github.com/ipfs/go-ipfs-exchange-interface"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p-core/peer"
)

var log = logging.Logger("relaysession")

type RelayRegistry struct {
	r      map[cid.Cid]map[peer.ID]int32
	Degree int32
	lk     sync.RWMutex
}

type RelaySession struct {
	Session  exchange.Fetcher
	Registry *RelayRegistry
	// We could add here the PeerBlockRegistry to start making decisions
	// also with the information tracked from want messages.
	// PeerBlockRegistry
}

func NewRelaySession(degree int32) *RelaySession {
	return &RelaySession{
		Session: nil,
		Registry: &RelayRegistry{
			r:      make(map[cid.Cid]map[peer.ID]int32, 0),
			Degree: degree,
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

	for _, t := range kt.T {
		if rs.Registry.r[t.Key] == nil {
			// We need to start a new search because the CID is not active.
			sessionBlks = append(sessionBlks, t.Key)
			// Add to the registry
			rs.Registry.r[t.Key] = make(map[peer.ID]int32, 0)
		}
		// Update with the latest TTL (this could be changed to perform
		// smarter logics over TTL).
		rs.Registry.r[t.Key][kt.Peer] = t.TTL
	}
	log.Debugf("Started new block search for keys: %v", sessionBlks)
	rs.Session.GetBlocks(ctx, sessionBlks)
}

// InterestedPeers returns peer looking for a cid.
func (rs *RelaySession) InterestedPeers(c cid.Cid) map[peer.ID]int32 {
	rs.Registry.lk.RLock()
	defer rs.Registry.lk.RUnlock()
	// Create brand new map to avoid data races.
	res := make(map[peer.ID]int32)
	for k, v := range rs.Registry.r[c] {
		res[k] = v
	}
	return res
}

// GetTTL returns peer looking for a cid.
func (rs *RelayRegistry) GetTTL(c cid.Cid) int32 {
	rs.lk.RLock()
	defer rs.lk.RUnlock()
	// TODO: We return the TTL for any of the peers requesting it. We could
	// perform other logic to select the right TTL to send here.
	for _, ttl := range rs.r[c] {
		return ttl
	}

	return 0
}

// func (rs *RelaySession)
type itemTrack struct {
	Key cid.Cid
	TTL int32
}
type keyTracker struct {
	Peer peer.ID
	T    []*itemTrack
}

func (kt *keyTracker) UpdateTracker(c cid.Cid, ttl int32) {
	kt.T = append(kt.T, &itemTrack{c, ttl})
}

func NewKeyTracker(p peer.ID) *keyTracker {
	return &keyTracker{
		T:    make([]*itemTrack, 0),
		Peer: p,
	}
}
