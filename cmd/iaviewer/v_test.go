package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReverseParseWeaveKey(t *testing.T) {
	// Test hex-encoded string
	hash, err := ReverseParseWeaveKey("2BF22AD660244C10:E78B32D50BCA0613E6DB55C25294F58252043B23370FE2")
	require.NoError(t, err)

	fmt.Printf("Hash: %x\n", hash)
}
