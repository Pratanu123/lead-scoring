package opensearch

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	url        string
	username   string
	password   string
	httpClient *http.Client
}

func NewClient(url, username, password string) *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &Client{
		url:      url,
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout:   5 * time.Second,
			Transport: tr,
		},
	}
}

// IndexLog posts a single log document into an index named logs-YYYY.MM.DD
func (c *Client) IndexLog(ctx context.Context, doc map[string]any) error {
	index := fmt.Sprintf("logs-%s", time.Now().UTC().Format("2006.01.02"))
	url := fmt.Sprintf("%s/%s/_doc", c.url, index)

	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("opensearch index failed: status=%d", resp.StatusCode)
	}

	return nil
}
