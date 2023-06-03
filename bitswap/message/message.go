package message

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/ipfs/boxo/bitswap/client/wantlist"
	pb "github.com/ipfs/boxo/bitswap/message/pb"

	blocks "github.com/ipfs/go-block-format"
	cid "github.com/ipfs/go-cid"
	pool "github.com/libp2p/go-buffer-pool"
	peer "github.com/libp2p/go-libp2p/core/peer"
	msgio "github.com/libp2p/go-msgio"

	u "github.com/ipfs/boxo/util"
	"github.com/libp2p/go-libp2p/core/network"
)

// BitSwapMessage is the basic interface for interacting building, encoding,
// and decoding messages sent on the BitSwap protocol.
type BitSwapMessage interface {
	// Wantlist returns a slice of unique keys that represent data wanted by
	// the sender.
	Wantlist() []Entry

	Forwardlist() []Entry

	// Blocks returns a slice of unique blocks.
	Blocks() []blocks.Block
	// BlockPresences returns the list of HAVE / DONT_HAVE in the message
	BlockPresences() []BlockPresence
	// Haves returns the Cids for each HAVE
	Haves() []cid.Cid
	// DontHaves returns the Cids for each DONT_HAVE
	DontHaves() []cid.Cid
	// ForwardHaves returns a list of peers that responded with a HAVE per Cid
	ForwardHaves() map[cid.Cid][]peer.ID
	// PendingBytes returns the number of outstanding bytes of data that the
	// engine has yet to send to the client (because they didn't fit in this
	// message)
	PendingBytes() int32

	// AddEntry adds an entry to the Wantlist.
	AddEntry(key cid.Cid, priority int32, wantType pb.Message_Wantlist_WantType, sendDontHave bool) int

	// Cancel adds a CANCEL for the given CID to the message
	// Returns the size of the CANCEL entry in the protobuf
	Cancel(key cid.Cid) int

	// AddForwardEntry adds an entry to the Wantlist that should be proxied. Only supports HAVEs.
	AddForwardEntry(key cid.Cid, priority int32) int

	// CancelForward adds a CANCEL for a forwarded request given by the CID.
	CancelForward(k cid.Cid) int

	// Remove removes any entries for the given CID. Useful when the want
	// status for the CID changes when preparing a message.
	Remove(key cid.Cid)

	// Remove removes any forward entries for the given CID.
	RemoveForward(key cid.Cid)

	// Empty indicates whether the message has any information
	Empty() bool
	// Size returns the size of the message in bytes
	Size() int

	// A full wantlist is an authoritative copy, a 'non-full' wantlist is a patch-set
	Full() bool

	// AddBlock adds a block to the message
	AddBlock(blocks.Block)
	// AddBlockPresence adds a HAVE / DONT_HAVE for the given Cid to the message
	AddBlockPresence(cid.Cid, pb.Message_BlockPresenceType)
	// AddHave adds a HAVE for the given Cid to the message
	AddHave(cid.Cid)
	// AddDontHave adds a DONT_HAVE for the given Cid to the message
	AddDontHave(cid.Cid)
	// AddForwardHave adds a list of peers that have responded with a HAVE for the given Cid
	AddForwardHave(cid.Cid, []peer.ID)
	// SetPendingBytes sets the number of bytes of data that are yet to be sent
	// to the client (because they didn't fit in this message)
	SetPendingBytes(int32)
	Exportable

	Loggable() map[string]interface{}

	// Reset the values in the message back to defaults, so it can be reused
	Reset(bool)

	// Clone the message fields
	Clone() BitSwapMessage
}

// Exportable is an interface for structures than can be
// encoded in a bitswap protobuf.
type Exportable interface {
	// Note that older Bitswap versions use a different wire format, so we need
	// to convert the message to the appropriate format depending on which
	// version of the protocol the remote peer supports.
	ToProtoV0() *pb.Message
	ToProtoV1() *pb.Message
	ToNetV0(w io.Writer) error
	ToNetV1(w io.Writer) error
}

