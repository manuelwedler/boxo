package relaymanager

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"
	"sync"
	"time"

	exchange "github.com/ipfs/boxo/exchange"
	cid "github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p/core/peer"
	ks "github.com/whyrusleeping/go-keyspace"
)

var log = logging.Logger("relaymanager")

const (
	// Default probability to go into proxy phase instead of forwarding
	// when receiving a want-forward
	defaultProxyPhaseTransitionProbability = 0.2
)

type ForwardSender interface {
	// ForwardWants sends want-forwards to one connected peer
	ForwardWants(ctx context.Context, c cid.Cid, exclude []peer.ID) (peer.ID, error)
	// ForwardWantsTo sends want-forwards to a specific peer
	ForwardWantsTo(ctx context.Context, c cid.Cid, to peer.ID) error
	// ForwardHaves sends forward-haves to a specified peer.
	ForwardHaves(ctx context.Context, to peer.ID, have cid.Cid, peers []peer.AddrInfo)
}

// CreateProxySession initializes a proxy session with the given context.
type CreateProxySession func(ctx context.Context, proxyDiscoveryCallback ProxyDiscoveryCallback) exchange.Fetcher

// ProxyDiscoveryCallback is called when a proxy session discovers peers for a CID
type ProxyDiscoveryCallback func(provider peer.AddrInfo, c cid.Cid, seesionClosed bool)

// PeerTagger is an interface for tagging peers with metadata
type PeerTagger interface {
	Protect(peer.ID, string)
	Unprotect(peer.ID, string) bool
}

func getConnectionProtectionTag(id cid.Cid) string {
	return fmt.Sprint("bs-rel-", id.String())
}

type RelayManager struct {
	Forwarder           ForwardSender
	CreateProxySession  CreateProxySession
	ProxyTransitionProb float64
	peerTagger          PeerTagger
	self                peer.ID

	// cid -> want source -> relayed to
	fromMap map[cid.Cid]map[peer.ID]peer.ID
	// cid -> want source -> proxied (when transitioned to proxy phase)
	proxyMap map[cid.Cid]map[peer.ID]bool
	// cid -> relayed to -> want source
	toMap map[cid.Cid]map[peer.ID]peer.ID
	lk    sync.RWMutex

	ProxyDistances []*big.Int
}

func NewRelayManager(peerTagger PeerTagger, self peer.ID) *RelayManager {
	return &RelayManager{
		Forwarder:           nil,
		CreateProxySession:  nil,
		ProxyTransitionProb: defaultProxyPhaseTransitionProbability,
		peerTagger:          peerTagger,
		self:                self,

		fromMap:        make(map[cid.Cid]map[peer.ID]peer.ID),
		proxyMap:       make(map[cid.Cid]map[peer.ID]bool),
		toMap:          make(map[cid.Cid]map[peer.ID]peer.ID),
		ProxyDistances: make([]*big.Int, 0),
	}
}

// ProcessForwards randomly decides for each cid either to forward it or start a
// proxy session. For the random decision ProxyTransitionProb is used.
// The relayledger is updated with the cids.
func (rm *RelayManager) ProcessForwards(ctx context.Context, kt *keyTracker) {
	rm.lk.Lock()
	defer rm.lk.Unlock()

	forwards := kt.T
	from := kt.Peer
	rand.Seed(time.Now().UnixNano())
	for _, c := range forwards {
		rm.peerTagger.Protect(from, getConnectionProtectionTag(c))
		if _, ok := rm.fromMap[c]; !ok {
			rm.fromMap[c] = make(map[peer.ID]peer.ID, 1)
		}
		if _, ok := rm.proxyMap[c]; !ok {
			rm.proxyMap[c] = make(map[peer.ID]bool, 1)
		}
		if _, ok := rm.toMap[c]; !ok {
			rm.toMap[c] = make(map[peer.ID]peer.ID, 1)
		}

		// If we are already the proxy, the session is still running
		if proxied, ok := rm.proxyMap[c][from]; ok && proxied {
			log.Debugf("processing forwards: already proxy for the combination, ignore; cid=%s, from=%s", c, from)
			continue
		}
		// Already relayed this cid from this peer
		if to, ok := rm.fromMap[c][from]; ok {
			// Send want-forward again to the same receiver
			log.Debugf("processing forwards: duplicate forward, sending to same peer; cid=%s, from=%s, to=%s", c, from, to)
			err := rm.Forwarder.ForwardWantsTo(ctx, c, to)
			if err != nil {
				log.Debugf("processing forwards: (same peer) forward error; starting proxy phase; cid=%s, from=%s, err=%s", c, from, err)
				rm.startProxyPhase(ctx, from, c)
				rm.proxyMap[c][from] = true
			}
			continue
		}

		rnd := rand.Float64()
		if rnd <= rm.ProxyTransitionProb {
			log.Debugf("processing forwards: starting proxy phase; cid=%s, from=%s", c, from)
			rm.startProxyPhase(ctx, from, c)
			rm.proxyMap[c][from] = true
		} else {
			log.Debugf("processing forwards: forwarding further; cid=%s, from=%s", c, from)
			excludes := make([]peer.ID, 0, len(rm.fromMap[c])+1)
			excludes = append(excludes, from)
			// Don't forward to a peer which already received a want-forward for this cid
			for _, alreadySentTo := range rm.fromMap[c] {
				excludes = append(excludes, alreadySentTo)
			}
			forwardedTo, err := rm.Forwarder.ForwardWants(ctx, c, excludes)
			if err != nil {
				log.Debugf("processing forwards: forward error; starting proxy phase; cid=%s, from=%s, err=%s", c, from, err)
				rm.startProxyPhase(ctx, from, c)
				rm.proxyMap[c][from] = true
				continue
			}
			rm.fromMap[c][from] = forwardedTo
			rm.toMap[c][forwardedTo] = from
		}
	}
}

