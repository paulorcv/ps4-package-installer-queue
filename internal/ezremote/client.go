// Package ezremote talks to the web API of ezRemote Client running on a PS4.
package ezremote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Client sends install requests to ezRemote.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a client for http://host:port.
func New(host string, port int, c *http.Client) *Client {
	return &Client{baseURL: fmt.Sprintf("http://%s:%d", host, port), http: c}
}

type installRequest struct {
	Action        string `json:"action"`
	URL           string `json:"url"`
	UseAllDebrid  bool   `json:"use_alldebrid"`
	UseRealDebrid bool   `json:"use_realdebrid"`
	EnableRPI     bool   `json:"enable_rpi"`
	UseDiskCache  bool   `json:"use_disk_cache"`
}

type apiResponse struct {
	Result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	} `json:"result"`
}

// ErrRejected wraps errors reported by ezRemote itself.
var ErrRejected = errors.New("ezRemote recusou a instalação")

// InstallURL asks the PS4 to download and install the package at pkgURL
// through the Remote Package Installer (the console downloads it directly).
//
// ezRemote only queues the task; it does not report progress afterwards.
func (c *Client) InstallURL(ctx context.Context, pkgURL string) error {
	body, err := json.Marshal(installRequest{Action: "installUrl", URL: pkgURL, EnableRPI: true})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/__local__/install_url", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sem resposta do ezRemote em %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ezRemote respondeu HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(raw))
	}

	var r apiResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		// Older builds may answer with an empty body on success.
		if len(bytes.TrimSpace(raw)) == 0 {
			return nil
		}
		return fmt.Errorf("resposta inesperada do ezRemote: %s", bytes.TrimSpace(raw))
	}
	if !r.Result.Success {
		msg := r.Result.Error
		if msg == "" {
			msg = "sem detalhes"
		}
		return fmt.Errorf("%w: %s", ErrRejected, msg)
	}
	return nil
}
