// Package pricing loads cloud/region-specific cost tables for cross-AZ and cross-region
// network transfer, so the collector can turn raw byte counts into dollar figures.
package pricing

import (
	"embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed aws_pricing.yaml
var embeddedFS embed.FS

// Table is a versioned pricing table for one or more cloud providers.
type Table struct {
	Version int                     `yaml:"version"`
	Pricing map[string]CloudPricing `yaml:"pricing"`
}

// CloudPricing holds default and per-region overrides for a single cloud provider.
type CloudPricing struct {
	DefaultCrossAZPerGB     float64                   `yaml:"default_cross_az_per_gb"`
	DefaultCrossRegionPerGB float64                   `yaml:"default_cross_region_per_gb"`
	Regions                 map[string]RegionOverride `yaml:"regions"`
}

// RegionOverride overrides the cloud-level default for a specific region.
type RegionOverride struct {
	CrossAZPerGB     float64 `yaml:"cross_az_per_gb"`
	CrossRegionPerGB float64 `yaml:"cross_region_per_gb"`
}

// LoadDefault loads the pricing table embedded at build time (AWS only for now).
func LoadDefault() (*Table, error) {
	data, err := embeddedFS.ReadFile("aws_pricing.yaml")
	if err != nil {
		return nil, fmt.Errorf("read embedded pricing table: %w", err)
	}
	var t Table
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse pricing table: %w", err)
	}
	return &t, nil
}

// CrossAZPerGB returns the $/GB cross-AZ rate for a cloud+region, falling back to the cloud's
// default rate if no region-specific override exists.
func (t *Table) CrossAZPerGB(cloud, region string) (float64, error) {
	cp, ok := t.Pricing[cloud]
	if !ok {
		return 0, fmt.Errorf("no pricing data for cloud %q", cloud)
	}
	if r, ok := cp.Regions[region]; ok && r.CrossAZPerGB > 0 {
		return r.CrossAZPerGB, nil
	}
	return cp.DefaultCrossAZPerGB, nil
}