// BlockPresence represents a HAVE / DONT_HAVE for a given Cid
type BlockPresence struct {
	Cid  cid.Cid
	Type pb.Message_BlockPresenceType
}

// Entry is a wantlist entry in a Bitswap message, with flags indicating
// - whether message is a cancel
// - whether requester wants a DONT_HAVE message
// - whether requester wants a HAVE message (instead of the block)
type Entry struct {
	wantlist.Entry
	Cancel       bool
	SendDontHave bool
}

// Get the size of the entry on the wire
func (e *Entry) Size() int {
	epb := e.ToPB()
	return epb.Size()
}

// Get the entry in protobuf form
func (e *Entry) ToPB() pb.Message_Wantlist_Entry {
	return pb.Message_Wantlist_Entry{
		Block:        pb.Cid{Cid: e.Cid},
		Priority:     int32(e.Priority),
		Cancel:       e.Cancel,
		WantType:     e.WantType,
		SendDontHave: e.SendDontHave,
	}
}

var MaxEntrySize = maxEntrySize()

func maxEntrySize() int {
	var maxInt32 int32 = (1 << 31) - 1

	c := cid.NewCidV0(u.Hash([]byte("cid")))
	e := Entry{
		Entry: wantlist.Entry{
			Cid:      c,
			Priority: maxInt32,
			WantType: pb.Message_Wantlist_Forward,
		},
		SendDontHave: true, // true takes up more space than false
		Cancel:       true,
	}
	return e.Size()
}

type impl struct {
	full                bool
	wantlist            map[cid.Cid]*Entry
	forwardlist         map[cid.Cid]*Entry
	blocks              map[cid.Cid]blocks.Block
	blockPresences      map[cid.Cid]pb.Message_BlockPresenceType
	blockPresencesPeers map[cid.Cid][]peer.ID
	pendingBytes        int32
}

// New returns a new, empty bitswap message
func New(full bool) BitSwapMessage {
	return newMsg(full)
}

func newMsg(full bool) *impl {
	return &impl{
		full:                full,
		wantlist:            make(map[cid.Cid]*Entry),
		forwardlist:         make(map[cid.Cid]*Entry),
		blocks:              make(map[cid.Cid]blocks.Block),
		blockPresences:      make(map[cid.Cid]pb.Message_BlockPresenceType),
		blockPresencesPeers: make(map[cid.Cid][]peer.ID),
	}
}

// Clone the message fields
func (m *impl) Clone() BitSwapMessage {
	msg := newMsg(m.full)
	for k := range m.wantlist {
		msg.wantlist[k] = m.wantlist[k]
	}
	for k := range m.forwardlist {
		msg.forwardlist[k] = m.forwardlist[k]
	}
	for k := range m.blocks {
		msg.blocks[k] = m.blocks[k]
	}
	for k := range m.blockPresences {
		msg.blockPresences[k] = m.blockPresences[k]
	}
	for k := range m.blockPresencesPeers {
		msg.blockPresencesPeers[k] = make([]peer.ID, 0, len(m.blockPresencesPeers[k]))
		msg.blockPresencesPeers[k] = append(msg.blockPresencesPeers[k], m.blockPresencesPeers[k]...)
	}
	msg.pendingBytes = m.pendingBytes
	return msg
}

// Reset the values in the message back to defaults, so it can be reused
func (m *impl) Reset(full bool) {
	m.full = full
	for k := range m.wantlist {
		delete(m.wantlist, k)
	}
	for k := range m.forwardlist {
		delete(m.forwardlist, k)
	}
	for k := range m.blocks {
		delete(m.blocks, k)
	}
	for k := range m.blockPresences {
		delete(m.blockPresences, k)
	}
	for k := range m.blockPresencesPeers {
		delete(m.blockPresencesPeers, k)
	}
	m.pendingBytes = 0
}

var errCidMissing = errors.New("missing cid")

