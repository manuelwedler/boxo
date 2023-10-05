package tracer

import (
	peer "github.com/libp2p/go-libp2p/core/peer"
	bsmsg "github.com/manuelwedler/boxo/bitswap/message"
)

// Tracer provides methods to access all messages sent and received by Bitswap.
// This interface can be used to implement various statistics (this is original intent).
type Tracer interface {
	MessageReceived(peer.ID, bsmsg.BitSwapMessage)
	MessageSent(peer.ID, bsmsg.BitSwapMessage)
}
