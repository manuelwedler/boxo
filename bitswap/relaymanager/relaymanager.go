package relaymanager

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	exchange "github.com/ipfs/boxo/exchange"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

var log = logging.Logger("relaymanager")

const (
	// Default probability to go into proxy phase instead of forwarding
	// when receiving a want-forward
	defaultProxyPhaseTransitionProbability = 0.2
)

type ForwardSender interface {
	// ForwardWants sends want-forwards to one connected peer
	ForwardWants(context.Context, []cid.Cid)
	// ForwardHaves sends forward-haves to a specified peer.
	ForwardHaves(ctx context.Context, to peer.ID, have cid.Cid, peers []peer.ID)
}

// CreateProxySession initializes a proxy session with the given context.
type CreateProxySession func(ctx context.Context, proxyDiscoveryCallback ProxyDiscoveryCallback) exchange.Fetcher

// ProxyDiscoveryCallback is called when a proxy session discovers peers for a CID
type ProxyDiscoveryCallback func(peer.ID, cid.Cid)

// PeerTagger is an interface for tagging peers with metadata
type PeerTagger interface {
	Protect(peer.ID, string)
	Unprotect(peer.ID, string) bool
}

func getConnectionProtectionTag(id cid.Cid) string {
	return fmt.Sprint("bs-rel-", id)
}

type RelayManager struct {
	Forwarder           ForwardSender
	CreateProxySession  CreateProxySession
	Ledger              *RelayLedger
	ProxyTransitionProb float64
	peerTagger          PeerTagger
	self                peer.ID
}

func NewRelayManager(peerTagger PeerTagger, self peer.ID) *RelayManager {
	return &RelayManager{
		Forwarder:          nil,
		CreateProxySession: nil,
		Ledger: &RelayLedger{
			items: make(map[cid.Cid]map[peer.ID]bool, 0),
		},
		ProxyTransitionProb: defaultProxyPhaseTransitionProbability,
		peerTagger:          peerTagger,
		self:                self,
	}
}

// ProcessForwards randomly decides for each cid either to forward it or start a
// proxy session. For the random decision ProxyTransitionProb is used.
// The relayledger is updated with the cids.
func (rm *RelayManager) ProcessForwards(ctx context.Context, kt *keyTracker) {
	rm.Ledger.Update(kt)
	forwards := kt.T

	rand.Seed(time.Now().UnixNano())
	for _, c := range forwards {
		rm.peerTagger.Protect(kt.Peer, getConnectionProtectionTag(c))

		rnd := rand.Float64()
		if rnd <= rm.ProxyTransitionProb {
			proxyDiscoveryCallback := func(provider peer.ID, received cid.Cid) {
				if received != c {
					log.Debugf("[recv] cid not equal proxy cid; cid=%s, peer=%s, proxycid=%s", received, provider, c)
					return
				}
				rm.RelayHaves(ctx, kt.Peer, c, []peer.ID{provider})
			}

			session := rm.CreateProxySession(ctx, proxyDiscoveryCallback)
			session.GetBlocks(ctx, []cid.Cid{c})
		} else {
			rm.Forwarder.ForwardWants(ctx, []cid.Cid{c})
		}
	}
}

// ProcessForwardHaves relays the forward-haves to all interested peers in the ledger
func (rm *RelayManager) ProcessForwardHaves(ctx context.Context, forwardHaves map[cid.Cid][]peer.ID) {
	for c, ps := range forwardHaves {
		interested := rm.Ledger.InterestedPeers(c)
		for _, to := range interested {
			rm.RelayHaves(ctx, to, c, ps)
		}
	}
}

func (rm *RelayManager) RelayHaves(ctx context.Context, to peer.ID, have cid.Cid, peers []peer.ID) {
	rm.Forwarder.ForwardHaves(ctx, to, have, peers)
	// For now, we just unprotect the connection when the first response is sent.
	// As later responses could be pruned, a more sophisticated approach might be worth it.
	rm.peerTagger.Unprotect(to, getConnectionProtectionTag(have))
}

type RelayLedger struct {
	items map[cid.Cid]map[peer.ID]bool
	lk    sync.RWMutex
}

// BlockSeen removes interest in relayManager if a peer interested have sent
// already the block to prevent from forwarding to it again.
func (rl *RelayLedger) BlockSeen(c cid.Cid, p peer.ID) {
	// RemoveInterest from this peer becaues he has sent the block for that CID (avoid resending)
	rl.RemoveInterest(c, p)
}

// RemoveInterest removes interest for a CID from a peer from the registry.
func (rl *RelayLedger) RemoveInterest(c cid.Cid, p peer.ID) {
	rl.lk.Lock()
	defer rl.lk.Unlock()

	delete(rl.items[c], p)
	if len(rl.items[c]) == 0 {
		delete(rl.items, c)
	}
}

func (rl *RelayLedger) Update(kt *keyTracker) {
	rl.lk.Lock()
	defer rl.lk.Unlock()

	for _, c := range kt.T {
		if rl.items[c] == nil {
			// Add to the registry
			rl.items[c] = make(map[peer.ID]bool, 1)
		}
		rl.items[c][kt.Peer] = true
	}
}

// InterestedPeers returns peer looking for a cid.
func (rl *RelayLedger) InterestedPeers(c cid.Cid) []peer.ID {
	rl.lk.RLock()
	defer rl.lk.RUnlock()
	// Create brand new map to avoid data races.
	res := make([]peer.ID, 0, len(rl.items[c]))
	for p, ok := range rl.items[c] {
		if ok {
			res = append(res, p)
		}
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
