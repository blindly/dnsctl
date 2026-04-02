package cloudflare

import (
	"context"
	"fmt"
	"strings"

	cf "github.com/cloudflare/cloudflare-go"
	"github.com/wk/dnsctl/internal/zone"
)

// Client wraps the cloudflare-go API client.
type Client struct {
	api *cf.API
}

// ZoneInfo holds basic zone identification info.
type ZoneInfo struct {
	ID   string
	Name string
}

// NewClient creates a new Cloudflare API client using the given API token.
func NewClient(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("CLOUDFLARE_API_TOKEN not set. Get a token at https://dash.cloudflare.com/profile/api-tokens")
	}
	api, err := cf.NewWithAPIToken(token)
	if err != nil {
		return nil, fmt.Errorf("creating cloudflare client: %w", err)
	}
	return &Client{api: api}, nil
}

// VerifyToken checks that the API token is valid.
func (c *Client) VerifyToken(ctx context.Context) error {
	_, err := c.api.VerifyAPIToken(ctx)
	if err != nil {
		return fmt.Errorf("invalid API token: %w", err)
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
// zone.Record values. It also returns an idMap mapping each record's Key() to
// its Cloudflare record ID so callers can perform updates/deletes.
// zoneName is required to simplify fully-qualified names to relative labels.
func (c *Client) ListRecords(ctx context.Context, zoneID, zoneName string) ([]zone.Record, map[string]string, error) {
	rc := cf.ZoneIdentifier(zoneID)
	records, _, err := c.api.ListDNSRecords(ctx, rc, cf.ListDNSRecordsParams{})
	if err != nil {
		return nil, nil, fmt.Errorf("listing records: %w", err)
	}

	var zoneRecords []zone.Record
	idMap := make(map[string]string)

	for _, r := range records {
		rec := zone.Record{
			Type:    r.Type,
			Name:    simplifyName(r.Name, zoneName),
			Content: r.Content,
			TTL:     r.TTL,
		}
		if r.Type == "A" || r.Type == "AAAA" || r.Type == "CNAME" {
			if r.Proxied != nil {
				proxied := *r.Proxied
				rec.Proxied = &proxied
			}
		}
		if r.Type == "MX" || r.Type == "SRV" {
			if r.Priority != nil {
				priority := int(*r.Priority)
				rec.Priority = &priority
			}
		}
		zoneRecords = append(zoneRecords, rec)
		idMap[rec.Key()] = r.ID
	}

	return zoneRecords, idMap, nil
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