func (rm *RelayManager) startProxyPhase(ctx context.Context, sender peer.ID, c cid.Cid) {
	innerCtx := ctx
	returnTo := sender
	k := c
	proxyDiscoveryCallback := func(provider peer.AddrInfo, received cid.Cid, sessionClosed bool) {
		rm.lk.Lock()
		defer rm.lk.Unlock()
		if _, ok := rm.proxyMap[c]; !ok {
			rm.proxyMap[c] = make(map[peer.ID]bool, 1)
		}

		if sessionClosed {
			rm.proxyMap[c][returnTo] = false
		}

		if provider.ID == peer.ID("") || !received.Defined() {
			return
		}

		if received != k {
			log.Debugf("[recv] cid not equal proxy cid; cid=%s, peer=%s, proxycid=%s", received, provider, k)
			return
		}
		log.Debugf("discovery callback; cid=%s, peer=%s, foundprovider=%s", received, returnTo, provider.ID)
		rm.RelayHaves(innerCtx, returnTo, k, []peer.AddrInfo{provider})
	}

	rm.addProxyDistance(c)
	session := rm.CreateProxySession(ctx, proxyDiscoveryCallback)
	session.GetBlocks(ctx, []cid.Cid{c})
}

// ProcessForwardHaves relays the forward-haves to all interested peers in the ledger
func (rm *RelayManager) ProcessForwardHaves(ctx context.Context, sender peer.ID, forwardHaves map[cid.Cid][]peer.AddrInfo) {
	for c, ps := range forwardHaves {
		if _, ok := rm.toMap[c]; !ok {
			log.Debugf("received a forward-have from a peer to which we did not relay; sender=%s, cid=%s", sender, c)
			continue
		}

		if _, ok := rm.toMap[c][sender]; !ok {
			log.Debugf("received a forward-have from a peer to which we did not relay; sender=%s, cid=%s", sender, c)
			continue
		}

		if rm.toMap[c][sender] == rm.self {
			// sessions are automatically notified
			continue
		}

		relayTo := rm.toMap[c][sender]
		rm.RelayHaves(ctx, relayTo, c, ps)
	}
}

func (rm *RelayManager) RelayHaves(ctx context.Context, to peer.ID, have cid.Cid, peers []peer.AddrInfo) {
	log.Debugf("relay haves; cid=%s, peer=%s, foundproviders=%s", have, to, peers)
	rm.Forwarder.ForwardHaves(ctx, to, have, peers)
	// For now, we just unprotect the connection when the first response is sent.
	// As later responses could be pruned, a more sophisticated approach might be worth it.
	rm.peerTagger.Unprotect(to, getConnectionProtectionTag(have))
}

// Just called by the requestor session
func (rm *RelayManager) ForwardSearch(ctx context.Context, c cid.Cid) error {
	rm.lk.Lock()
	defer rm.lk.Unlock()

	if _, ok := rm.fromMap[c]; !ok {
		rm.fromMap[c] = make(map[peer.ID]peer.ID, 1)
	}
	if _, ok := rm.proxyMap[c]; !ok {
		rm.proxyMap[c] = make(map[peer.ID]bool, 1)
	}
	if _, ok := rm.toMap[c]; !ok {
		rm.toMap[c] = make(map[peer.ID]peer.ID, 1)
	}

	log.Debugf("sending forward; cid=%s, from=%s", c, rm.self)
	excludes := make([]peer.ID, 0, len(rm.fromMap[c])+1)
	// Don't forward to a peer which already received a want-forward for this cid
	for _, alreadySentTo := range rm.fromMap[c] {
		excludes = append(excludes, alreadySentTo)
	}
	forwardedTo, err := rm.Forwarder.ForwardWants(ctx, c, excludes)
	if err != nil {
		log.Debugf("sending forward: forward error; cid=%s, from=%s, err=%s", c, rm.self, err)
		return err
	}
	rm.fromMap[c][rm.self] = forwardedTo
	rm.toMap[c][forwardedTo] = rm.self
	return nil
}

func (rm *RelayManager) addProxyDistance(c cid.Cid) {
	key := ks.XORKeySpace.Key([]byte(c.String()))
	d := ks.XORKeySpace.Key([]byte(rm.self)).Distance(key)
	rm.ProxyDistances = append(rm.ProxyDistances, d)
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
