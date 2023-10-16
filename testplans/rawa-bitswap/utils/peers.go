package utils

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"sort"

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

func SpyDialPeersDeterministically(ctx context.Context, self host.Host, ais []peer.AddrInfo, spys map[peer.ID]struct{}, count int) ([]peer.AddrInfo, error) {
	sortedAisHonest := make([]peer.AddrInfo, 0, len(ais))
	sortedAisHonest = append(sortedAisHonest, ais...)
	for _, ai := range ais {
		if _, ok := spys[ai.ID]; ok {
			continue
		}
		sortedAisHonest = append(sortedAisHonest, ai)
	}
	sort.SliceStable(sortedAisHonest, func(i, j int) bool {
		id1, _ := sortedAisHonest[i].ID.MarshalBinary()
		id2, _ := sortedAisHonest[j].ID.MarshalBinary()
		return bytes.Compare(id1, id2) < 0
	})

	sortedSpys := make([]peer.ID, 0, len(spys))
	for s := range spys {
		sortedSpys = append(sortedSpys, s)
	}
	sort.SliceStable(sortedSpys, func(i, j int) bool {
		id1, _ := sortedSpys[i].MarshalBinary()
		id2, _ := sortedSpys[j].MarshalBinary()
		return bytes.Compare(id1, id2) < 0
	})

	var selfSpyIndex int
	for i, s := range sortedSpys {
		if s == self.ID() {
			selfSpyIndex = i
			break
		}
	}

	// Grab list of other peers that are available for this Run
	toDial := make([]peer.AddrInfo, 0, count)
	for i, ai := range sortedAisHonest {
		if i >= selfSpyIndex*count && i < (selfSpyIndex+1)*count {
			toDial = append(toDial, ai)
		}
	}

	// Dial to all selected peers
	g, ctx := errgroup.WithContext(ctx)
	for _, ai := range toDial {
		ai := ai
		if ai.ID == self.ID() {
			continue
		}
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

func SpyDialAllPeers(ctx context.Context, self host.Host, ais []peer.AddrInfo, spys map[peer.ID]struct{}) ([]peer.AddrInfo, error) {
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

func DialOtherPeers(ctx context.Context, self host.Host, ais []peer.AddrInfo, excludes map[peer.ID]struct{}, count int, rSeed int64) ([]peer.AddrInfo, error) {
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
		} // todo / this compare should actually be < 0

		// Don't connect to spys.
		// They choose their connections themselves.
		if _, ok := excludes[ai.ID]; ok {
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
