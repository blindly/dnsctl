package zone

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseZoneFile(t *testing.T) {
	input := `zone: example.com
records:
  - type: A
    name: "@"
    content: 192.0.2.1
    ttl: 300
    proxied: true
  - type: MX
    name: "@"
    content: mail.example.com
    priority: 10
    ttl: 3600
`
	zf, err := Parse([]byte(input))
	require.NoError(t, err)
	assert.Equal(t, "example.com", zf.Zone)
	assert.Len(t, zf.Records, 2)
	assert.Equal(t, "A", zf.Records[0].Type)
	assert.Equal(t, "@", zf.Records[0].Name)
	assert.Equal(t, "192.0.2.1", zf.Records[0].Content)
	assert.Equal(t, 300, zf.Records[0].TTL)
	assert.Equal(t, true, *zf.Records[0].Proxied)
	assert.Equal(t, "MX", zf.Records[1].Type)
	assert.Equal(t, 10, *zf.Records[1].Priority)
}

func TestMarshalZoneFile(t *testing.T) {
	proxied := true
	priority := 10
	zf := &ZoneFile{
		Zone: "example.com",
		Records: []Record{
			{Type: "MX", Name: "@", Content: "mail.example.com", TTL: 3600, Priority: &priority},
			{Type: "A", Name: "@", Content: "192.0.2.1", TTL: 300, Proxied: &proxied},
		},
	}
	data, err := Marshal(zf)
	require.NoError(t, err)

	// Should be sorted: A before MX (by type, then name)
	parsed, err := Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "A", parsed.Records[0].Type)
	assert.Equal(t, "MX", parsed.Records[1].Type)
}

func TestRecordKey(t *testing.T) {
	r := Record{Type: "A", Name: "www", Content: "192.0.2.1"}
	assert.Equal(t, "a]www]192.0.2.1", r.Key())
}

func TestSortRecords(t *testing.T) {
	records := []Record{
		{Type: "TXT", Name: "@", Content: "v=spf1"},
		{Type: "A", Name: "www", Content: "192.0.2.1"},
		{Type: "A", Name: "@", Content: "192.0.2.1"},
		{Type: "CNAME", Name: "blog", Content: "example.netlify.app"},
	}
	sorted := SortRecords(records)
	assert.Equal(t, "A", sorted[0].Type)
	assert.Equal(t, "@", sorted[0].Name)
	assert.Equal(t, "A", sorted[1].Type)
	assert.Equal(t, "www", sorted[1].Name)
	assert.Equal(t, "CNAME", sorted[2].Type)
	assert.Equal(t, "TXT", sorted[3].Type)
}
