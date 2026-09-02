package scrape

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDiscoverAgents_SkipsNonRunningAndPortlessPods(t *testing.T) {
	running := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-1", Namespace: "zonetax", Labels: map[string]string{"app.kubernetes.io/component": "agent"}},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{Name: "agent", Ports: []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9090}}},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.1.5"},
	}
	pending := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-2", Namespace: "zonetax", Labels: map[string]string{"app.kubernetes.io/component": "agent"}},
		Spec: corev1.PodSpec{
			NodeName: "node-2",
			Containers: []corev1.Container{
				{Name: "agent", Ports: []corev1.ContainerPort{{Name: "metrics", ContainerPort: 9090}}},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	noPort := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-3", Namespace: "zonetax", Labels: map[string]string{"app.kubernetes.io/component": "agent"}},
		Spec: corev1.PodSpec{
			NodeName:   "node-3",
			Containers: []corev1.Container{{Name: "agent"}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.1.7"},
	}

	clientset := fake.NewSimpleClientset(running, pending, noPort)
	targets, err := DiscoverAgents(context.Background(), clientset, "zonetax", "app.kubernetes.io/component=agent")
	if err != nil {
		t.Fatalf("DiscoverAgents() error = %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1 (only the running pod with a metrics port)", len(targets))
	}
	if targets[0].PodName != "agent-1" || targets[0].Port != 9090 {
		t.Errorf("target = %+v, want agent-1 on port 9090", targets[0])
	}
}

func TestTarget_URL(t *testing.T) {
	tgt := Target{IP: "10.0.1.5", Port: 9090}
	want := "http://10.0.1.5:9090/metrics"
	if got := tgt.URL(); got != want {
		t.Errorf("URL() = %q, want %q", got, want)
	}
}

// TestScrapeAll_ParsesRealPrometheusTextFormat is a regression test for a real bug found
// deploying to solrn-dev: prometheus/common v0.71+ requires model.NameValidationScheme to be
// set globally before expfmt.TextParser can be used, or TextToMetricFamilies panics. Unit tests
// that only construct dto.MetricFamily structs by hand (as costengine's tests do) never
// exercise that code path — this test hits a real HTTP server serving real Prometheus text
// format, the same way the collector scrapes real agents, so it would have caught this.
func TestScrapeAll_ParsesRealPrometheusTextFormat(t *testing.T) {
	body := `# HELP zonetax_agent_cross_az_bytes_total test
# TYPE zonetax_agent_cross_az_bytes_total counter
zonetax_agent_cross_az_bytes_total{dst_zone="us-east-1a",src_namespace="prod",src_workload="web",src_zone="us-east-1b"} 1234
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	host := srv.Listener.Addr().String() // e.g. "127.0.0.1:54321"
	target := Target{PodName: "agent-test", IP: hostOnly(host), Port: portOnly(host)}

	results := ScrapeAll(context.Background(), []Target{target}, 5*time.Second)
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("scrape error = %v, want nil", results[0].Err)
	}
	mf, ok := results[0].Metrics["zonetax_agent_cross_az_bytes_total"]
	if !ok || len(mf.Metric) != 1 {
		t.Fatalf("Metrics = %+v, want one zonetax_agent_cross_az_bytes_total sample", results[0].Metrics)
	}
	if got := mf.Metric[0].GetCounter().GetValue(); got != 1234 {
		t.Errorf("counter value = %v, want 1234", got)
	}
}

func hostOnly(hostport string) string {
	for i := len(hostport) - 1; i >= 0; i-- {
		if hostport[i] == ':' {
			return hostport[:i]
		}
	}
	return hostport
}

func portOnly(hostport string) int32 {
	for i := len(hostport) - 1; i >= 0; i-- {
		if hostport[i] == ':' {
			var port int32
			for _, c := range hostport[i+1:] {
				port = port*10 + int32(c-'0')
			}
			return port
		}
	}
	return 0
}
