package cloudflare

import (
	"context"
	"fmt"
	"os"
	"strings"

	cf "github.com/cloudflare/cloudflare-go"
	"github.com/wk/dnsctl/internal/state"
	"github.com/wk/dnsctl/internal/zone"
)

// Client wraps the cloudflare-go API client.
type Client struct {
	api      *cf.API
	useToken bool // true for API token, false for global API key
}

// ZoneInfo holds basic zone identification info.
type ZoneInfo struct {
	ID   string
	Name string
}

// NewClient creates a new Cloudflare API client.
// It checks CLOUDFLARE_API_TOKEN first (scoped API token),
// then falls back to CLOUDFLARE_API_KEY + CLOUDFLARE_API_EMAIL (global API key).
func NewClient(token string) (*Client, error) {
	// Try API token first
	if token != "" {
		api, err := cf.NewWithAPIToken(token)
		if err != nil {
			return nil, fmt.Errorf("creating cloudflare client: %w", err)
		}
		return &Client{api: api, useToken: true}, nil
	}

	// Fall back to global API key
	apiKey := os.Getenv("CLOUDFLARE_API_KEY")
	apiEmail := os.Getenv("CLOUDFLARE_API_EMAIL")
	if apiKey != "" && apiEmail != "" {
		api, err := cf.New(apiKey, apiEmail)
		if err != nil {
			return nil, fmt.Errorf("creating cloudflare client: %w", err)
		}
		return &Client{api: api, useToken: false}, nil
	}

	return nil, fmt.Errorf("no Cloudflare credentials found. Set either:\n  CLOUDFLARE_API_TOKEN (recommended)\n  CLOUDFLARE_API_KEY + CLOUDFLARE_API_EMAIL")
}

// VerifyToken checks that the credentials are valid.
func (c *Client) VerifyToken(ctx context.Context) error {
	if c.useToken {
		_, err := c.api.VerifyAPIToken(ctx)
		if err != nil {
			return fmt.Errorf("invalid API token: %w", err)
		}
		return nil
	}
	// For global API key, verify by listing zones (no dedicated verify endpoint)
	_, err := c.api.ListZones(ctx)
	if err != nil {
		return fmt.Errorf("invalid API key/email: %w", err)
	}
	return nil
}

// ListZones returns all zones accessible to the token.
func (c *Client) ListZones(ctx context.Context) ([]ZoneInfo, error) {
	zones, err := c.api.ListZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing zones: %w", err)
	}
	var result []ZoneInfo
	for _, z := range zones {
		result = append(result, ZoneInfo{ID: z.ID, Name: z.Name})
	}
	return result, nil
}

// ListRecords fetches all DNS records for the given zone and returns them as
// zone.Record values plus a map of record key -> RecordState for state tracking.
func (c *Client) ListRecords(ctx context.Context, zoneID, zoneName string) ([]zone.Record, map[string]*state.RecordState, error) {
	rc := cf.ZoneIdentifier(zoneID)
	records, _, err := c.api.ListDNSRecords(ctx, rc, cf.ListDNSRecordsParams{})
	if err != nil {
		return nil, nil, fmt.Errorf("listing records: %w", err)
	}

	var zoneRecords []zone.Record
	stateMap := make(map[string]*state.RecordState)

	for _, r := range records {
		rec := zone.Record{
			Type:    r.Type,
			Name:    simplifyName(r.Name, zoneName),
			Content: r.Content,
			TTL:     r.TTL,
		}
		rs := &state.RecordState{
			ID:  r.ID,
			TTL: r.TTL,
		}
		if r.Type == "A" || r.Type == "AAAA" || r.Type == "CNAME" {
			if r.Proxied != nil {
				proxied := *r.Proxied
				rec.Proxied = &proxied
				rs.Proxied = &proxied
			}
		}
		if r.Type == "MX" || r.Type == "SRV" {
			if r.Priority != nil {
				priority := int(*r.Priority)
				rec.Priority = &priority
				rs.Priority = &priority
			}
		}
		zoneRecords = append(zoneRecords, rec)
		stateMap[rec.Key()] = rs
	}

	return zoneRecords, stateMap, nil
}

// CreateRecord creates a new DNS record and returns its Cloudflare record ID.
func (c *Client) CreateRecord(ctx context.Context, zoneID string, rec zone.Record, zoneName string) (string, error) {
	rc := cf.ZoneIdentifier(zoneID)
	params := cf.CreateDNSRecordParams{
		Type:    rec.Type,
		Name:    expandName(rec.Name, zoneName),
		Content: rec.Content,
		TTL:     rec.TTL,
	}
	if rec.Proxied != nil {
		params.Proxied = rec.Proxied
	}
	if rec.Priority != nil {
		p := uint16(*rec.Priority)
		params.Priority = &p
	}

	result, err := c.api.CreateDNSRecord(ctx, rc, params)
	if err != nil {
		return "", fmt.Errorf("creating record %s %s: %w", rec.Type, rec.Name, err)
	}
	return result.ID, nil
}

// UpdateRecord updates an existing DNS record identified by recordID.
func (c *Client) UpdateRecord(ctx context.Context, zoneID, recordID string, rec zone.Record, zoneName string) error {
	rc := cf.ZoneIdentifier(zoneID)
	params := cf.UpdateDNSRecordParams{
		ID:      recordID,
		Type:    rec.Type,
		Name:    expandName(rec.Name, zoneName),
		Content: rec.Content,
		TTL:     rec.TTL,
	}
	if rec.Proxied != nil {
		params.Proxied = rec.Proxied
	}
	if rec.Priority != nil {
		p := uint16(*rec.Priority)
		params.Priority = &p
	}

	_, err := c.api.UpdateDNSRecord(ctx, rc, params)
	if err != nil {
		return fmt.Errorf("updating record %s %s: %w", rec.Type, rec.Name, err)
	}
	return nil
}

// DeleteRecord deletes a DNS record by its Cloudflare record ID.
func (c *Client) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	rc := cf.ZoneIdentifier(zoneID)
	err := c.api.DeleteDNSRecord(ctx, rc, recordID)
	if err != nil {
		return fmt.Errorf("deleting record %s: %w", recordID, err)
	}
	return nil
}

// simplifyName converts a fully-qualified DNS name to a relative label.
// "example.com" -> "@", "www.example.com" -> "www".
func simplifyName(fullName, zoneName string) string {
	if fullName == zoneName {
		return "@"
	}
	return strings.TrimSuffix(fullName, "."+zoneName)
}

// expandName converts a relative label back to a fully-qualified DNS name.
// "@" -> "example.com", "www" -> "www.example.com".
func expandName(name, zoneName string) string {
	if name == "@" {
		return zoneName
	}
	return name + "." + zoneName
}