func newMessageFromProto(pbm pb.Message) (BitSwapMessage, error) {
	m := newMsg(pbm.Wantlist.Full)
	for _, e := range pbm.Wantlist.Entries {
		if !e.Block.Cid.Defined() {
			return nil, errCidMissing
		}
		if e.WantType == pb.Message_Wantlist_Forward {
			m.addForwardEntry(e.Block.Cid, e.Priority, e.Cancel)
		} else {
			m.addEntry(e.Block.Cid, e.Priority, e.Cancel, e.WantType, e.SendDontHave)
		}
	}

	// deprecated
	for _, d := range pbm.Blocks {
		// CIDv0, sha256, protobuf only
		b := blocks.NewBlock(d)
		m.AddBlock(b)
	}
	//

	for _, b := range pbm.GetPayload() {
		pref, err := cid.PrefixFromBytes(b.GetPrefix())
		if err != nil {
			return nil, err
		}

		c, err := pref.Sum(b.GetData())
		if err != nil {
			return nil, err
		}

		blk, err := blocks.NewBlockWithCid(b.GetData(), c)
		if err != nil {
			return nil, err
		}

		m.AddBlock(blk)
	}

	for _, bi := range pbm.GetBlockPresences() {
		if !bi.Cid.Cid.Defined() {
			return nil, errCidMissing
		}
		if bi.Type == pb.Message_ForwardHave {
			peers := make([]peer.ID, 0, len(bi.PeerIds))
			for _, bs := range bi.PeerIds {
				p, err := peer.IDFromBytes(bs)
				if err == nil {
					peers = append(peers, p)
				}
			}
			m.AddForwardHave(bi.Cid.Cid, peers)
		} else {
			m.AddBlockPresence(bi.Cid.Cid, bi.Type)
		}
	}

	m.pendingBytes = pbm.PendingBytes

	return m, nil
}

func (m *impl) Full() bool {
	return m.full
}

func (m *impl) Empty() bool {
	return len(m.blocks) == 0 && len(m.wantlist) == 0 && len(m.blockPresences) == 0 && len(m.forwardlist) == 0 && len(m.blockPresencesPeers) == 0
}

func (m *impl) Wantlist() []Entry {
	out := make([]Entry, 0, len(m.wantlist))
	for _, e := range m.wantlist {
		out = append(out, *e)
	}
	return out
}

func (m *impl) Forwardlist() []Entry {
	out := make([]Entry, 0, len(m.forwardlist))
	for _, e := range m.forwardlist {
		out = append(out, *e)
	}
	return out
}

func (m *impl) Blocks() []blocks.Block {
	bs := make([]blocks.Block, 0, len(m.blocks))
	for _, block := range m.blocks {
		bs = append(bs, block)
	}
	return bs
}

func (m *impl) BlockPresences() []BlockPresence {
	bps := make([]BlockPresence, 0, len(m.blockPresences))
	for c, t := range m.blockPresences {
		bps = append(bps, BlockPresence{c, t})
	}
	return bps
}

func (m *impl) Haves() []cid.Cid {
	return m.getBlockPresenceByType(pb.Message_Have)
}

func (m *impl) DontHaves() []cid.Cid {
	return m.getBlockPresenceByType(pb.Message_DontHave)
}

func (m *impl) ForwardHaves() map[cid.Cid][]peer.ID {
	cids := make(map[cid.Cid][]peer.ID)
	for c, ps := range m.blockPresencesPeers {
		if cids[c] == nil {
			cids[c] = make([]peer.ID, 0, len(m.blockPresencesPeers[c]))
		}
		cids[c] = append(cids[c], ps...)
	}
	return cids
}

func (m *impl) getBlockPresenceByType(t pb.Message_BlockPresenceType) []cid.Cid {
	cids := make([]cid.Cid, 0, len(m.blockPresences))
	for c, bpt := range m.blockPresences {
		if bpt == t {
			cids = append(cids, c)
		}
	}
	return cids
}

func (m *impl) PendingBytes() int32 {
	return m.pendingBytes
}

func (m *impl) SetPendingBytes(pendingBytes int32) {
	m.pendingBytes = pendingBytes
}

func (m *impl) Remove(k cid.Cid) {
	delete(m.wantlist, k)
}

func (m *impl) RemoveForward(k cid.Cid) {
	delete(m.forwardlist, k)
}

