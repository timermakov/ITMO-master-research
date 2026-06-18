package coord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/itmo-vkr/dwss/warmkit"
)

// MirrorConfig sets mirror routing via coordinator (sole ZK writer).
func MirrorConfig(coordURL string, cfg warmkit.MirrorConfig) error {
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, coordURL+"/v1/mirror/config", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("mirror config status %d", resp.StatusCode)
	}
	return nil
}

// BaselineMirror disables mirror and routes all traffic to active.
func BaselineMirror(coordURL, activeID string) error {
	return MirrorConfig(coordURL, warmkit.MirrorConfig{
		Enabled:          false,
		Ratio:            0,
		ActiveInstanceID: activeID,
	})
}

// StartSession creates a dynamic warmup session and returns session id.
func StartSession(coordURL, targetID, activeID string, readyAfter int) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"targetInstanceId": targetID,
		"activeInstanceId": activeID,
		"readyAfter":       readyAfter,
	})
	resp, err := http.Post(coordURL+"/v1/warmup/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("session status %d", resp.StatusCode)
	}
	var sess struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sess); err != nil {
		return "", err
	}
	return sess.ID, nil
}

// WaitSessionCompleted polls until session status is completed.
func WaitSessionCompleted(coordURL, sessionID string, timeoutSec int) error {
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(coordURL + "/v1/warmup/sessions/" + sessionID)
		if err == nil {
			var s struct {
				Status string `json:"status"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&s)
			_ = resp.Body.Close()
			if s.Status == "completed" {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("session %s not completed", sessionID)
}
