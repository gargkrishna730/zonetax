// Package scrape discovers zonetax-agent DaemonSet pods via the Kubernetes API and scrapes
// their Prometheus /metrics endpoints, so the collector doesn't depend on a separate Prometheus
// server being present (though nothing stops one from also scraping the agents independently).
package scrape

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Target identifies one agent pod to scrape.
type Target struct {
	NodeName string
	PodName  string
	IP       string
	Port     int32
}

// URL returns the /metrics URL for this target.
func (t Target) URL() string {
	return fmt.Sprintf("http://%s:%d/metrics", t.IP, t.Port)
}

// DiscoverAgents lists Running pods matching labelSelector in namespace and returns one Target
// per pod that has an assigned IP and a container port named "metrics". Pods without an IP yet
// (e.g. still starting) are skipped rather than erroring, since the DaemonSet's pod set changes
// over time and a partial scrape is far more useful than failing the whole cycle.
func DiscoverAgents(ctx context.Context, clientset kubernetes.Interface, namespace, labelSelector string) ([]Target, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("scrape: list agent pods: %w", err)
	}

	var targets []Target
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Status.PodIP == "" {
			continue
		}
		port, ok := metricsPort(&pod)
		if !ok {
			continue
		}
		targets = append(targets, Target{
			NodeName: pod.Spec.NodeName,
			PodName:  pod.Name,
			IP:       pod.Status.PodIP,
			Port:     port,
		})
	}
	return targets, nil
}

func metricsPort(pod *corev1.Pod) (int32, bool) {
	for _, c := range pod.Spec.Containers {
		for _, p := range c.Ports {
			if p.Name == "metrics" {
				return p.ContainerPort, true
			}
		}
	}
	return 0, false
}

// Result bundles one agent's scrape outcome, so a single unreachable/misbehaving agent doesn't
// fail the whole collection cycle — callers can inspect Err per-target and proceed with the rest.
type Result struct {
	Target  Target
	Metrics map[string]*dto.MetricFamily
	Err     error
}

// ScrapeAll fetches and parses /metrics from every target concurrently, with a per-request
// timeout so one hung agent can't stall the whole cycle.
func ScrapeAll(ctx context.Context, targets []Target, timeout time.Duration) []Result {
	client := &http.Client{Timeout: timeout}
	results := make([]Result, len(targets))
	done := make(chan int, len(targets))

	for i, t := range targets {
		go func(i int, t Target) {
			mf, err := scrapeOne(ctx, client, t)
			results[i] = Result{Target: t, Metrics: mf, Err: err}
			done <- i
		}(i, t)
	}
	for range targets {
		<-done
	}
	return results
}

func scrapeOne(ctx context.Context, client *http.Client, t Target) (map[string]*dto.MetricFamily, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scrape %s: %w", t.PodName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("scrape %s: HTTP %d: %s", t.PodName, resp.StatusCode, body)
	}

	var parser expfmt.TextParser
	mf, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("scrape %s: parse metrics: %w", t.PodName, err)
	}
	return mf, nil
}
