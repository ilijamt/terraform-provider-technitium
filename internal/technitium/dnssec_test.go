package technitium

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitAlgorithm(t *testing.T) {
	for name, want := range map[string][3]string{
		"ECDSAP256SHA256":  {"ECDSA", "P256", ""},
		"ECDSAP384SHA384":  {"ECDSA", "P384", ""},
		"ED25519":          {"EDDSA", "ED25519", ""},
		"ED448":            {"EDDSA", "ED448", ""},
		"RSASHA256":        {"RSA", "", "SHA256"},
		"RSASHA512":        {"RSA", "", "SHA512"},
		"RSASHA1":          {"RSA", "", "SHA1"},
		"RSASHA1NSEC3SHA1": {"RSA", "", "SHA1"},
		"RSAMD5":           {"RSA", "", "MD5"},
	} {
		a, c, h := splitAlgorithm(name)
		assert.Equal(t, want, [3]string{a, c, h}, name)
	}
}

func TestNum(t *testing.T) {
	assert.Equal(t, int64(13), num(json.Number("13")).ValueInt64())
	assert.Equal(t, int64(2), num("2").ValueInt64(), "digest type numbers come as strings")
	assert.True(t, num(nil).IsNull())
	assert.True(t, num("x").IsNull())
}
