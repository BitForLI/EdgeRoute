package quality

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type PrometheusProvider struct {
	baseURL string
	client  *http.Client
}

func NewPrometheusProvider(baseURL string, timeout time.Duration) *PrometheusProvider {
	return &PrometheusProvider{baseURL: baseURL, client: &http.Client{Timeout: timeout}}
}

func (p *PrometheusProvider) QueryNode(ctx context.Context, node string, at time.Time) (NodeSample, error) {
	active, err := p.query(ctx, fmt.Sprintf(`nginx_connections_active{node=%q}`, node), at)
	if err != nil {
		return NodeSample{}, fmt.Errorf("active requests: %w", err)
	}
	requests, err := p.query(ctx, fmt.Sprintf(`nginx_http_requests_total{node=%q}`, node), at)
	if err != nil {
		return NodeSample{}, fmt.Errorf("request count: %w", err)
	}
	return NodeSample{Timestamp: at, ActiveRequests: int(active), RequestCount: uint64(requests)}, nil
}

func (p *PrometheusProvider) query(ctx context.Context, expression string, at time.Time) (float64, error) {
	u, err := url.Parse(p.baseURL + "/api/v1/query")
	if err != nil {
		return 0, err
	}
	q := u.Query()
	q.Set("query", expression)
	q.Set("time", strconv.FormatInt(at.Unix(), 10))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus returned %s", resp.Status)
	}
	var body struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Value []any `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, err
	}
	if body.Status != "success" || len(body.Data.Result) != 1 || len(body.Data.Result[0].Value) != 2 {
		return 0, fmt.Errorf("expected one sample")
	}
	raw, ok := body.Data.Result[0].Value[1].(string)
	if !ok {
		return 0, fmt.Errorf("sample is not a string")
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0, fmt.Errorf("invalid sample %q", raw)
	}
	return value, nil
}
