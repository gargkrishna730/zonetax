// Package pricing_test validates the embedded pricing table loads and resolves correctly.
package pricing

import "testing"

func TestLoadDefault(t *testing.T) {
	table, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error = %v", err)
	}
	if table.Version != 1 {
		t.Errorf("Version = %d, want 1", table.Version)
	}
	if _, ok := table.Pricing["aws"]; !ok {
		t.Fatal("expected \"aws\" pricing entry")
	}
}

func TestCrossAZPerGB(t *testing.T) {
	table, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault() error = %v", err)
	}

	tests := []struct {
		name    string
		cloud   string
		region  string
		want    float64
		wantErr bool
	}{
		{"known region", "aws", "us-east-1", 0.01, false},
		{"unknown region falls back to default", "aws", "af-south-1", 0.01, false},
		{"unknown cloud errors", "gcp", "us-east-1", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := table.CrossAZPerGB(tt.cloud, tt.region)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CrossAZPerGB() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("CrossAZPerGB() = %v, want %v", got, tt.want)
			}
		})
	}
}
