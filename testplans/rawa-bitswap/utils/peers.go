package utils

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/sync/errgroup"
)

func AddrInfosFromChan(peerCh chan *peer.AddrInfo, count int) ([]peer.AddrInfo, error) {
	var ais []peer.AddrInfo
	for i := 1; i <= count; i++ {
		ai, ok := <-peerCh
		if !ok {
			return ais, fmt.Errorf("subscription closed")
		}
		ais = append(ais, *ai)
	}
	return ais, nil
}

func SpyDialPeers(ctx context.Context, self host.Host, ais []peer.AddrInfo, spys map[peer.ID]struct{}) ([]peer.AddrInfo, error) {
	// Grab list of other peers that are available for this Run
	var toDial []peer.AddrInfo
	for _, ai := range ais {
		id1, _ := ai.ID.MarshalBinary()
		id2, _ := self.ID().MarshalBinary()

		// skip over dialing ourselves, and prevent TCP simultaneous
		// connect (known to fail) by only dialing peers whose peer ID
		// is smaller than ours.
		if _, ok := spys[ai.ID]; ok && bytes.Compare(id1, id2) >= 0 {
			continue
		} // todo should spys even connect to other spys?

		toDial = append(toDial, ai)
	}

	// Dial to all the other peers
	g, ctx := errgroup.WithContext(ctx)
	for _, ai := range toDial {
		ai := ai
		g.Go(func() error {
			if err := self.Connect(ctx, ai); err != nil {
				return fmt.Errorf("error while dialing peer %v: %w", ai.Addrs, err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return toDial, nil
}

func DialOtherPeers(ctx context.Context, self host.Host, ais []peer.AddrInfo, spys map[peer.ID]struct{}, count int, rSeed int64) ([]peer.AddrInfo, error) {
	// Grab list of other peers that are available for this Run
	var toDial []peer.AddrInfo
	for _, ai := range ais {
		id1, _ := ai.ID.MarshalBinary()
		id2, _ := self.ID().MarshalBinary()

		// skip over dialing ourselves, and prevent TCP simultaneous
		// connect (known to fail) by only dialing peers whose peer ID
		// is smaller than ours.
		if bytes.Compare(id1, id2) >= 0 {
			continue
		}

		// Don't connect to spys.
		// They choose their connections themselves.
		if _, ok := spys[ai.ID]; ok {
			continue
		}

		toDial = append(toDial, ai)
	}

	// Select randomly peers according to count
	var randDial []peer.AddrInfo
	for len(randDial) < count && len(randDial) < len(toDial) {
		rand.Seed(rSeed)
		perm := rand.Perm(len(toDial))
		randDial = append(randDial, toDial[perm[0]])
	}

	// Dial to all the other peers
	g, ctx := errgroup.WithContext(ctx)
	for _, ai := range randDial {
		ai := ai
		g.Go(func() error {
			if err := self.Connect(ctx, ai); err != nil {
				return fmt.Errorf("error while dialing peer %v: %w", ai.Addrs, err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return randDial, nil
}
