package keystore

import (
	"github.com/ligo-ai/ligo-chain//common"
)

func KeyFileName(keyAddr common.Address) string {
	return keyFileName(keyAddr)
}

func WriteKeyStore(filepath string, keyjson []byte) error {
	return writeKeyFile(filepath, keyjson)
}
