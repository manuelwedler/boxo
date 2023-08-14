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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client := sync.MustBoundClient(ctx, runenv)

	// Signal that this node is in the given state, and wait for all peers to
	// send the same signal
	signalAndWaitForAll := func(state string) error {
		_, err := client.SignalAndWait(ctx, sync.State(state), runenv.TestInstanceCount)
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

	h, err := libp2p.New()
	if err != nil {
		return err
	}
	defer h.Close()
	runenv.RecordMessage("I am %s with addrs: %v", h.ID(), h.Addrs())

	// Publish addrs
	peersTopic := sync.NewTopic("peers", &peer.AddrInfo{})
	_, err = client.Publish(ctx, peersTopic, host.InfoFromHost(h))
	if err != nil {
		return err
	}

	// Get addresses of all peers
	peerCh := make(chan *peer.AddrInfo)
	sctx, cancelSub := context.WithCancel(ctx)
	if _, err := client.Subscribe(sctx, peersTopic, peerCh); err != nil {
		cancelSub()
		return err
	}
	addrInfos, err := utils.AddrInfosFromChan(peerCh, runenv.TestInstanceCount)
	if err != nil {
		cancelSub()
		return fmt.Errorf("no addrs in %d seconds", timeout/time.Second)
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
			return err
		}
	}

	var rootCid cid.Cid
	var providerInfo peer.AddrInfo
	for runNum := 1; runNum < runCount+1; runNum++ {
		isFirstRun := runNum == 1
		runId := fmt.Sprintf("%d", runNum)

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
			bsnode, err = utils.CreateBitswapNode(ctx, h, bstore, dht)
			if err != nil {
				return err
			}

			// If this is the first run
			if isFirstRun {
				runenv.RecordMessage("Generating seed data of %d bytes", fileSize)

				tmpFile := utils.RandReader(fileSize)
				ipldNode, err := bsnode.Add(ctx, tmpFile)
				if err != nil {
					return fmt.Errorf("failed to set up seed: %w", err)
				}
				rootCid = ipldNode.Cid()
				providerInfo = *host.InfoFromHost(h)

				// Inform other nodes of the root CID
				if _, err = client.Publish(ctx, rootCidTopic, &rootCid); err != nil {
					return fmt.Errorf("failed to get Redis Sync rootCidTopic %w", err)
				}
				// Inform other nodes of the provider ID
				if _, err = client.Publish(ctx, providerTopic, &providerInfo); err != nil {
					return fmt.Errorf("failed to get Redis Sync providerTopic %w", err)
				}
				runenv.RecordMessage("Provider initialization done")
			}
		} else {
			// For leeches and passives, create a new blockstore on each run
			bstore, err = utils.CreateBlockstore(ctx, bstoreDelay)
			if err != nil {
				return err
			}

			// Create a new bitswap node from the blockstore
			bsnode, err = utils.CreateBitswapNode(ctx, h, bstore, dht)
			if err != nil {
				return err
			}

			// If this is the first run for this file size
			if isFirstRun {
				runenv.RecordMessage("Accessing root cid and provider information for stub dht")
				// Get the root CID from a seed
				rootCidCh := make(chan *cid.Cid, 1)
				sctx, cancelRootCidSub := context.WithCancel(ctx)
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
				sctx, cancelProviderInfoSub := context.WithCancel(ctx)
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
		dialed, err := utils.DialOtherPeers(ctx, h, addrInfos, connPerNode)
		if err != nil {
			return err
		}
		runenv.RecordMessage("Dialed %d other nodes (run %d)", len(dialed), runNum)

		// Wait for all nodes to be connected
		err = signalAndWaitForAll("connect-complete-" + runId)
		if err != nil {
			return err
		}

		/// --- Start test
		runenv.RecordMessage("Test start (run %d)", runNum)
		var timeToFetch time.Duration
		if requestor {
			runenv.RecordMessage("Starting fetch (run %d)", runNum)
			start := time.Now()
			err := bsnode.FetchGraph(ctx, rootCid)
			timeToFetch = time.Since(start)
			if err != nil {
				return fmt.Errorf("error fetching data through Bitswap: %w", err)
			}
			runenv.RecordMessage("Leech fetch complete (%s) (run %d)", timeToFetch, runNum)
		}

		// Wait for all leeches to have downloaded the data from seeds
		err = signalAndWaitForAll("transfer-complete-" + runId)
		if err != nil {
			return err
		}

		/// --- Report stats
		runenv.R().RecordPoint("time-to-fetch-ms", float64(timeToFetch.Milliseconds()))
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
			if err := utils.ClearBlockstore(ctx, bstore); err != nil {
				return fmt.Errorf("error clearing blockstore: %w", err)
			}
		}
	}

	/// --- Ending the test

	return nil
}
