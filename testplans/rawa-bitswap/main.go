package main

import (
	"context"
	"fmt"
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
	// ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
	options = append(options, libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/tcp/0", ip)))

	options = append(options, libp2p.Transport(libp2pquic.NewTransport))
	options = append(options, libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/%s/udp/0/quic", ip)))
	options = append(options, libp2p.Security(noise.ID, noise.New))

	// listen, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%d", initCtx.NetClient.MustGetDataNetworkIP().String(), 3333+initCtx.GlobalSeq))
	// if err != nil {
	// 	return err
	// }
	// h, err := libp2p.New(libp2p.ListenAddrs(listen))

	// h, err := libp2p.New()

	h, err := libp2p.New(options...)
	if err != nil {
		return fmt.Errorf("error during libp2p node creation: %s", err)
	}
	defer h.Close()
	runenv.RecordMessage("I am %s with addrs: %v", h.ID(), h.Addrs())

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

	// for _, ai := range addrInfos {
	// 	h.Peerstore().AddAddrs(ai.ID, ai.Addrs, peerstore.AddressTTL)
	// }

	// Instance type
	provider := false
	requestor := false
	switch c := initCtx.GlobalSeq; {
	case c == 1:
		runenv.RecordMessage("running provider")
		provider = true
	case c == 2:
		runenv.RecordMessage("running requestor")
		requestor = true
	default:
		runenv.RecordMessage("running passive")
	}

	// Use the same blockstore on all runs for the seed node
	var bstore blockstore.Blockstore
	if provider {
		bstore, err = utils.CreateBlockstore(ctx, bstoreDelay)
		if err != nil {
			return fmt.Errorf("error during blockstore creation (provider): %s", err)
		}
	}

	var rootCid cid.Cid
	var providerInfo peer.AddrInfo
	for runNum := 1; runNum < runCount+1; runNum++ {
		isFirstRun := runNum == 1
		runId := fmt.Sprintf("%d", runNum)
		runCtx, cancelRun := context.WithCancel(ctx)
		defer cancelRun()

		// Wait for all nodes to be ready to start the run
		err = signalAndWaitForAll("start-run-" + runId)
		if err != nil {
			return err
		}

		runenv.RecordMessage("Starting run %d / %d (%d bytes)", runNum, runCount, fileSize)
		var bsnode *utils.Node
		rootCidTopic := sync.NewTopic("root-cid", &cid.Cid{})
		providerTopic := sync.NewTopic("provider-info", &peer.AddrInfo{})

		dht := utils.ConstructStubDhtClient(dhtStubDelay)
		if provider {
			// For seeds, create a new bitswap node from the existing datastore
			bsnode, err = utils.CreateBitswapNode(runCtx, h, bstore, dht)
			if err != nil {
				return fmt.Errorf("error during bs node creation (provider): %s", err)
			}

			// If this is the first run
			if isFirstRun {
				runenv.RecordMessage("Generating seed data of %d bytes", fileSize)

				tmpFile := utils.RandReader(fileSize)
				ipldNode, err := bsnode.Add(runCtx, tmpFile)
				if err != nil {
					return fmt.Errorf("failed to set up seed: %w", err)
				}
				rootCid = ipldNode.Cid()
				providerInfo = *host.InfoFromHost(h)

				// Inform other nodes of the root CID
				if _, err = client.Publish(runCtx, rootCidTopic, &rootCid); err != nil {
					return fmt.Errorf("failed to get Redis Sync rootCidTopic %w", err)
				}
				// Inform other nodes of the provider ID
				if _, err = client.Publish(runCtx, providerTopic, &providerInfo); err != nil {
					return fmt.Errorf("failed to get Redis Sync providerTopic %w", err)
				}
				runenv.RecordMessage("Provider initialization done")
			}
		} else {
			// For leeches and passives, create a new blockstore on each run
			bstore, err = utils.CreateBlockstore(runCtx, bstoreDelay)
			if err != nil {
				return fmt.Errorf("error during blockstore creation (passive / requestor): %s", err)
			}

			// Create a new bitswap node from the blockstore
			bsnode, err = utils.CreateBitswapNode(runCtx, h, bstore, dht)
			if err != nil {
				return fmt.Errorf("error during bs node creation (passive / requestor): %s", err)
			}

			// If this is the first run for this file size
			if isFirstRun {
				runenv.RecordMessage("Accessing root cid and provider information for stub dht")
				// Get the root CID from a seed
				rootCidCh := make(chan *cid.Cid, 1)
				sctx, cancelRootCidSub := context.WithCancel(runCtx)
				if _, err := client.Subscribe(sctx, rootCidTopic, rootCidCh); err != nil {
					cancelRootCidSub()
					return fmt.Errorf("failed to subscribe to rootCidTopic %w", err)
				}
				rootCidPtr, ok := <-rootCidCh
				cancelRootCidSub()
				if !ok {
					return fmt.Errorf("no root cid in %d seconds", timeout/time.Second)
				}
				rootCid = *rootCidPtr

				// Get the provider info
				providerInfoCh := make(chan *peer.AddrInfo, 1)
				sctx, cancelProviderInfoSub := context.WithCancel(runCtx)
				if _, err := client.Subscribe(sctx, providerTopic, providerInfoCh); err != nil {
					cancelProviderInfoSub()
					return fmt.Errorf("failed to subscribe to providerTopic %w", err)
				}
				providerInfoPtr, ok := <-providerInfoCh
				cancelProviderInfoSub()
				if !ok {
					return fmt.Errorf("no provider info in %d seconds", timeout/time.Second)
				}
				providerInfo = *providerInfoPtr
				runenv.RecordMessage("Requestor / passive initialization done")
			}
		}
		dht.AddProviderData(rootCid, []peer.AddrInfo{providerInfo})
		dht.AddPeerData(addrInfos)

		// Wait for all nodes to be ready to dial
		err = signalAndWaitForAll("ready-to-connect-" + runId)
		if err != nil {
			return err
		}

		// Dial connPerNode peers
		dialed, err := utils.DialOtherPeers(runCtx, h, addrInfos, connPerNode)
		if err != nil {
			return fmt.Errorf("error during peer dial: %s", err)
		}
		runenv.RecordMessage("Dialed %d other nodes (run %d)", len(dialed), runNum)

		// Wait for all nodes to be connected
		err = signalAndWaitForAll("connect-complete-" + runId)
		if err != nil {
			return err
		}

		/// --- Start test
		runenv.RecordMessage("Test start (run %d)", runNum)
		if requestor {
			var timeToFetch time.Duration
			runenv.RecordMessage("Starting fetch (run %d)", runNum)
			start := time.Now()
			err := bsnode.FetchGraph(runCtx, rootCid)
			timeToFetch = time.Since(start)
			if err != nil {
				return fmt.Errorf("error fetching data through Bitswap: %w", err)
			}
			runenv.RecordMessage("Leech fetch complete (%s) (run %d)", timeToFetch, runNum)
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

		// Disconnect peers
		for _, c := range h.Network().Conns() {
			err := c.Close()
			if err != nil {
				return fmt.Errorf("error disconnecting: %w", err)
			}
		}

		if !provider {
			// Free up memory by clearing the leech blockstore at the end of each run.
			// Note that although we create a new blockstore for the leech at the
			// start of the run, explicitly cleaning up the blockstore from the
			// previous run allows it to be GCed.
			if err := utils.ClearBlockstore(runCtx, bstore); err != nil {
				return fmt.Errorf("error clearing blockstore: %w", err)
			}
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
