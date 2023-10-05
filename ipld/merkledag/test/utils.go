package mdutils

import (
	dag "github.com/manuelwedler/boxo/ipld/merkledag"

	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	ipld "github.com/ipfs/go-ipld-format"
	bsrv "github.com/manuelwedler/boxo/blockservice"
	blockstore "github.com/manuelwedler/boxo/blockstore"
	offline "github.com/manuelwedler/boxo/exchange/offline"
)

// Mock returns a new thread-safe, mock DAGService.
func Mock() ipld.DAGService {
	return dag.NewDAGService(Bserv())
}

// Bserv returns a new, thread-safe, mock BlockService.
func Bserv() bsrv.BlockService {
	bstore := blockstore.NewBlockstore(dssync.MutexWrap(ds.NewMapDatastore()))
	return bsrv.New(bstore, offline.Exchange(bstore))
}
