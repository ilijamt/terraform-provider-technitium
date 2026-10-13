package framework

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecord(t *testing.T) {
	s := Record("MX",
		Attr{Name: "exchange", Param: "exchange", Type: String, Key: "rData.exchange", Identity: true},
		Attr{Name: "preference", Param: "preference", Type: Int64, Key: "rData.preference"},
	)

	assert.Equal(t, "record_mx", s.Name)
	assert.Equal(t, map[string]string{"type": "MX"}, s.Fixed)
	assert.Equal(t, "records", s.List)

	var names []string
	for _, a := range s.Attrs {
		names = append(names, a.Name)
	}
	assert.Equal(t, []string{"zone", "domain", "ttl", "comments", "expiry_ttl", "exchange", "preference", "disable"}, names)

	for _, a := range s.Attrs[5:7] {
		assert.True(t, a.Create && a.Update, "%s is accepted by add and update", a.Name)
	}
	assert.Equal(t, []string{"zone", "domain", "exchange"}, func() (out []string) {
		for _, a := range s.identity() {
			out = append(out, a.Name)
		}
		return out
	}())
}
