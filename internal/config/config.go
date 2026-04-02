package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Provider string   `yaml:"provider"`
	Zones    []string `yaml:"zones"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func FindRoot(from string) (string, error) {
	dir := from
	for {
		if _, err := os.Stat(filepath.Join(dir, ".dnsctl")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not a dnsctl repository (no .dnsctl directory found)")
		}
		dir = parent
	}
}

func ConfigPath(root string) string {
	return filepath.Join(root, ".dnsctl", "config.yaml")
}

func StatePath(root string) string {
	return filepath.Join(root, ".dnsctl", "state.json")
}
