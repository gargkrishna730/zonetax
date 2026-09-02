package azmap

import "testing"

func TestIsCrossAZ(t *testing.T) {
	tests := []struct {
		name string
		a, b NodeInfo
		want bool
	}{
		{"same zone", NodeInfo{Zone: "us-east-1a"}, NodeInfo{Zone: "us-east-1a"}, false},
		{"different zone", NodeInfo{Zone: "us-east-1a"}, NodeInfo{Zone: "us-east-1b"}, true},
		{"missing zone a", NodeInfo{Zone: ""}, NodeInfo{Zone: "us-east-1b"}, false},
		{"missing zone b", NodeInfo{Zone: "us-east-1a"}, NodeInfo{Zone: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCrossAZ(tt.a, tt.b); got != tt.want {
				t.Errorf("IsCrossAZ() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStaticResolver(t *testing.T) {
	r := NewStaticResolver([]NodeInfo{
		{Name: "node-1", Zone: "us-east-1a", Region: "us-east-1"},
	})
	got, ok := r.NodeInfo("node-1")
	if !ok || got.Zone != "us-east-1a" {
		t.Errorf("NodeInfo(node-1) = %+v, %v", got, ok)
	}
	if _, ok := r.NodeInfo("unknown"); ok {
		t.Error("NodeInfo(unknown) should return false")
	}
}
