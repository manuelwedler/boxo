package relaymanager

import (
	"context"
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
}

// CreateProxySession initializes a proxy session with the given context.
type CreateProxySession func(ctx context.Context, proxyDiscoveryCallback ProxyDiscoveryCallback) exchange.Fetcher

// ProxyDiscoveryCallback is called when a proxy session discovers peers for a CID
type ProxyDiscoveryCallback func(peer.ID, cid.Cid)

type RelayManager struct {
	Forwarder           ForwardSender
	CreateProxySession  CreateProxySession
	Ledger              *RelayLedger
	proxyTransitionProb float64
}

func NewRelayManager() *RelayManager {
	return &RelayManager{
		Forwarder:          nil,
		CreateProxySession: nil,
		Ledger: &RelayLedger{
			r: make(map[cid.Cid]map[peer.ID]bool, 0),
		},
		proxyTransitionProb: defaultProxyPhaseTransitionProbability,
	}
}

// ProcessForwards randomly decides for each cid either to forward it or start a
// proxy session. For the random decision proxyTransitionProb is used.
// The relayledger is updated with the cids.
func (rm *RelayManager) ProcessForwards(ctx context.Context, kt *keyTracker) {
	rm.Ledger.Update(kt)
	forwards := kt.T

	rand.Seed(time.Now().UnixNano())
	for _, c := range forwards {
		rnd := rand.Float64()

		if rnd <= rm.proxyTransitionProb {
			proxyDiscoveryCallback := func(provider peer.ID, received cid.Cid) {
				if received != c {
					log.Debugf("[recv] cid not equal proxy cid; cid=%s, peer=%s, proxycid=%s", received, provider, c)
					return
				}
				// todo / should be non-blocking
				// TODO / send forward-have for c to kt.peer
			}
			session := rm.CreateProxySession(ctx, proxyDiscoveryCallback)
			session.GetBlocks(ctx, []cid.Cid{c})
		} else {
			rm.Forwarder.ForwardWants(ctx, []cid.Cid{c})
		}
	}
}

type RelayLedger struct {
	r  map[cid.Cid]map[peer.ID]bool
	lk sync.RWMutex
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

	delete(rl.r[c], p)
	if len(rl.r[c]) == 0 {
		delete(rl.r, c)
	}
}

func (rl *RelayLedger) Update(kt *keyTracker) {
	rl.lk.Lock()
	defer rl.lk.Unlock()

	for _, c := range kt.T {
		if rl.r[c] == nil {
			// Add to the registry
			rl.r[c] = make(map[peer.ID]bool, 1)
		}
		rl.r[c][kt.Peer] = true
	}
}

// InterestedPeers returns peer looking for a cid.
func (rl *RelayLedger) InterestedPeers(c cid.Cid) map[peer.ID]bool {
	rl.lk.RLock()
	defer rl.lk.RUnlock()
	// Create brand new map to avoid data races.
	res := make(map[peer.ID]bool)
	for k, v := range rl.r[c] {
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
