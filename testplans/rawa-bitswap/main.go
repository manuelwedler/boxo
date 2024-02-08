package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"time"

	logging "github.com/ipfs/go-log"

	"github.com/ipfs/go-cid"
	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	bs "github.com/manuelwedler/boxo/bitswap"
	bsfs "github.com/manuelwedler/boxo/bitswap/forwardstrategy"
	bsnet "github.com/manuelwedler/boxo/bitswap/network"
	blockstore "github.com/manuelwedler/boxo/blockstore"
	"github.com/manuelwedler/boxo/testplan/rawa-bitswap/utils"

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
	jitter       = 10 * time.Millisecond // 10% jitter
	bandwidth    = 1024 * 1024           // 1MiB
	dhtStubDelay = time.Duration(622) * time.Millisecond
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
	firstSpyMode := runenv.BooleanParam("first_spy")
	forwardExploiterCount := runenv.IntParam("forward_exploiter")
	forwardExploiterMode := forwardExploiterCount > 0
	subgraphAwareExploiterCount := runenv.IntParam("subgraph_aware_exploiter")
	subgraphAwareExploiterMode := subgraphAwareExploiterCount > 0
	if firstSpyMode && forwardExploiterMode || firstSpyMode && subgraphAwareExploiterMode || forwardExploiterMode && subgraphAwareExploiterMode {
		return fmt.Errorf("cannot run two different adversaries at the same time")
	}

	proxyTransitionProb := runenv.FloatParam("proxy_transition_prob")
	unforwardedSearchTime := time.Duration(runenv.IntParam("unforwarded_search_time")) * time.Second
	forwardGraphDegree := uint64(runenv.IntParam("forward_graph_degree"))
	closestPeeridForward := runenv.BooleanParam("closest_peerid_forward")

	if forwardGraphDegree == 0 {
		forwardGraphDegree = math.MaxUint64
	}

	latency := time.Duration(runenv.IntParam("net_latency")) * time.Millisecond
	// Chunker chunks into 256kiB blocks
	fileSize := runenv.IntParam("file_size")

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
		RoutingPolicy:  network.DenyAll,
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
	spy := false
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
	if firstSpyMode && initCtx.GlobalSeq == 3 {
		spy = true
		provider = false
		requestor = false
		recordMessage("running first-spy estimator")
	}
	if forwardExploiterMode && (initCtx.GlobalSeq >= 3 && initCtx.GlobalSeq <= 2+int64(forwardExploiterCount)) {
		spy = true
		provider = false
		requestor = false
		recordMessage("running want-forward exploiter")
	}
	if subgraphAwareExploiterMode && (initCtx.GlobalSeq >= 3 && initCtx.GlobalSeq <= 2+int64(subgraphAwareExploiterCount)) {
		spy = true
		provider = false
		requestor = false
		recordMessage("running subgraph-aware want-forward exploiter")
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

	spyCount := 0
	if firstSpyMode {
		spyCount = 1
	}
	if forwardExploiterMode {
		spyCount = forwardExploiterCount
	}
	if subgraphAwareExploiterMode {
		spyCount = subgraphAwareExploiterCount
	}
	// number of seeds and leeds is always the same
	var seedLeechCount int
	if allMode {
		seedLeechCount = runenv.TestInstanceCount - spyCount
	} else {
		seedLeechCount = 1
	}

	// Publish spy ids
	spyTopic := sync.NewTopic("spys", &peer.AddrInfo{})
	if spy {
		_, err = client.Publish(ctx, spyTopic, host.InfoFromHost(h))
		if err != nil {
			return fmt.Errorf("error during spy publish: %s", err)
		}
	}

	// Get ids of all spys
	spyCh := make(chan *peer.AddrInfo)
	sctx, cancelSpySub := context.WithCancel(ctx)
	if _, err := client.Subscribe(sctx, spyTopic, spyCh); err != nil {
		cancelSpySub()
		return fmt.Errorf("error during waiting for spy addrs (sub): %s", err)
	}
	spyInfos, err := utils.AddrInfosFromChan(spyCh, spyCount)
	if err != nil {
		cancelSpySub()
		return fmt.Errorf("error during waiting for spy addrs (chan): %s", err)
	}
	cancelSpySub()

	allSpys := make(map[peer.ID]struct{})
	for _, s := range spyInfos {
		allSpys[s.ID] = struct{}{}
	}

	// Stub DHT
	dht := utils.ConstructStubDhtClient(dhtStubDelay, r.Int63())

	var ownProviderInfo ProviderInfo
	var allProviderInfos []ProviderInfo
	for runNum := 1; runNum < runCount+1; runNum++ {
		isFirstRun := runNum == 1
		runId := fmt.Sprintf("%d", runNum)

		var runCtx context.Context
		var cancelRun context.CancelFunc
		if isFirstRun {
			// First run needs a lot of initialization
			runCtx, cancelRun = context.WithCancel(ctx)
		} else {
			runCtx, cancelRun = context.WithTimeout(ctx, timeout)
		}
		defer cancelRun()

		// Wait for all nodes to be ready to start the run
		err = signalAndWaitForAll("start-run-" + runId)
		if err != nil {
			return err
		}

		recordMessage("Starting run %d / %d (%d bytes)", runNum, runCount, fileSize)
		var bsnode *utils.Node
		providerTopic := sync.NewTopic("provider-info", &ProviderInfo{})
		interestTopic := sync.NewTopic("interest-info-"+runId, &InterestInfo{})
		subgraphSuccessorsTopic := sync.NewTopic("subgraph-successors-"+runId, &SuccessorsInfo{})

		network := bsnet.NewFromIpfsHost(h, dht)

		messageListeners := make([]bsnet.Receiver, 0, 1)
		var recorder *utils.MessageRecorder
		if spy && firstSpyMode {
			recorder = utils.NewMessageRecorder(allSpys)
			messageListeners = append(messageListeners, recorder)
		}
		var exploiter *utils.WantForwardExploiter
		if spy && forwardExploiterMode {
			exploiter = utils.NewWantForwardExploiter(*host.InfoFromHost(h), network)
			messageListeners = append(messageListeners, exploiter)
		}
		var awareExploiter *utils.SubgraphAwareWantForwardExploiter
		if spy && subgraphAwareExploiterMode {
			awareExploiter = utils.NewSubgraphAwareWantForwardExploiter(*host.InfoFromHost(h), network)
			messageListeners = append(messageListeners, awareExploiter)
		}

		// Add values for configurable parameters
		var bsOptions []bs.Option
		bsOptions = append(bsOptions, bs.UnforwardedSearchDelay(unforwardedSearchTime))
		bsOptions = append(bsOptions, bs.ProxyPhaseTransitionProbability(proxyTransitionProb))
		bsOptions = append(bsOptions, bs.SetForwardGraphDegree(forwardGraphDegree))
		// If not enabled, RaWa-Bitswap uses bsfs.RandomForward by default
		if closestPeeridForward {
			bsOptions = append(bsOptions, bs.SetForwardStrategy(bsfs.NewCloseCidForward()))
		}

		// Create a new bitswap node from the blockstore
		bsnode, err = utils.CreateBitswapNode(runCtx, network, h, bstore, messageListeners, bsOptions...)
		if err != nil {
			return fmt.Errorf("error during bs node creation: %s", err)
		}
		if isFirstRun {
			if provider {
				recordMessage("Generating seed data of %d bytes", fileSize)

				tmpFile := utils.RandReader(fileSize, initCtx.GlobalSeq)
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

			if spy && (forwardExploiterMode || subgraphAwareExploiterMode) {
				// Generate all blocks to satisfy requestors
				for i := 1; i <= runenv.TestInstanceCount; i++ {
					if i >= 3 && i <= 2+spyCount {
						continue
					}
					tmpFile := utils.RandReader(fileSize, int64(i))
					ipldNode, err := bsnode.Add(runCtx, tmpFile)
					if err != nil {
						return fmt.Errorf("failed to set up spy: %w", err)
					}
					recordMessage("Spy generated block for cid: %s", ipldNode.Cid())
				}
			}

			recordMessage("Accessing root cid and provider information for stub dht")
			// Get the provider info
			providerInfoCh := make(chan *ProviderInfo)
			sctx, cancelProviderInfoSub := context.WithCancel(runCtx)
			if _, err := client.Subscribe(sctx, providerTopic, providerInfoCh); err != nil {
				cancelProviderInfoSub()
				return fmt.Errorf("failed to subscribe to providerTopic %w", err)
			}
			for i := 1; i <= seedLeechCount; i++ {
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

		var ownInterest cid.Cid
		if requestor {
			// Select random Cid from the available ones
			chosen := ownProviderInfo.Cid
			for chosen == ownProviderInfo.Cid {
				perm := r.Perm(len(allProviderInfos))
				chosen = allProviderInfos[perm[0]].Cid
			}
			ownInterest = chosen
			recordMessage("Chosen cid to fetch: %s", ownInterest)

			// Inform other nodes of the interest
			if _, err = client.Publish(runCtx, interestTopic, &InterestInfo{Cid: ownInterest, Peer: h.ID()}); err != nil {
				return fmt.Errorf("failed to get Redis Sync interestTopic %w", err)
			}
		}

		var correctInterests map[peer.ID]cid.Cid
		if spy {
			recordMessage("Accessing interest infos")
			correctInterests = make(map[peer.ID]cid.Cid, seedLeechCount)
			allInterestInfos := make([]InterestInfo, 0, seedLeechCount)
			interestInfoCh := make(chan *InterestInfo)
			sctx, cancelInterestInfoSub := context.WithCancel(runCtx)
			if _, err := client.Subscribe(sctx, interestTopic, interestInfoCh); err != nil {
				cancelInterestInfoSub()
				return fmt.Errorf("failed to subscribe to interestTopic %w", err)
			}
			for i := 1; i <= seedLeechCount; i++ {
				info, ok := <-interestInfoCh
				if !ok {
					cancelInterestInfoSub()
					return fmt.Errorf("subscription to interestTopic closed")
				}
				allInterestInfos = append(allInterestInfos, *info)
			}
			cancelInterestInfoSub()

			for _, interest := range allInterestInfos {
				correctInterests[interest.Peer] = interest.Cid
			}

			recordMessage("Accessing interest information done")
		}

		// Wait for all nodes to be ready to dial
		err = signalAndWaitForAll("ready-to-connect-" + runId)
		if err != nil {
			return err
		}

		// Dial connPerNode peers
		var err error
		var dialed []peer.AddrInfo
		if spy && firstSpyMode {
			// connects to all peers
			dialed, err = utils.SpyDialAllPeers(runCtx, h, addrInfos, allSpys)
		} else if spy && (forwardExploiterMode || subgraphAwareExploiterMode) {
			dialed, err = utils.SpyDialPeersDeterministically(runCtx, h, addrInfos, allSpys, connPerNode)
		} else {
			dialed, err = utils.DialOtherPeers(runCtx, h, addrInfos, allSpys, connPerNode, r.Int63())
		}
		if err != nil {
			return fmt.Errorf("error during peer dial: %s", err)
		}
		recordMessage("Dialed %d other nodes (run %d)", len(dialed), runNum)

		// Wait for all nodes to be connected
		err = signalAndWaitForAll("connect-complete-" + runId)
		if err != nil {
			return err
		}

		// Shuffle successors
		successors := bsnode.Bitswap.SelectNewSuccessors()

		// Inform other nodes of the successors
		if _, err = client.Publish(runCtx, subgraphSuccessorsTopic, &SuccessorsInfo{Successors: successors, Peer: h.ID()}); err != nil {
			return fmt.Errorf("failed to get Redis Sync interestTopic %w", err)
		}

		err = signalAndWaitForAll("ready-to-download-" + runId)
		if err != nil {
			return err
		}

		/// --- Start test
		recordMessage("Test start (run %d)", runNum)
		if requestor {
			recordMessage("Starting fetch (run %d)", runNum)
			recordMessage("Fetching cid %s", ownInterest)
			start := time.Now()
			err := bsnode.FetchGraph(runCtx, ownInterest)
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

		spyClassificationTopic := sync.NewTopic(
			"spy-classification-"+runId,
			&SingleSpyClassification{},
		)
		if spy && forwardExploiterMode {
			obsC := make([]cid.Cid, 0, len(exploiter.ObservedCids))
			for c := range exploiter.ObservedCids {
				obsC = append(obsC, c)
			}
			class := make([]InterestInfo, 0, len(exploiter.WantBlockFromPeer))
			for p, c := range exploiter.WantBlockFromPeer {
				class = append(class, InterestInfo{Cid: c, Peer: p})
			}
			_, err = client.Publish(
				runCtx,
				spyClassificationTopic,
				&SingleSpyClassification{ObservedCids: obsC, Classification: class},
			)
			if err != nil {
				return fmt.Errorf("failed to get Redis Sync spyClassificationTopic %w", err)
			}
			recordMessage("Published spy data, %d classifications and %d observed cids", len(class), len(obsC))
		} else if spy && subgraphAwareExploiterMode {
			obsC := make([]cid.Cid, 0, len(awareExploiter.ObservedCids))
			for c := range awareExploiter.ObservedCids {
				obsC = append(obsC, c)
			}
			class := make([]InterestInfo, 0, len(awareExploiter.WantBlockFromPeer))
			for p, c := range awareExploiter.WantBlockFromPeer {
				class = append(class, InterestInfo{Cid: c, Peer: p})
			}
			proxies := make([]InterestInfo, 0, len(awareExploiter.WantHaveFromPeer))
			for p, c := range awareExploiter.WantHaveFromPeer {
				proxies = append(proxies, InterestInfo{Cid: c, Peer: p})
			}
			_, err = client.Publish(
				runCtx,
				spyClassificationTopic,
				&SingleSpyClassification{ObservedCids: obsC, Classification: class, ObservedProxies: proxies},
			)
			if err != nil {
				return fmt.Errorf("failed to get Redis Sync spyClassificationTopic %w", err)
			}
			recordMessage("Published spy data, %d classifications, %d observed cids, and %d observed proxies", len(class), len(obsC), len(proxies))
		}

		// Calculate privacy metric on one node
		if spy && initCtx.GlobalSeq == 3 {
			var classification map[peer.ID]cid.Cid
			var spyTypeStr string
			if firstSpyMode {
				classification = classifyByFirstSpyEstimator(recorder, correctInterests, r.Int63())
				spyTypeStr = "first-spy-estimator"
			} else if forwardExploiterMode || subgraphAwareExploiterMode {
				recordMessage("Accessing other spys classification data")
				allSpyClassifications := make([]SingleSpyClassification, 0, spyCount)
				spyClassificationCh := make(chan *SingleSpyClassification)
				sctx, cancelSpyClassificationSub := context.WithCancel(runCtx)
				if _, err := client.Subscribe(sctx, spyClassificationTopic, spyClassificationCh); err != nil {
					cancelSpyClassificationSub()
					return fmt.Errorf("failed to subscribe to spyClassificationTopic %w", err)
				}
				for i := 1; i <= spyCount; i++ {
					singleClassification, ok := <-spyClassificationCh
					if !ok {
						cancelSpyClassificationSub()
						return fmt.Errorf("subscription to spyClassificationTopic closed")
					}
					allSpyClassifications = append(allSpyClassifications, *singleClassification)
				}
				cancelSpyClassificationSub()
				recordMessage("Accessing other spys classification data done")

				if forwardExploiterMode {
					wantBlockFromPeer := make(map[peer.ID]cid.Cid)
					observedCids := make(map[cid.Cid]struct{})
					for _, singleClassification := range allSpyClassifications {
						for _, i := range singleClassification.Classification {
							wantBlockFromPeer[i.Peer] = i.Cid
						}
						for _, c := range singleClassification.ObservedCids {
							observedCids[c] = struct{}{}
						}
					}
					recordMessage("Classification data merged")

					classification = classifyByWantForwardExploiter(wantBlockFromPeer, observedCids, correctInterests, r.Int63())
					spyTypeStr = "want-forward-exploiter"
				} else if subgraphAwareExploiterMode {
					wantBlockFromPeer := make(map[peer.ID]cid.Cid)
					observedCids := make(map[cid.Cid]struct{})
					wantHaveFromPeer := make(map[peer.ID]cid.Cid)
					for _, singleClassification := range allSpyClassifications {
						for _, i := range singleClassification.Classification {
							wantBlockFromPeer[i.Peer] = i.Cid
						}
						for _, c := range singleClassification.ObservedCids {
							observedCids[c] = struct{}{}
						}
						for _, wantHave := range singleClassification.ObservedProxies {
							if _, ok := wantHaveFromPeer[wantHave.Peer]; ok {
								// Prefer earlier proxied want-haves
								continue
							}
							wantHaveFromPeer[wantHave.Peer] = wantHave.Cid
						}

					}
					recordMessage("Classification data merged")

					recordMessage("Accessing subgraph topology")
					allSuccessorsInfo := make([]SuccessorsInfo, 0, runenv.TestInstanceCount)
					allSuccessorsCh := make(chan *SuccessorsInfo)
					sctx, cancelSuccessorsInfoSub := context.WithCancel(runCtx)
					if _, err := client.Subscribe(sctx, subgraphSuccessorsTopic, allSuccessorsCh); err != nil {
						cancelSuccessorsInfoSub()
						return fmt.Errorf("failed to subscribe to subgraphSuccessorsTopic %w", err)
					}
					for i := 1; i <= runenv.TestInstanceCount; i++ {
						succs, ok := <-allSuccessorsCh
						if !ok {
							cancelSuccessorsInfoSub()
							return fmt.Errorf("subscription to subgraphSuccessorsTopic closed")
						}
						allSuccessorsInfo = append(allSuccessorsInfo, *succs)
					}
					cancelSuccessorsInfoSub()
					subgraphTopology := make(map[peer.ID]SubgraphConnections, runenv.TestInstanceCount)
					for _, si := range allSuccessorsInfo {
						if _, ok := subgraphTopology[si.Peer]; !ok {
							subgraphTopology[si.Peer] = SubgraphConnections{
								Successors:   make(map[peer.ID]struct{}),
								Predecessors: make(map[peer.ID]struct{}),
							}
						}
						for _, successor := range si.Successors {
							subgraphTopology[si.Peer].Successors[successor] = struct{}{}

							if _, ok := subgraphTopology[successor]; !ok {
								subgraphTopology[successor] = SubgraphConnections{
									Successors:   make(map[peer.ID]struct{}),
									Predecessors: make(map[peer.ID]struct{}),
								}
							}
							subgraphTopology[successor].Predecessors[si.Peer] = struct{}{}
						}
					}
					recordMessage("Accessing subgraph topology done")

					classification = classifyBySubgraphAwareWantForwardExploiter(wantBlockFromPeer, observedCids, wantHaveFromPeer, subgraphTopology, correctInterests, r.Int63())
					spyTypeStr = "subgraph-aware-want-forward-exploiter"
				}
			}
			precision, recall := calculatePrecisionRecall(classification, correctInterests)

			recordMessage(spyTypeStr+" precision: %g", precision)
			recordMessage(spyTypeStr+" recall: %g", recall)
			runenv.R().RecordPoint(spyTypeStr+"-precision", precision)
			runenv.R().RecordPoint(spyTypeStr+"-recall", recall)
		}

		stats, err := bsnode.Bitswap.Stat()
		if err != nil {
			return fmt.Errorf("error getting stats from Bitswap: %w", err)
		}

		// Record unforwarded search triggers
		if requestor {
			runenv.R().RecordPoint("unforwarded-search-counter", float64(stats.UnforwardedSearchCounter))
		}

		// Record proxy distances
		for _, d := range stats.ProxyDistances {
			runenv.R().RecordPoint(fmt.Sprintf("proxy-distance,value=%s", d.String()), 0)
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

		if !(spy && (forwardExploiterMode || subgraphAwareExploiterMode)) {
			// Free up memory by clearing the leech blockstore at the end of each run.
			// Note that explicitly cleaning up the blockstore from the
			// previous run allows it to be GCed.
			if err := utils.ClearBlockstore(runCtx, bstore, ownProviderInfo.Cid); err != nil {
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

func classifyByFirstSpyEstimator(recorder *utils.MessageRecorder, correctInterests map[peer.ID]cid.Cid, randSeed int64) map[peer.ID]cid.Cid {
	// correctInterests is only used to determine which honest peers exist

	classification := make(map[peer.ID]cid.Cid, len(correctInterests))
	for p, c := range recorder.FirstNewCidForPeer {
		classification[p] = c
	}

	observedCids := make([]cid.Cid, 0, len(recorder.ObservedCids))
	for c := range recorder.ObservedCids {
		observedCids = append(observedCids, c)
	}

	// Selected a random cid for any peer not classified yet
	for p := range correctInterests {
		if _, ok := classification[p]; ok {
			continue
		}
		rand.Seed(time.Now().Unix() + randSeed)
		perm := rand.Perm(len(observedCids))
		selected := observedCids[perm[0]]
		classification[p] = selected
	}

	return classification
}

func classifyByWantForwardExploiter(wantBlockFromPeer map[peer.ID]cid.Cid, cids map[cid.Cid]struct{}, correctInterests map[peer.ID]cid.Cid, randSeed int64) map[peer.ID]cid.Cid {
	// correctInterests is only used to determine which honest peers exist

	classification := make(map[peer.ID]cid.Cid, len(correctInterests))
	for p, c := range wantBlockFromPeer {
		classification[p] = c
	}

	observedCids := make([]cid.Cid, 0, len(cids))
	for c := range cids {
		observedCids = append(observedCids, c)
	}

	// Selected a random cid for any peer not classified yet
	for p := range correctInterests {
		if _, ok := classification[p]; ok {
			continue
		}
		rand.Seed(time.Now().Unix() + randSeed)
		perm := rand.Perm(len(observedCids))
		selected := observedCids[perm[0]]
		classification[p] = selected
	}

	return classification
}

func classifyBySubgraphAwareWantForwardExploiter(
	wantBlockFromPeer map[peer.ID]cid.Cid,
	cids map[cid.Cid]struct{},
	wantHaveFromPeer map[peer.ID]cid.Cid,
	subgraphTopology map[peer.ID]SubgraphConnections,
	correctInterests map[peer.ID]cid.Cid,
	randSeed int64,
) map[peer.ID]cid.Cid {
	// correctInterests is only used to determine which honest peers exist

	classification := make(map[peer.ID]cid.Cid, len(correctInterests))
	for p, c := range wantBlockFromPeer {
		classification[p] = c
	}

	// Assign the predecessors of proxies the cid of the proxied want-have
	for proxy, c := range wantHaveFromPeer {
		if _, ok := subgraphTopology[proxy]; !ok {
			continue
		}
		for pred := range subgraphTopology[proxy].Predecessors {
			if _, ok := classification[pred]; ok {
				continue
			}
			classification[pred] = c
		}
	}

	observedCids := make([]cid.Cid, 0, len(cids))
	for c := range cids {
		observedCids = append(observedCids, c)
	}
	// Selected a random cid for any peer not classified yet
	for p := range correctInterests {
		if _, ok := classification[p]; ok {
			continue
		}
		rand.Seed(time.Now().Unix() + randSeed)
		perm := rand.Perm(len(observedCids))
		selected := observedCids[perm[0]]
		classification[p] = selected
	}

	return classification
}

// Returns precision and recall
func calculatePrecisionRecall(classification map[peer.ID]cid.Cid, correctInterests map[peer.ID]cid.Cid) (float64, float64) {
	precisionPerPeer := make([]float64, 0, len(correctInterests))
	recallPerPeer := make([]float64, 0, len(correctInterests))
	for p, correctCid := range correctInterests {
		correct := 0.
		if classification[p] == correctCid {
			correct = 1.
		}
		r := correct

		cidCount := 0.
		for _, k := range classification {
			if k == correctCid {
				cidCount += 1
			}
		}

		d := 0.
		if cidCount > 0 {
			d = correct / cidCount
		}
		precisionPerPeer = append(precisionPerPeer, d)
		recallPerPeer = append(recallPerPeer, r)
	}

	sumPrecisions := 0.
	for _, d := range precisionPerPeer {
		sumPrecisions += d
	}
	precision := sumPrecisions / float64(len(correctInterests))

	sumRecalls := 0.
	for _, r := range recallPerPeer {
		sumRecalls += r
	}
	recall := sumRecalls / float64(len(correctInterests))

	return precision, recall
}

type ProviderInfo struct {
	Cid       cid.Cid
	AddrsInfo peer.AddrInfo
}

type InterestInfo struct {
	Cid  cid.Cid
	Peer peer.ID
}

type SuccessorsInfo struct {
	Successors []peer.ID
	Peer       peer.ID
}

type SingleSpyClassification struct {
	ObservedCids   []cid.Cid
	Classification []InterestInfo
	// The type is misused here as want-haves do not represent an interest.
	// The peer.ID field is the proxy who sent the want-have.
	ObservedProxies []InterestInfo
}

type SubgraphConnections struct {
	Successors   map[peer.ID]struct{}
	Predecessors map[peer.ID]struct{}
}
