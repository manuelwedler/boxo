package tests

import (
	"context"
	"math"
	gopath "path"
	"strings"
	"testing"

	path "github.com/manuelwedler/boxo/coreiface/path"

	coreiface "github.com/manuelwedler/boxo/coreiface"

	ipldcbor "github.com/ipfs/go-ipld-cbor"
	ipld "github.com/ipfs/go-ipld-format"
	mh "github.com/multiformats/go-multihash"
)

func (tp *TestSuite) TestDag(t *testing.T) {
	tp.hasApi(t, func(api coreiface.CoreAPI) error {
		if api.Dag() == nil {
			return errAPINotImplemented
		}
		return nil
	})

	t.Run("TestPut", tp.TestPut)
	t.Run("TestPutWithHash", tp.TestPutWithHash)
	t.Run("TestPath", tp.TestDagPath)
	t.Run("TestTree", tp.TestTree)
	t.Run("TestBatch", tp.TestBatch)
}

var (
	treeExpected = map[string]struct{}{
		"a":   {},
		"b":   {},
		"c":   {},
		"c/d": {},
		"c/e": {},
	}
)

func (tp *TestSuite) TestPut(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api, err := tp.makeAPI(ctx)
	if err != nil {
		t.Fatal(err)
	}

	nd, err := ipldcbor.FromJSON(strings.NewReader(`"Hello"`), math.MaxUint64, -1)
	if err != nil {
		t.Fatal(err)
	}

	err = api.Dag().Add(ctx, nd)
	if err != nil {
		t.Fatal(err)
	}

	if nd.Cid().String() != "bafyreicnga62zhxnmnlt6ymq5hcbsg7gdhqdu6z4ehu3wpjhvqnflfy6nm" {
		t.Errorf("got wrong cid: %s", nd.Cid().String())
	}
}

func (tp *TestSuite) TestPutWithHash(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api, err := tp.makeAPI(ctx)
	if err != nil {
		t.Fatal(err)
	}

	nd, err := ipldcbor.FromJSON(strings.NewReader(`"Hello"`), mh.SHA3_256, -1)
	if err != nil {
		t.Fatal(err)
	}

	err = api.Dag().Add(ctx, nd)
	if err != nil {
		t.Fatal(err)
	}

	if nd.Cid().String() != "bafyrmifu7haikttpqqgc5ewvmp76z3z4ebp7h2ph4memw7dq4nt6btmxny" {
		t.Errorf("got wrong cid: %s", nd.Cid().String())
	}
}

func (tp *TestSuite) TestDagPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api, err := tp.makeAPI(ctx)
	if err != nil {
		t.Fatal(err)
	}

	snd, err := ipldcbor.FromJSON(strings.NewReader(`"foo"`), math.MaxUint64, -1)
	if err != nil {
		t.Fatal(err)
	}

	err = api.Dag().Add(ctx, snd)
	if err != nil {
		t.Fatal(err)
	}

	nd, err := ipldcbor.FromJSON(strings.NewReader(`{"lnk": {"/": "`+snd.Cid().String()+`"}}`), math.MaxUint64, -1)
	if err != nil {
		t.Fatal(err)
	}

	err = api.Dag().Add(ctx, nd)
	if err != nil {
		t.Fatal(err)
	}

	p := path.New(gopath.Join(nd.Cid().String(), "lnk"))

	rp, err := api.ResolvePath(ctx, p)
	if err != nil {
		t.Fatal(err)
	}

	ndd, err := api.Dag().Get(ctx, rp.Cid())
	if err != nil {
		t.Fatal(err)
	}

	if ndd.Cid().String() != snd.Cid().String() {
		t.Errorf("got unexpected cid %s, expected %s", ndd.Cid().String(), snd.Cid().String())
	}
}

func (tp *TestSuite) TestTree(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api, err := tp.makeAPI(ctx)
	if err != nil {
		t.Fatal(err)
	}

	nd, err := ipldcbor.FromJSON(strings.NewReader(`{"a": 123, "b": "foo", "c": {"d": 321, "e": 111}}`), math.MaxUint64, -1)
	if err != nil {
		t.Fatal(err)
	}

	err = api.Dag().Add(ctx, nd)
	if err != nil {
		t.Fatal(err)
	}

	res, err := api.Dag().Get(ctx, nd.Cid())
	if err != nil {
		t.Fatal(err)
	}

	lst := res.Tree("", -1)
	if len(lst) != len(treeExpected) {
		t.Errorf("tree length of %d doesn't match expected %d", len(lst), len(treeExpected))
	}

	for _, ent := range lst {
		if _, ok := treeExpected[ent]; !ok {
			t.Errorf("unexpected tree entry %s", ent)
		}
	}
}

func (tp *TestSuite) TestBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api, err := tp.makeAPI(ctx)
	if err != nil {
		t.Fatal(err)
	}

	nd, err := ipldcbor.FromJSON(strings.NewReader(`"Hello"`), math.MaxUint64, -1)
	if err != nil {
		t.Fatal(err)
	}

	if nd.Cid().String() != "bafyreicnga62zhxnmnlt6ymq5hcbsg7gdhqdu6z4ehu3wpjhvqnflfy6nm" {
		t.Errorf("got wrong cid: %s", nd.Cid().String())
	}

	_, err = api.Dag().Get(ctx, nd.Cid())
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatal(err)
	}

	if err := api.Dag().AddMany(ctx, []ipld.Node{nd}); err != nil {
		t.Fatal(err)
	}

	_, err = api.Dag().Get(ctx, nd.Cid())
	if err != nil {
		t.Fatal(err)
	}
}