func (m *impl) Cancel(k cid.Cid) int {
	return m.addEntry(k, 0, true, pb.Message_Wantlist_Block, false)
}

func (m *impl) AddEntry(k cid.Cid, priority int32, wantType pb.Message_Wantlist_WantType, sendDontHave bool) int {
	return m.addEntry(k, priority, false, wantType, sendDontHave)
}

func (m *impl) addEntry(c cid.Cid, priority int32, cancel bool, wantType pb.Message_Wantlist_WantType, sendDontHave bool) int {
	e, exists := m.wantlist[c]
	if exists {
		// Only change priority if want is of the same type
		if e.WantType == wantType {
			e.Priority = priority
		}
		// Only change from "dont cancel" to "do cancel"
		if cancel {
			e.Cancel = cancel
		}
		// Only change from "dont send" to "do send" DONT_HAVE
		if sendDontHave {
			e.SendDontHave = sendDontHave
		}
		// want-block overrides existing want-have
		if wantType == pb.Message_Wantlist_Block && e.WantType == pb.Message_Wantlist_Have {
			e.WantType = wantType
		}
		m.wantlist[c] = e
		return 0
	}

	e = &Entry{
		Entry: wantlist.Entry{
			Cid:      c,
			Priority: priority,
			WantType: wantType,
		},
		SendDontHave: sendDontHave,
		Cancel:       cancel,
	}
	m.wantlist[c] = e

	return e.Size()
}

func (m *impl) CancelForward(k cid.Cid) int {
	return m.addForwardEntry(k, 0, true)
}

func (m *impl) AddForwardEntry(k cid.Cid, priority int32) int {
	return m.addForwardEntry(k, priority, false)
}

func (m *impl) addForwardEntry(c cid.Cid, priority int32, cancel bool) int {
	e, exists := m.forwardlist[c]
	if exists && e.Cancel != cancel {
		e.Cancel = cancel
		m.forwardlist[c] = e
		return 0
	}

	e = &Entry{
		Entry: wantlist.Entry{
			Cid:      c,
			Priority: priority,
			WantType: pb.Message_Wantlist_Forward,
		},
		SendDontHave: false,
		Cancel:       cancel,
	}
	m.forwardlist[c] = e

	return e.Size()
}

func (m *impl) AddBlock(b blocks.Block) {
	delete(m.blockPresences, b.Cid())
	m.blocks[b.Cid()] = b
}

func (m *impl) AddBlockPresence(c cid.Cid, t pb.Message_BlockPresenceType) {
	if _, ok := m.blocks[c]; ok {
		return
	}
	m.blockPresences[c] = t
}

func (m *impl) AddHave(c cid.Cid) {
	m.AddBlockPresence(c, pb.Message_Have)
}

func (m *impl) AddDontHave(c cid.Cid) {
	m.AddBlockPresence(c, pb.Message_DontHave)
}

func (m *impl) AddForwardHave(c cid.Cid, ps []peer.ID) {
	m.blockPresences[c] = pb.Message_ForwardHave
	if m.blockPresencesPeers[c] == nil {
		m.blockPresencesPeers[c] = make([]peer.ID, 0, len(ps))
	}
	m.blockPresencesPeers[c] = append(m.blockPresencesPeers[c], ps...)
}

func (m *impl) Size() int {
	size := 0
	for _, block := range m.blocks {
		size += len(block.RawData())
	}
	for c := range m.blockPresences {
		peers := make([][]byte, 0, len(m.blockPresencesPeers[c]))
		ps, exists := m.blockPresencesPeers[c]
		if exists {
			for _, p := range ps {
				mp, err := p.MarshalBinary()
				if err == nil {
					peers = append(peers, mp)
				}
			}
		}
		presence := &pb.Message_BlockPresence{
			Cid:     pb.Cid{Cid: c},
			Type:    pb.Message_Have,
			PeerIds: peers,
		}
		size += presence.Size()
	}
	for _, e := range m.wantlist {
		size += e.Size()
	}
	for _, e := range m.forwardlist {
		size += e.Size()
	}

	return size
}

