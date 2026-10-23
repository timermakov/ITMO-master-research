package warmkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ofrepClient struct {
	baseURL string
	http    *http.Client
}

type ofrepEvalResponse struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	Reason    string          `json:"reason"`
	Variant   string          `json:"variant"`
	ErrorCode string          `json:"errorCode"`
	ErrorMsg  string          `json:"errorDetails"`
}

func newOFREPClient(baseURL string, timeout time.Duration) *ofrepClient {
	return &ofrepClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *ofrepClient) evaluate(ctx context.Context, flagKey string) (json.RawMessage, error) {
	u := c.baseURL + "/ofrep/v1/evaluate/flags/" + url.PathEscape(flagKey)
	body, err := json.Marshal(map[string]any{
		"context": map[string]any{
			"targetingKey": "dwss",
			"app":          "dwss",
		},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ofrep %s: status %d: %s", flagKey, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var ev ofrepEvalResponse
	if err := json.Unmarshal(raw, &ev); err != nil {
		return nil, err
	}
	if ev.ErrorCode != "" {
		return nil, fmt.Errorf("ofrep %s: %s %s", flagKey, ev.ErrorCode, ev.ErrorMsg)
	}
	return ev.Value, nil
}

func (c *ofrepClient) view(ctx context.Context, service string) (CoordinationView, error) {
	mirrorRaw, err := c.evaluate(ctx, MirrorFlagKey(service))
	if err != nil {
		return CoordinationView{}, err
	}
	instRaw, err := c.evaluate(ctx, InstancesFlagKey(service))
	if err != nil {
		return CoordinationView{}, err
	}
	mirror, err := decodeJSON[MirrorConfig](mirrorRaw)
	if err != nil {
		return CoordinationView{}, fmt.Errorf("decode mirror flag: %w", err)
	}
	instances, err := decodeJSON[map[string]InstanceRecord](instRaw)
	if err != nil {
		return CoordinationView{}, fmt.Errorf("decode instances flag: %w", err)
	}
	if instances == nil {
		instances = map[string]InstanceRecord{}
	}
	return CoordinationView{Mirror: mirror, Instances: instances}, nil
}
