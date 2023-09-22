package utils

import (
	"context"
	"math/rand"
	"time"

	bsnet "github.com/ipfs/boxo/bitswap/network"
	"github.com/ipfs/go-cid"
	delay "github.com/ipfs/go-ipfs-delay"
	"github.com/libp2p/go-libp2p/core/peer"
)

type stubDhtClient struct {
	providers   map[cid.Cid][]peer.AddrInfo
	peers       map[peer.ID]peer.AddrInfo
	accessDelay delay.D
}

func (d *stubDhtClient) FindProvidersAsync(ctx context.Context, k cid.Cid, _ int) <-chan peer.AddrInfo {
	out := make(chan peer.AddrInfo, len(d.providers[k]))
	go func() {
		defer close(out)
		d.accessDelay.Wait()
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
	d.accessDelay.Wait()
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

// ConstructStubDhtClient creates an Routing client which returns the predefined data after an delay
func ConstructStubDhtClient(delayTime time.Duration) *stubDhtClient {
	// Add a uniform distributed variation of 10 % to the delay
	randSource := rand.NewSource(time.Now().UnixNano())
	minDelay := time.Duration(float64(delayTime) * 0.9)
	maxVariation := time.Duration(float64(delayTime) * 0.2)
	accessDelay := delay.VariableUniform(minDelay, maxVariation, rand.New(randSource))
	return &stubDhtClient{
		providers:   make(map[cid.Cid][]peer.AddrInfo),
		peers:       make(map[peer.ID]peer.AddrInfo),
		accessDelay: accessDelay,
	}
}

// ensure stubDhtClient satisfies interface
var _ bsnet.ContentAndPeerRouting = &stubDhtClient{}
