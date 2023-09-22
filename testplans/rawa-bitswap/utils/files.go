package utils

import (
	"bytes"
	"io"
	"math/rand"
)

var randReader *rand.Rand

func RandReader(len int, seed int64) io.Reader {
	if randReader == nil {
		randReader = rand.New(rand.NewSource(seed))
	}
	data := make([]byte, len)
	randReader.Read(data)
	return bytes.NewReader(data)
}
