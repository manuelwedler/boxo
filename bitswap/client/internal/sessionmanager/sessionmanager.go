package sessionmanager

import (
	"context"
	"strconv"
	"sync"
	"time"

	cid "github.com/ipfs/go-cid"
	delay "github.com/ipfs/go-ipfs-delay"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/ipfs/boxo/bitswap/client/internal"
	bsbpm "github.com/ipfs/boxo/bitswap/client/internal/blockpresencemanager"
	notifications "github.com/ipfs/boxo/bitswap/client/internal/notifications"
	bssession "github.com/ipfs/boxo/bitswap/client/internal/session"
	bssim "github.com/ipfs/boxo/bitswap/client/internal/sessioninterestmanager"
	bsrm "github.com/ipfs/boxo/bitswap/relaymanager"
	exchange "github.com/ipfs/boxo/exchange"
	logging "github.com/ipfs/go-log"
	peer "github.com/libp2p/go-libp2p/core/peer"
)

var log = logging.Logger("bs:sessmgr")

// Session is a session that is managed by the session manager
type Session interface {
	exchange.Fetcher
	ID() uint64
	ReceiveFrom(peer.AddrInfo, []cid.Cid, []cid.Cid, []cid.Cid)
	Shutdown()
}

// SessionFactory generates a new session for the SessionManager to track.
type SessionFactory func(
	ctx context.Context,
	sm bssession.SessionManager,
	id uint64,
	sprm bssession.SessionPeerManager,
	sim *bssim.SessionInterestManager,
	pm bssession.PeerManager,
	bpm *bsbpm.BlockPresenceManager,
	notif notifications.PubSub,
	provSearchDelay time.Duration,
	rebroadcastDelay delay.D,
	self peer.ID,
	proxy bool,
	proxyDiscoveryCallback bsrm.ProxyDiscoveryCallback,
	unforwardedSearchDelay time.Duration) Session

// PeerManagerFactory generates a new peer manager for a session.
type PeerManagerFactory func(ctx context.Context, id uint64) bssession.SessionPeerManager

// SessionManager is responsible for creating, managing, and dispatching to
// sessions.
type SessionManager struct {
	ctx                    context.Context
	sessionFactory         SessionFactory
	sessionInterestManager *bssim.SessionInterestManager
	peerManagerFactory     PeerManagerFactory
	blockPresenceManager   *bsbpm.BlockPresenceManager
	peerManager            bssession.PeerManager
	notif                  notifications.PubSub

	// Sessions
	sessLk   sync.RWMutex
	sessions map[uint64]Session

	// Session Index
	sessIDLk sync.Mutex
	sessID   uint64

	self peer.ID
}

// New creates a new SessionManager.
func New(ctx context.Context, sessionFactory SessionFactory, sessionInterestManager *bssim.SessionInterestManager, peerManagerFactory PeerManagerFactory,
	blockPresenceManager *bsbpm.BlockPresenceManager, peerManager bssession.PeerManager, notif notifications.PubSub, self peer.ID) *SessionManager {

	return &SessionManager{
		ctx:                    ctx,
		sessionFactory:         sessionFactory,
		sessionInterestManager: sessionInterestManager,
		peerManagerFactory:     peerManagerFactory,
		blockPresenceManager:   blockPresenceManager,
		peerManager:            peerManager,
		notif:                  notif,
		sessions:               make(map[uint64]Session),
		self:                   self,
	}
}

// NewSession initializes a session with the given context, and adds to the
// session manager.
func (sm *SessionManager) NewSession(ctx context.Context,
	provSearchDelay time.Duration,
	rebroadcastDelay delay.D,
	unforwardedSearchDelay time.Duration) exchange.Fetcher {
	id := sm.GetNextSessionID()

	ctx, span := internal.StartSpan(ctx, "SessionManager.NewSession", trace.WithAttributes(attribute.String("ID", strconv.FormatUint(id, 10))))
	defer span.End()

	pm := sm.peerManagerFactory(ctx, id)
	session := sm.sessionFactory(ctx, sm, id, pm, sm.sessionInterestManager, sm.peerManager, sm.blockPresenceManager, sm.notif, provSearchDelay, rebroadcastDelay, sm.self, false, func(peer.AddrInfo, cid.Cid) {}, unforwardedSearchDelay)

	sm.sessLk.Lock()
	if sm.sessions != nil { // check if SessionManager was shutdown
		sm.sessions[id] = session
	}
	sm.sessLk.Unlock()

	return session
}

