package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/ipfs/go-cid"
	"github.com/manuelwedler/boxo/blockservice"
	"github.com/manuelwedler/boxo/examples/gateway/common"
	offline "github.com/manuelwedler/boxo/exchange/offline"
	"github.com/manuelwedler/boxo/gateway"
	carblockstore "github.com/manuelwedler/boxo/ipld/car/v2/blockstore"
)

func main() {
	carFilePtr := flag.String("c", "", "path to CAR file to back this gateway from")
	port := flag.Int("p", 8040, "port to run this gateway from")
	flag.Parse()

	blockService, roots, f, err := newBlockServiceFromCAR(*carFilePtr)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	gwAPI, err := gateway.NewBlocksGateway(blockService)
	if err != nil {
		log.Fatal(err)
	}

	handler := common.NewHandler(gwAPI)

	log.Printf("Listening on http://localhost:%d", *port)
	log.Printf("Metrics available at http://127.0.0.1:%d/debug/metrics/prometheus", *port)
	for _, cid := range roots {
		log.Printf("Hosting CAR root at http://localhost:%d/ipfs/%s", *port, cid.String())
	}

	if err := http.ListenAndServe(":"+strconv.Itoa(*port), handler); err != nil {
		log.Fatal(err)
	}
}

func newBlockServiceFromCAR(filepath string) (blockservice.BlockService, []cid.Cid, io.Closer, error) {
	r, err := os.Open(filepath)
	if err != nil {
		return nil, nil, nil, err
	}

	bs, err := carblockstore.NewReadOnly(r, nil)
	if err != nil {
		_ = r.Close()
		return nil, nil, nil, err
	}

	roots, err := bs.Roots()
	if err != nil {
		return nil, nil, nil, err
	}

	blockService := blockservice.New(bs, offline.Exchange(bs))
	return blockService, roots, r, nil
}
