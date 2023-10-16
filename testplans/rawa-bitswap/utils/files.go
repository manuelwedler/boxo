package utils

import (
	"bytes"
	"io"
	"math/rand"
)

func RandReader(len int, seed int64) io.Reader {
	randReader := rand.New(rand.NewSource(seed))
	data := make([]byte, len)
	randReader.Read(data)
	return bytes.NewReader(data)
}