// NewProxySession initializes a proxy session with the given context, and adds to the
// session manager.
func (sm *SessionManager) NewProxySession(ctx context.Context,
	proxyDiscoveryCallback bsrm.ProxyDiscoveryCallback,
	provSearchDelay time.Duration,
	rebroadcastDelay delay.D,
	unforwardedSearchDelay time.Duration) exchange.Fetcher {
	id := sm.GetNextSessionID()

	ctx, span := internal.StartSpan(ctx, "SessionManager.NewProxySession", trace.WithAttributes(attribute.String("ID", strconv.FormatUint(id, 10))))
	defer span.End()

	pm := sm.peerManagerFactory(ctx, id)
	session := sm.sessionFactory(ctx, sm, id, pm, sm.sessionInterestManager, sm.peerManager, sm.blockPresenceManager, sm.notif, provSearchDelay, rebroadcastDelay, sm.self, true, proxyDiscoveryCallback, unforwardedSearchDelay)

	sm.sessLk.Lock()
	if sm.sessions != nil { // check if SessionManager was shutdown
		sm.sessions[id] = session
	}
	sm.sessLk.Unlock()

	return session
}

func (sm *SessionManager) Shutdown() {
	sm.sessLk.Lock()

	sessions := make([]Session, 0, len(sm.sessions))
	for _, ses := range sm.sessions {
		sessions = append(sessions, ses)
	}

	// Ensure that if Shutdown() is called twice we only shut down
	// the sessions once
	sm.sessions = nil

	sm.sessLk.Unlock()

	for _, ses := range sessions {
		ses.Shutdown()
	}
}

func (sm *SessionManager) RemoveSession(sesid uint64) {
	// Remove session from SessionInterestManager - returns the keys that no
	// session is interested in anymore.
	cancelKs := sm.sessionInterestManager.RemoveSession(sesid)

	// Cancel keys that no session is interested in anymore
	sm.cancelWants(cancelKs)

	sm.sessLk.Lock()
	defer sm.sessLk.Unlock()

	// Clean up session
	if sm.sessions != nil { // check if SessionManager was shutdown
		delete(sm.sessions, sesid)
	}
}

// GetNextSessionID returns the next sequential identifier for a session.
func (sm *SessionManager) GetNextSessionID() uint64 {
	sm.sessIDLk.Lock()
	defer sm.sessIDLk.Unlock()

	sm.sessID++
	return sm.sessID
}

// ReceiveFrom is called when a new message is received
func (sm *SessionManager) ReceiveFrom(ctx context.Context, p peer.ID, blks []cid.Cid, haves []cid.Cid, dontHaves []cid.Cid, forwardHaves map[cid.Cid][]peer.AddrInfo) {
	// Record block presence for HAVE / DONT_HAVE
	sm.blockPresenceManager.ReceiveFrom(p, haves, dontHaves)

	forwardCids := make([]cid.Cid, 0, len(forwardHaves))

	for c, forwardedPeers := range forwardHaves {
		log.Debugw("ReceiveFrom <- forwardHaves", "from", p, "cid", c, "forwardedPeers", forwardedPeers)
		forwardCids = append(forwardCids, c)

		for _, forwardPeer := range forwardedPeers {
			sm.blockPresenceManager.ReceiveFrom(forwardPeer.ID, []cid.Cid{c}, []cid.Cid{})
		}
	}

	// Notify each session that is interested in the FORWARD_HAVEs
	for _, id := range sm.sessionInterestManager.ForwardInterestedSessions(forwardCids) {
		sm.sessLk.RLock()
		if sm.sessions == nil { // check if SessionManager was shutdown
			sm.sessLk.RUnlock()
			return
		}
		sess, ok := sm.sessions[id]
		sm.sessLk.RUnlock()

		if !ok {
			continue
		}

		for c, forwardedPeers := range forwardHaves {
			for _, forwardPeer := range forwardedPeers {
				sess.ReceiveFrom(forwardPeer, []cid.Cid{}, []cid.Cid{c}, []cid.Cid{})
			}
		}
	}

	// Notify each session that is interested in the blocks / HAVEs / DONT_HAVEs
	for _, id := range sm.sessionInterestManager.InterestedSessions(blks, haves, dontHaves) {
		sm.sessLk.RLock()
		if sm.sessions == nil { // check if SessionManager was shutdown
			sm.sessLk.RUnlock()
			return
		}
		sess, ok := sm.sessions[id]
		sm.sessLk.RUnlock()

		if ok {
			sess.ReceiveFrom(peer.AddrInfo{ID: p}, blks, haves, dontHaves)
		}
	}

	// Send CANCEL to all peers with want-have / want-block
	sm.peerManager.SendCancels(ctx, blks)
}

// CancelSessionWants is called when a session cancels wants because a call to
// GetBlocks() is cancelled
func (sm *SessionManager) CancelSessionWants(sesid uint64, wants []cid.Cid) {
	// Remove session's interest in the given blocks - returns the keys that no
	// session is interested in anymore.
	cancelKs := sm.sessionInterestManager.RemoveSessionInterested(sesid, wants)
	sm.cancelWants(cancelKs)
}

func (sm *SessionManager) cancelWants(wants []cid.Cid) {
	// Free up block presence tracking for keys that no session is interested
	// in anymore
	sm.blockPresenceManager.RemoveKeys(wants)

	// Send CANCEL to all peers for blocks that no session is interested in
	// anymore.
	// Note: use bitswap context because session context may already be Done.
	sm.peerManager.SendCancels(sm.ctx, wants)
}
