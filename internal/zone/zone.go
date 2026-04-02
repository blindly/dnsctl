package zone

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Record struct {
	Type     string `yaml:"type"`
	Name     string `yaml:"name"`
	Content  string `yaml:"content"`
	TTL      int    `yaml:"ttl"`
	Proxied  *bool  `yaml:"proxied,omitempty"`
	Priority *int   `yaml:"priority,omitempty"`
}

func (r Record) Key() string {
	return fmt.Sprintf("%s]%s]%s", strings.ToLower(r.Type), strings.ToLower(r.Name), strings.ToLower(r.Content))
}

type ZoneFile struct {
	Zone    string   `yaml:"zone"`
	Records []Record `yaml:"records"`
}

func Parse(data []byte) (*ZoneFile, error) {
	var zf ZoneFile
	if err := yaml.Unmarshal(data, &zf); err != nil {
		return nil, fmt.Errorf("parsing zone file: %w", err)
	}
	return &zf, nil
}

func Marshal(zf *ZoneFile) ([]byte, error) {
	sorted := *zf
	sorted.Records = SortRecords(sorted.Records)
	data, err := yaml.Marshal(&sorted)
	if err != nil {
		return nil, fmt.Errorf("marshaling zone file: %w", err)
	}
	return data, nil
}

func SortRecords(records []Record) []Record {
	sorted := make([]Record, len(records))
	copy(sorted, records)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Type != sorted[j].Type {
			return sorted[i].Type < sorted[j].Type
		}
		if sorted[i].Name != sorted[j].Name {
			return sorted[i].Name < sorted[j].Name
		}
		return sorted[i].Content < sorted[j].Content
	})
	return sorted
}

func ReadFile(path string) (*ZoneFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading zone file %s: %w", path, err)
	}
	return Parse(data)
}

func WriteFile(path string, zf *ZoneFile) error {
	data, err := Marshal(zf)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
