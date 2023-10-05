package migrate

import (
	"encoding/json"
	"fmt"
	"io"
)

type Config struct {
	ImportPaths map[string]string
	Modules     []string
}

var DefaultConfig = Config{
	ImportPaths: map[string]string{
		"github.com/ipfs/go-bitswap":                     "github.com/manuelwedler/boxo/bitswap",
		"github.com/ipfs/go-ipfs-files":                  "github.com/manuelwedler/boxo/files",
		"github.com/ipfs/tar-utils":                      "github.com/manuelwedler/boxo/tar",
		"github.com/ipfs/interface-go-ipfs-core":         "github.com/manuelwedler/boxo/coreiface",
		"github.com/ipfs/go-unixfs":                      "github.com/manuelwedler/boxo/ipld/unixfs",
		"github.com/ipfs/go-pinning-service-http-client": "github.com/manuelwedler/boxo/pinning/remote/client",
		"github.com/ipfs/go-path":                        "github.com/manuelwedler/boxo/path",
		"github.com/ipfs/go-namesys":                     "github.com/manuelwedler/boxo/namesys",
		"github.com/ipfs/go-mfs":                         "github.com/manuelwedler/boxo/mfs",
		"github.com/ipfs/go-ipfs-provider":               "github.com/manuelwedler/boxo/provider",
		"github.com/ipfs/go-ipfs-pinner":                 "github.com/manuelwedler/boxo/pinning/pinner",
		"github.com/ipfs/go-ipfs-keystore":               "github.com/manuelwedler/boxo/keystore",
		"github.com/ipfs/go-filestore":                   "github.com/manuelwedler/boxo/filestore",
		"github.com/ipfs/go-ipns":                        "github.com/manuelwedler/boxo/ipns",
		"github.com/ipfs/go-blockservice":                "github.com/manuelwedler/boxo/blockservice",
		"github.com/ipfs/go-ipfs-chunker":                "github.com/manuelwedler/boxo/chunker",
		"github.com/ipfs/go-fetcher":                     "github.com/manuelwedler/boxo/fetcher",
		"github.com/ipfs/go-ipfs-blockstore":             "github.com/manuelwedler/boxo/blockstore",
		"github.com/ipfs/go-ipfs-posinfo":                "github.com/manuelwedler/boxo/filestore/posinfo",
		"github.com/ipfs/go-ipfs-util":                   "github.com/manuelwedler/boxo/util",
		"github.com/ipfs/go-ipfs-ds-help":                "github.com/manuelwedler/boxo/datastore/dshelp",
		"github.com/ipfs/go-verifcid":                    "github.com/manuelwedler/boxo/verifcid",
		"github.com/ipfs/go-ipfs-exchange-offline":       "github.com/manuelwedler/boxo/exchange/offline",
		"github.com/ipfs/go-ipfs-routing":                "github.com/manuelwedler/boxo/routing",
		"github.com/ipfs/go-ipfs-exchange-interface":     "github.com/manuelwedler/boxo/exchange",
		"github.com/ipfs/go-merkledag":                   "github.com/manuelwedler/boxo/ipld/merkledag",
		"github.com/ipld/go-car":                         "github.com/manuelwedler/boxo/ipld/car",

		// Pre Boxo rename
		"github.com/ipfs/go-libipfs/gateway":               "github.com/manuelwedler/boxo/gateway",
		"github.com/ipfs/go-libipfs/bitswap":               "github.com/manuelwedler/boxo/bitswap",
		"github.com/ipfs/go-libipfs/files":                 "github.com/manuelwedler/boxo/files",
		"github.com/ipfs/go-libipfs/tar":                   "github.com/manuelwedler/boxo/tar",
		"github.com/ipfs/go-libipfs/coreiface":             "github.com/manuelwedler/boxo/coreiface",
		"github.com/ipfs/go-libipfs/unixfs":                "github.com/manuelwedler/boxo/ipld/unixfs",
		"github.com/ipfs/go-libipfs/pinning/remote/client": "github.com/manuelwedler/boxo/pinning/remote/client",
		"github.com/ipfs/go-libipfs/path":                  "github.com/manuelwedler/boxo/path",
		"github.com/ipfs/go-libipfs/namesys":               "github.com/manuelwedler/boxo/namesys",
		"github.com/ipfs/go-libipfs/mfs":                   "github.com/manuelwedler/boxo/mfs",
		"github.com/ipfs/go-libipfs/provider":              "github.com/manuelwedler/boxo/provider",
		"github.com/ipfs/go-libipfs/pinning/pinner":        "github.com/manuelwedler/boxo/pinning/pinner",
		"github.com/ipfs/go-libipfs/keystore":              "github.com/manuelwedler/boxo/keystore",
		"github.com/ipfs/go-libipfs/filestore":             "github.com/manuelwedler/boxo/filestore",
		"github.com/ipfs/go-libipfs/ipns":                  "github.com/manuelwedler/boxo/ipns",
		"github.com/ipfs/go-libipfs/blockservice":          "github.com/manuelwedler/boxo/blockservice",
		"github.com/ipfs/go-libipfs/chunker":               "github.com/manuelwedler/boxo/chunker",
		"github.com/ipfs/go-libipfs/fetcher":               "github.com/manuelwedler/boxo/fetcher",
		"github.com/ipfs/go-libipfs/blockstore":            "github.com/manuelwedler/boxo/blockstore",
		"github.com/ipfs/go-libipfs/filestore/posinfo":     "github.com/manuelwedler/boxo/filestore/posinfo",
		"github.com/ipfs/go-libipfs/util":                  "github.com/manuelwedler/boxo/util",
		"github.com/ipfs/go-libipfs/datastore/dshelp":      "github.com/manuelwedler/boxo/datastore/dshelp",
		"github.com/ipfs/go-libipfs/verifcid":              "github.com/manuelwedler/boxo/verifcid",
		"github.com/ipfs/go-libipfs/exchange/offline":      "github.com/manuelwedler/boxo/exchange/offline",
		"github.com/ipfs/go-libipfs/routing":               "github.com/manuelwedler/boxo/routing",
		"github.com/ipfs/go-libipfs/exchange":              "github.com/manuelwedler/boxo/exchange",

		// Unmigrated things
		"github.com/ipfs/go-libipfs/blocks":   "github.com/ipfs/go-block-format",
		"github.com/manuelwedler/boxo/blocks": "github.com/ipfs/go-block-format",
	},
	Modules: []string{
		"github.com/ipfs/go-bitswap",
		"github.com/ipfs/go-ipfs-files",
		"github.com/ipfs/tar-utils",
		"gihtub.com/ipfs/go-block-format",
		"github.com/ipfs/interface-go-ipfs-core",
		"github.com/ipfs/go-unixfs",
		"github.com/ipfs/go-pinning-service-http-client",
		"github.com/ipfs/go-path",
		"github.com/ipfs/go-namesys",
		"github.com/ipfs/go-mfs",
		"github.com/ipfs/go-ipfs-provider",
		"github.com/ipfs/go-ipfs-pinner",
		"github.com/ipfs/go-ipfs-keystore",
		"github.com/ipfs/go-filestore",
		"github.com/ipfs/go-ipns",
		"github.com/ipfs/go-blockservice",
		"github.com/ipfs/go-ipfs-chunker",
		"github.com/ipfs/go-fetcher",
		"github.com/ipfs/go-ipfs-blockstore",
		"github.com/ipfs/go-ipfs-posinfo",
		"github.com/ipfs/go-ipfs-util",
		"github.com/ipfs/go-ipfs-ds-help",
		"github.com/ipfs/go-verifcid",
		"github.com/ipfs/go-ipfs-exchange-offline",
		"github.com/ipfs/go-ipfs-routing",
		"github.com/ipfs/go-ipfs-exchange-interface",
		"github.com/ipfs/go-libipfs",
	},
}

func ReadConfig(r io.Reader) (Config, error) {
	var config Config
	err := json.NewDecoder(r).Decode(&config)
	if err != nil {
		return Config{}, fmt.Errorf("reading and decoding config: %w", err)
	}
	return config, nil
}
