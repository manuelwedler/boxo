package unixfs

import (
	"bytes"
	"testing"

	proto "github.com/gogo/protobuf/proto"

	pb "github.com/manuelwedler/boxo/ipld/unixfs/pb"
)

func TestFSNode(t *testing.T) {
	fsn := NewFSNode(TFile)
	for i := 0; i < 16; i++ {
		fsn.AddBlockSize(100)
	}
	fsn.RemoveBlockSize(15)

	fsn.SetData(make([]byte, 128))

	b, err := fsn.GetBytes()
	if err != nil {
		t.Fatal(err)
	}

	pbn := new(pb.Data)
	err = proto.Unmarshal(b, pbn)
	if err != nil {
		t.Fatal(err)
	}

	ds, err := DataSize(b)
	if err != nil {
		t.Fatal(err)
	}
	nKids := fsn.NumChildren()
	if nKids != 15 {
		t.Fatal("Wrong number of child nodes")
	}

	if ds != (100*15)+128 {
		t.Fatal("Datasize calculations incorrect!")
	}

	nfsn, err := FSNodeFromBytes(b)
	if err != nil {
		t.Fatal(err)
	}

	if nfsn.FileSize() != (100*15)+128 {
		t.Fatal("fsNode FileSize calculations incorrect")
	}
}

func TestPBdataTools(t *testing.T) {
	raw := []byte{0x00, 0x01, 0x02, 0x17, 0xA1}
	rawPB := WrapData(raw)

	pbDataSize, err := DataSize(rawPB)
	if err != nil {
		t.Fatal(err)
	}

	same := len(raw) == int(pbDataSize)
	if !same {
		t.Fatal("WrapData changes the size of data.")
	}

	rawPBBytes, err := UnwrapData(rawPB)
	if err != nil {
		t.Fatal(err)
	}

	same = bytes.Equal(raw, rawPBBytes)
	if !same {
		t.Fatal("Unwrap failed to produce the correct wrapped data.")
	}

	rawPBdata, err := FSNodeFromBytes(rawPB)
	if err != nil {
		t.Fatal(err)
	}

	isRaw := rawPBdata.Type() == TRaw
	if !isRaw {
		t.Fatal("WrapData does not create pb.Data_Raw!")
	}

	catFile := []byte("Mr_Meowgie.gif")
	catPBfile := FilePBData(catFile, 17)
	catSize, err := DataSize(catPBfile)
	if catSize != 17 {
		t.Fatal("FilePBData is the wrong size.")
	}
	if err != nil {
		t.Fatal(err)
	}

	dirPB := FolderPBData()
	dir, err := FSNodeFromBytes(dirPB)
	isDir := dir.Type() == TDirectory
	if !isDir {
		t.Fatal("FolderPBData does not create a directory!")
	}
	if err != nil {
		t.Fatal(err)
	}
	_, dirErr := DataSize(dirPB)
	if dirErr == nil {
		t.Fatal("DataSize didn't throw an error when taking the size of a directory.")
	}

	catSym, err := SymlinkData("/ipfs/adad123123/meowgie.gif")
	if err != nil {
		t.Fatal(err)
	}

	catSymPB, err := FSNodeFromBytes(catSym)
	isSym := catSymPB.Type() == TSymlink
	if !isSym {
		t.Fatal("Failed to make a Symlink.")
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestSymlinkFilesize(t *testing.T) {
	path := "/ipfs/adad123123/meowgie.gif"
	sym, err := SymlinkData(path)
	if err != nil {
		t.Fatal(err)
	}
	size, err := DataSize(sym)
	if err != nil {
		t.Fatal(err)
	}
	if int(size) != len(path) {
		t.Fatalf("size mismatch: %d != %d", size, len(path))
	}
}

func TestMetadata(t *testing.T) {
	meta := &Metadata{
		MimeType: "audio/aiff",
		Size:     12345,
	}

	_, err := meta.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	metaPB, err := BytesForMetadata(meta)
	if err != nil {
		t.Fatal(err)
	}

	meta, err = MetadataFromBytes(metaPB)
	if err != nil {
		t.Fatal(err)
	}

	mimeAiff := meta.MimeType == "audio/aiff"
	if !mimeAiff {
		t.Fatal("Metadata does not Marshal and Unmarshal properly!")
	}

}

func TestIsDir(t *testing.T) {
	prepares := map[pb.Data_DataType]bool{
		TDirectory: true,
		THAMTShard: true,
		TFile:      false,
		TMetadata:  false,
		TRaw:       false,
		TSymlink:   false,
	}
	for typ, v := range prepares {
		fsn := NewFSNode(typ)
		if fsn.IsDir() != v {
			t.Fatalf("type %v, IsDir() should be %v, but %v", typ, v, fsn.IsDir())
		}
	}
}
