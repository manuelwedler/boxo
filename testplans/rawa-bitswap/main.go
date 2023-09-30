package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"time"

	logging "github.com/ipfs/go-log"

	blockstore "github.com/ipfs/boxo/blockstore"
	"github.com/ipfs/boxo/testplan/rawa-bitswap/utils"
	"github.com/ipfs/go-cid"
	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	noise "github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	"github.com/testground/sdk-go/network"
	"github.com/testground/sdk-go/run"
	"github.com/testground/sdk-go/runtime"
	"github.com/testground/sdk-go/sync"
)

var (
	testcases = map[string]interface{}{
		"rawa-test": run.InitializedTestCaseFn(runRaWaTest),
	}
	bstoreDelay  = time.Duration(5) * time.Millisecond
	latency      = 100 * time.Millisecond
	jitter       = 10 * time.Millisecond // 10% jitter
	bandwidth    = 1024 * 1024           // 1MiB
	dhtStubDelay = time.Duration(622) * time.Millisecond
	// Chunker chunks into 256kiB blocks
	// This size fits in 1 block
	fileSize = 150 * 1024 // 150 kiB
)

func main() {
	run.InvokeMap(testcases)
}

func runRaWaTest(runenv *runtime.RunEnv, initCtx *run.InitContext) error {
	// Parameters
	debug := runenv.BooleanParam("debug")
	timeout := time.Duration(runenv.IntParam("timeout_secs")) * time.Second
	runCount := runenv.IntParam("run_count")
	connPerNode := runenv.IntParam("conn_per_node")
	allMode := runenv.BooleanParam("all_mode")

	// Show debug logs
	if debug {
		logging.SetAllLoggers(logging.LevelDebug)

		err := logging.SetLogLevel("*", "debug")
		if err != nil {
			fmt.Print("Logging error:" + err.Error())
		}
	}

	// Set up
	runenv.RecordMessage("running RaWa-Bitswap test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := sync.MustBoundClient(ctx, runenv)

	// Wait until all instances in this test run have signalled.
	initCtx.MustWaitAllInstancesInitialized(ctx)

	// Signal that this node is in the given state, and wait for all peers to
	// send the same signal
	signalAndWaitForAll := func(state string) error {
		_, err := client.SignalAndWait(ctx, sync.State(state), runenv.TestInstanceCount)
		if err != nil {
			err = fmt.Errorf("error during waiting for state %s: %s", state, err)
		}
		return err
	}

	// Network shaping
	linkShape := network.LinkShape{
		Latency:       latency,
		Jitter:        jitter,
		Bandwidth:     uint64(bandwidth),
		Loss:          0,
		Corrupt:       0,
		CorruptCorr:   0,
		Reorder:       0,
		ReorderCorr:   0,
		Duplicate:     0,
		DuplicateCorr: 0,
	}
	initCtx.NetClient.MustConfigureNetwork(ctx, &network.Config{
		Network:        "default",
		Enable:         true,
		Default:        linkShape,
		CallbackState:  sync.State("network-configured"),
		CallbackTarget: runenv.TestGroupInstanceCount,
		RoutingPolicy:  network.AllowAll,
	})

	ip := initCtx.NetClient.MustGetDataNetworkIP()
	var options []libp2p.Option
	options = append(options, libp2p.Transport(tcp.NewTCPTransport))
	options = append(options, libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/tcp/%d", ip, 3333+initCtx.GlobalSeq)))

	options = append(options, libp2p.Transport(libp2pquic.NewTransport))
	options = append(options, libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/udp/%d/quic", ip, 6666+initCtx.GlobalSeq)))

	options = append(options, libp2p.Security(noise.ID, noise.New))

	h, err := libp2p.New(options...)
	if err != nil {
		return fmt.Errorf("error during libp2p node creation: %s", err)
	}
	defer h.Close()
	runenv.RecordMessage("I am %s with addrs: %v", h.ID(), h.Addrs())

	// Log PeerID with every message
	recordMessage := func(msg string, a ...interface{}) {
		id := h.ID().String()
		prefix := fmt.Sprintf("[...%s] ", id[len(id)-6:])
		runenv.RecordMessage(prefix+msg, a...)
	}

	// Publish addrs
	peersTopic := sync.NewTopic("peers", &peer.AddrInfo{})
	_, err = client.Publish(ctx, peersTopic, host.InfoFromHost(h))
	if err != nil {
		return fmt.Errorf("error during addrs publish: %s", err)
	}

	// Get addresses of all peers
	peerCh := make(chan *peer.AddrInfo)
	sctx, cancelSub := context.WithCancel(ctx)
	if _, err := client.Subscribe(sctx, peersTopic, peerCh); err != nil {
		cancelSub()
		return fmt.Errorf("error during waiting for others addrs (sub): %s", err)
	}
	addrInfos, err := utils.AddrInfosFromChan(peerCh, runenv.TestInstanceCount)
	if err != nil {
		cancelSub()
		return fmt.Errorf("error during waiting for others addrs (chan): %s", err)
	}
	cancelSub()

	// Instance type
	provider := false
	requestor := false
	if allMode {
		provider = true
		requestor = true
		recordMessage("running requestor and provider")
	} else {
		switch c := initCtx.GlobalSeq; {
		case c == 1:
			recordMessage("running provider")
			provider = true
		case c == 2:
			recordMessage("running requestor")
			requestor = true
		default:
			recordMessage("running passive")
		}
	}

	// Use the same blockstore on all runs, just make sure to clear later
	var bstore blockstore.Blockstore
	bstore, err = utils.CreateBlockstore(ctx, bstoreDelay)
	if err != nil {
		return fmt.Errorf("error during blockstore creation: %s", err)
	}

	// Rand source from peer ID to generate different data on each peer
	idBytes, err := h.ID().MarshalBinary()
	if err != nil {
		return fmt.Errorf("error during rand source creation: %s", err)
	}
	rSeed := int64(binary.LittleEndian.Uint64(idBytes[:8]))
	rSource := rand.NewSource(rSeed)
	r := rand.New(rSource)
	// Initialize global seed for places where we need concurrency
	rand.Seed(rSeed)

	// Stub DHT
	dht := utils.ConstructStubDhtClient(dhtStubDelay, r.Int63())

	var ownProviderInfo ProviderInfo
	var allProviderInfos []ProviderInfo
	for runNum := 1; runNum < runCount+1; runNum++ {
		isFirstRun := runNum == 1
		runId := fmt.Sprintf("%d", runNum)
		runCtx, cancelRun := context.WithTimeout(ctx, timeout)
		defer cancelRun()

		// Wait for all nodes to be ready to start the run
		err = signalAndWaitForAll("start-run-" + runId)
		if err != nil {
			return err
		}

		recordMessage("Starting run %d / %d (%d bytes)", runNum, runCount, fileSize)
		var bsnode *utils.Node
		providerTopic := sync.NewTopic("provider-info", &ProviderInfo{})

		// Create a new bitswap node from the blockstore
		bsnode, err = utils.CreateBitswapNode(runCtx, h, bstore, dht)
		if err != nil {
			return fmt.Errorf("error during bs node creation: %s", err)
		}
		if isFirstRun {
			if provider || allMode {
				recordMessage("Generating seed data of %d bytes", fileSize)

				tmpFile := utils.RandReader(fileSize, r.Int63())
				ipldNode, err := bsnode.Add(runCtx, tmpFile)
				if err != nil {
					return fmt.Errorf("failed to set up seed: %w", err)
				}
				ownProviderInfo = ProviderInfo{
					Cid:       ipldNode.Cid(),
					AddrsInfo: *host.InfoFromHost(h),
				}
				recordMessage("Generated block for cid: %s", ownProviderInfo.Cid)

				// Inform other nodes of the provider ID
				if _, err = client.Publish(runCtx, providerTopic, &ownProviderInfo); err != nil {
					return fmt.Errorf("failed to get Redis Sync providerTopic %w", err)
				}
				recordMessage("Provider initialization done")
			}
			recordMessage("Accessing root cid and provider information for stub dht")
			// Get the provider info
			providerInfoCh := make(chan *ProviderInfo)
			sctx, cancelProviderInfoSub := context.WithCancel(runCtx)
			if _, err := client.Subscribe(sctx, providerTopic, providerInfoCh); err != nil {
				cancelProviderInfoSub()
				return fmt.Errorf("failed to subscribe to providerTopic %w", err)
			}
			var count int
			if allMode {
				count = runenv.TestInstanceCount
			} else {
				count = 1
			}
			for i := 1; i <= count; i++ {
				info, ok := <-providerInfoCh
				if !ok {
					cancelProviderInfoSub()
					return fmt.Errorf("subscription to providerTopic closed")
				}
				allProviderInfos = append(allProviderInfos, *info)
			}
			cancelProviderInfoSub()
			recordMessage("Accessing root cid and provider information done")

			// Add data to DHT
			for _, provider := range allProviderInfos {
				dht.AddProviderData(provider.Cid, []peer.AddrInfo{provider.AddrsInfo})
			}
			dht.AddPeerData(addrInfos)
		}

		// Wait for all nodes to be ready to dial
		err = signalAndWaitForAll("ready-to-connect-" + runId)
		if err != nil {
			return err
		}

		// Dial connPerNode peers
		dialed, err := utils.DialOtherPeers(runCtx, h, addrInfos, connPerNode, r.Int63())
		if err != nil {
			return fmt.Errorf("error during peer dial: %s", err)
		}
		recordMessage("Dialed %d other nodes (run %d)", len(dialed), runNum)

		// Wait for all nodes to be connected
		err = signalAndWaitForAll("connect-complete-" + runId)
		if err != nil {
			return err
		}

		/// --- Start test
		recordMessage("Test start (run %d)", runNum)
		if requestor || allMode {
			recordMessage("Starting fetch (run %d)", runNum)
			// Fetch random Cid from the available ones
			chosen := ownProviderInfo.Cid
			for chosen == ownProviderInfo.Cid {
				perm := r.Perm(len(allProviderInfos))
				chosen = allProviderInfos[perm[0]].Cid
			}
			recordMessage("Fetching cid %s", chosen)
			start := time.Now()
			err := bsnode.FetchGraph(runCtx, chosen)
			timeToFetch := time.Since(start)
			if err != nil {
				return fmt.Errorf("error fetching data through Bitswap: %w", err)
			}
			recordMessage("Leech fetch complete (%s) (run %d)", timeToFetch, runNum)
			/// --- Report stats
			runenv.R().RecordPoint("time-to-fetch-ms", float64(timeToFetch.Milliseconds()))
		}

		// Wait for all nodes
		err = signalAndWaitForAll("transfer-complete-" + runId)
		if err != nil {
			return err
		}

		// Shut down bitswap
		err = bsnode.Close()
		if err != nil {
			return fmt.Errorf("error closing Bitswap: %w", err)
		}

		// Wait for all nodes
		err = signalAndWaitForAll("bitswap-closed" + runId)
		if err != nil {
			return err
		}

		// Disconnect peers
		for _, c := range h.Network().Conns() {
			err = c.Close()
			if err != nil {
				return fmt.Errorf("error disconnecting: %w", err)
			}
		}

		// Free up memory by clearing the leech blockstore at the end of each run.
		// Note that explicitly cleaning up the blockstore from the
		// previous run allows it to be GCed.
		if err := utils.ClearBlockstore(runCtx, bstore, ownProviderInfo.Cid); err != nil {
			return fmt.Errorf("error clearing blockstore: %w", err)
		}

		// Clear peerstores
		for _, ai := range addrInfos {
			h.Peerstore().ClearAddrs(ai.ID)
			h.Peerstore().RemovePeer(ai.ID)
		}

		cancelRun()

		// Wait for all nodes
		err = signalAndWaitForAll("run-done-" + runId)
		if err != nil {
			return err
		}
	}

	/// --- Ending the test

	return nil
}

type ProviderInfo struct {
	Cid       cid.Cid
	AddrsInfo peer.AddrInfo
}
