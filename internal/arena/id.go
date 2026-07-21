package arena

import (
	"crypto/rand"
	"encoding/hex"
)

func newID(prefix string) string {
	var bytes [8]byte
	_, _ = rand.Read(bytes[:])
	return prefix + "_" + hex.EncodeToString(bytes[:])
}