func BlockPresenceSize(c cid.Cid) int {
	return (&pb.Message_BlockPresence{
		Cid:  pb.Cid{Cid: c},
		Type: pb.Message_Have,
	}).Size()
}

// FromNet generates a new BitswapMessage from incoming data on an io.Reader.
func FromNet(r io.Reader) (BitSwapMessage, error) {
	reader := msgio.NewVarintReaderSize(r, network.MessageSizeMax)
	return FromMsgReader(reader)
}

// FromPBReader generates a new Bitswap message from a gogo-protobuf reader
func FromMsgReader(r msgio.Reader) (BitSwapMessage, error) {
	msg, err := r.ReadMsg()
	if err != nil {
		return nil, err
	}

	var pb pb.Message
	err = pb.Unmarshal(msg)
	r.ReleaseMsg(msg)
	if err != nil {
		return nil, err
	}

	return newMessageFromProto(pb)
}

func (m *impl) ToProtoV0() *pb.Message {
	pbm := new(pb.Message)
	pbm.Wantlist.Entries = make([]pb.Message_Wantlist_Entry, 0, len(m.wantlist))
	for _, e := range m.wantlist {
		pbm.Wantlist.Entries = append(pbm.Wantlist.Entries, e.ToPB())
	}
	pbm.Wantlist.Full = m.full

	blocks := m.Blocks()
	pbm.Blocks = make([][]byte, 0, len(blocks))
	for _, b := range blocks {
		pbm.Blocks = append(pbm.Blocks, b.RawData())
	}
	return pbm
}

func (m *impl) ToProtoV1() *pb.Message {
	pbm := new(pb.Message)
	pbm.Wantlist.Entries = make([]pb.Message_Wantlist_Entry, 0, len(m.wantlist))
	for _, e := range m.wantlist {
		pbm.Wantlist.Entries = append(pbm.Wantlist.Entries, e.ToPB())
	}
	for _, f := range m.forwardlist {
		pbm.Wantlist.Entries = append(pbm.Wantlist.Entries, f.ToPB())
	}
	pbm.Wantlist.Full = m.full

	blocks := m.Blocks()
	pbm.Payload = make([]pb.Message_Block, 0, len(blocks))
	for _, b := range blocks {
		pbm.Payload = append(pbm.Payload, pb.Message_Block{
			Data:   b.RawData(),
			Prefix: b.Cid().Prefix().Bytes(),
		})
	}

	pbm.BlockPresences = make([]pb.Message_BlockPresence, 0, len(m.blockPresences))
	for c, t := range m.blockPresences {
		peers := make([][]byte, 0, len(m.blockPresencesPeers[c]))
		ps, exists := m.blockPresencesPeers[c]
		if exists {
			for _, p := range ps {
				mp, err := p.Marshal()
				if err == nil {
					peers = append(peers, mp)
				}
			}
		}
		pbm.BlockPresences = append(pbm.BlockPresences, pb.Message_BlockPresence{
			Cid:     pb.Cid{Cid: c},
			Type:    t,
			PeerIds: peers,
		})
	}

	pbm.PendingBytes = m.PendingBytes()

	return pbm
}

func (m *impl) ToNetV0(w io.Writer) error {
	return write(w, m.ToProtoV0())
}

func (m *impl) ToNetV1(w io.Writer) error {
	return write(w, m.ToProtoV1())
}

func write(w io.Writer, m *pb.Message) error {
	size := m.Size()

	buf := pool.Get(size + binary.MaxVarintLen64)
	defer pool.Put(buf)

	n := binary.PutUvarint(buf, uint64(size))

	written, err := m.MarshalTo(buf[n:])
	if err != nil {
		return err
	}
	n += written

	_, err = w.Write(buf[:n])
	return err
}

func (m *impl) Loggable() map[string]interface{} {
	blocks := make([]string, 0, len(m.blocks))
	for _, v := range m.blocks {
		blocks = append(blocks, v.Cid().String())
	}
	return map[string]interface{}{
		"blocks":   blocks,
		"wants":    m.Wantlist(),
		"forwards": m.Forwardlist(),
	}
}
