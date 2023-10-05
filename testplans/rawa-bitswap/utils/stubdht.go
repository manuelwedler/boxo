package utils

import (
	"context"
	"math/rand"
	"time"

	"github.com/ipfs/go-cid"
	delay "github.com/ipfs/go-ipfs-delay"
	"github.com/libp2p/go-libp2p/core/peer"
	bsnet "github.com/manuelwedler/boxo/bitswap/network"
)

type stubDhtClient struct {
	providers map[cid.Cid][]peer.AddrInfo
	peers     map[peer.ID]peer.AddrInfo
	delayTime time.Duration
	randSeed  int64
}

func (d *stubDhtClient) FindProvidersAsync(ctx context.Context, k cid.Cid, _ int) <-chan peer.AddrInfo {
	out := make(chan peer.AddrInfo, len(d.providers[k]))
	go func() {
		defer close(out)
		d.wait()
		for _, p := range d.providers[k] {
			// Note that ipfs_impl of network interface only returns peer.ID from the peer.AddrInfo
			select {
			case <-ctx.Done():
				return
			case out <- p:
			}
		}
	}()
	return out
}

func (d *stubDhtClient) Provide(_ context.Context, _ cid.Cid, _ bool) error {
	return nil
}

func (d *stubDhtClient) AddProviderData(c cid.Cid, ais []peer.AddrInfo) {
	if _, ok := d.providers[c]; !ok {
		d.providers[c] = make([]peer.AddrInfo, 0, len(ais))
	}
	d.providers[c] = append(d.providers[c], ais...)
}

func (d *stubDhtClient) FindPeer(ctx context.Context, p peer.ID) (peer.AddrInfo, error) {
	d.wait()
	select {
	case <-ctx.Done():
		return peer.AddrInfo{}, ctx.Err()
	default:
	}
	return d.peers[p], nil
}

func (d *stubDhtClient) AddPeerData(ais []peer.AddrInfo) {
	for _, ai := range ais {
		d.peers[ai.ID] = ai
	}
}

func (d *stubDhtClient) wait() {
	// Add a uniform distributed variation of 10 % to the delay
	minDelay := time.Duration(float64(d.delayTime) * 0.9)
	maxVariation := time.Duration(float64(d.delayTime) * 0.2)
	rand.Seed(time.Now().Unix() + d.randSeed)
	accessDelay := delay.VariableUniform(minDelay, maxVariation, rand.New(rand.NewSource(rand.Int63())))
	accessDelay.Wait()
}

// ConstructStubDhtClient creates an Routing client which returns the predefined data after an delay
func ConstructStubDhtClient(delayTime time.Duration, seed int64) *stubDhtClient {
	return &stubDhtClient{
		providers: make(map[cid.Cid][]peer.AddrInfo),
		peers:     make(map[peer.ID]peer.AddrInfo),
		delayTime: delayTime,
		randSeed:  seed,
	}
}

// ensure stubDhtClient satisfies interface
var _ bsnet.ContentAndPeerRouting = &stubDhtClient{}
