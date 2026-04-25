package notify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NtfyConfig is the per-backend slice of the notifications config.
// Constructed by the daemon (or test helpers) and handed to NewNtfy.
type NtfyConfig struct {
	URL   string
	Topic string
}

// Ntfy posts to a ntfy server. Stateless — all knobs come from cfg.
type Ntfy struct {
	cfg    NtfyConfig
	client *http.Client
}

// NewNtfy returns a configured backend. The backend is NOT registered
// automatically; the caller (typically internal/serve) decides which
// backends to register based on the resolved config.
func NewNtfy(cfg NtfyConfig) *Ntfy {
	return &Ntfy{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name implements Backend.
func (Ntfy) Name() string { return "ntfy" }

// Send POSTs n to the configured topic. Body is n.Body, headers carry
// title and tags.
func (n *Ntfy) Send(ctx context.Context, msg Notification) error {
	u := strings.TrimRight(n.cfg.URL, "/") + "/" + n.cfg.Topic
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(msg.Body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", msg.Title)
	if len(msg.Tags) > 0 {
		req.Header.Set("Tags", strings.Join(msg.Tags, ","))
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned %s", resp.Status)
	}
	return nil
}

// SendTest fires a single canned ntfy push. Used by the `calemdar notify
// test` cli to prove the URL/topic work before the daemon flips on.
func (n *Ntfy) SendTest(ctx context.Context) error {
	return n.Send(ctx, Notification{
		Title: "calemdar: test",
		Body:  "calemdar notify test — if you see this, the wiring is good.",
		Tags:  []string{"calendar", "test"},
	})
}

// RedactURL strips userinfo (basic-auth creds) from a URL for safe
// logging. Falls back to the raw string if parsing fails.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Redacted()
}
