package mcpcredential

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
)

const (
	CanonicalFileName = "mcp-access-key"
	keyPrefix         = "lsmcp_"
	keySuffixLength   = 43
	keyLength         = len(keyPrefix) + keySuffixLength
	canonicalLength   = keyLength + 1
)

func generateKey(entropy io.Reader) ([]byte, error) {
	random := make([]byte, 32)
	defer clear(random)
	if _, err := io.ReadFull(entropy, random); err != nil {
		return nil, fmt.Errorf("generate MCP access key: %w", err)
	}
	key := make([]byte, keyLength)
	copy(key, keyPrefix)
	base64.RawURLEncoding.Encode(key[len(keyPrefix):], random)
	return key, nil
}

func parseCanonical(content []byte) ([]byte, error) {
	if len(content) != canonicalLength || content[canonicalLength-1] != '\n' {
		return nil, fmt.Errorf("invalid MCP access-key format")
	}
	key := content[:keyLength]
	if string(key[:len(keyPrefix)]) != keyPrefix {
		return nil, fmt.Errorf("invalid MCP access-key format")
	}
	decoded := make([]byte, 32)
	defer clear(decoded)
	n, err := base64.RawURLEncoding.Decode(decoded, key[len(keyPrefix):])
	if err != nil || n != 32 {
		return nil, fmt.Errorf("invalid MCP access-key format")
	}
	canonical := make([]byte, keySuffixLength)
	base64.RawURLEncoding.Encode(canonical, decoded)
	if subtle.ConstantTimeCompare(canonical, key[len(keyPrefix):]) != 1 {
		return nil, fmt.Errorf("invalid MCP access-key format")
	}
	return append([]byte(nil), key...), nil
}

func canonicalBytes(key []byte) []byte {
	result := make([]byte, len(key)+1)
	copy(result, key)
	result[len(key)] = '\n'
	return result
}
