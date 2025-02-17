package wantlist

import (
	"github.com/ipfs/go-cid"
	"github.com/manuelwedler/boxo/bitswap/client/wantlist"
)

type (
	// Deprecated: use wantlist.Entry instead
	Entry = wantlist.Entry
	// Deprecated: use wantlist.Wantlist instead
	Wantlist = wantlist.Wantlist
)

// Deprecated: use wantlist.New instead
func New() *Wantlist {
	return wantlist.New()
}

// Deprecated: use wantlist.NewRefEntry instead
func NewRefEntry(c cid.Cid, p int32) Entry {
	return wantlist.NewRefEntry(c, p)
}
