package bitswap

import (
	"time"

	delay "github.com/ipfs/go-ipfs-delay"
	"github.com/manuelwedler/boxo/bitswap/client"
	bsfs "github.com/manuelwedler/boxo/bitswap/forwardstrategy"
	"github.com/manuelwedler/boxo/bitswap/server"
	"github.com/manuelwedler/boxo/bitswap/tracer"
)

type option func(*Bitswap)

// Option is interface{} of server.Option or client.Option or func(*Bitswap)
// wrapped in a struct to gain strong type checking.
type Option struct {
	v interface{}
}

func EngineBlockstoreWorkerCount(count int) Option {
	return Option{server.EngineBlockstoreWorkerCount(count)}
}

func EngineTaskWorkerCount(count int) Option {
	return Option{server.EngineTaskWorkerCount(count)}
}

func MaxOutstandingBytesPerPeer(count int) Option {
	return Option{server.MaxOutstandingBytesPerPeer(count)}
}

func MaxQueuedWantlistEntriesPerPeer(count uint) Option {
	return Option{server.MaxQueuedWantlistEntriesPerPeer(count)}
}

// MaxCidSize only affects the server.
// If it is 0 no limit is applied.
func MaxCidSize(n uint) Option {
	return Option{server.MaxCidSize(n)}
}

func TaskWorkerCount(count int) Option {
	return Option{server.TaskWorkerCount(count)}
}

func ProvideEnabled(enabled bool) Option {
	return Option{server.ProvideEnabled(enabled)}
}

func SetSendDontHaves(send bool) Option {
	return Option{server.SetSendDontHaves(send)}
}

func WithPeerBlockRequestFilter(pbrf server.PeerBlockRequestFilter) Option {
	return Option{server.WithPeerBlockRequestFilter(pbrf)}
}

func WithScoreLedger(scoreLedger server.ScoreLedger) Option {
	return Option{server.WithScoreLedger(scoreLedger)}
}

func WithTargetMessageSize(tms int) Option {
	return Option{server.WithTargetMessageSize(tms)}
}

func WithTaskComparator(comparator server.TaskComparator) Option {
	return Option{server.WithTaskComparator(comparator)}
}

func ProviderSearchDelay(newProvSearchDelay time.Duration) Option {
	return Option{client.ProviderSearchDelay(newProvSearchDelay)}
}

func RebroadcastDelay(newRebroadcastDelay delay.D) Option {
	return Option{client.RebroadcastDelay(newRebroadcastDelay)}
}

func SetSimulateDontHavesOnTimeout(send bool) Option {
	return Option{client.SetSimulateDontHavesOnTimeout(send)}
}

func UnforwardedSearchDelay(delay time.Duration) Option {
	return Option{client.UnforwardedSearchDelay(delay)}
}

func ProxyPhaseTransitionProbability(probability float64) Option {
	return Option{client.ProxyPhaseTransitionProbability(probability)}
}

func SetForwardGraphDegree(degree uint64) Option {
	return Option{client.SetForwardGraphDegree(degree)}
}

func SetForwardStrategy(strategy bsfs.ForwardStrategy) Option {
	return Option{client.SetForwardStrategy(strategy)}
}

func WithTracer(tap tracer.Tracer) Option {
	// Only trace the server, both receive the same messages anyway
	return Option{
		option(func(bs *Bitswap) {
			bs.tracer = tap
		}),
	}
}
