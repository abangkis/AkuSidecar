package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func screenshotMediaURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range []string{"pbs.twimg.com", "licdn.com", "fbcdn.net", "fbsbx.com", "cdninstagram.com"} {
		if host == allowed || (allowed != "pbs.twimg.com" && strings.HasSuffix(host, "."+allowed)) {
			return true
		}
	}
	return false
}

// No proxy, cookie jar, Authorization, Referer or user profile is involved.
// Pin every dial to a public address after resolving, including redirects.
func screenshotMediaClient() *http.Client {
	transport := &http.Transport{Proxy: nil, MaxIdleConns: 2, ResponseHeaderTimeout: 5 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !publicScreenshotIP(ip.IP) {
				return nil, errors.New("nonpublic media address")
			}
		}
		for _, ip := range ips {
			conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, errors.New("media connection unavailable")
	}
	return &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || !screenshotMediaURL(req.URL.String()) {
			return errors.New("media redirect rejected")
		}
		return nil
	}}
}
func publicScreenshotIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, cidr := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32", "64:ff9b::/96"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

func screenshotMedia(ctx context.Context, snapshot map[string]any) (map[string]string, []string) {
	media := map[string]string{}
	seen := map[string]bool{}
	var urls []string
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if key == "avatarUrl" || key == "socialContextAvatarUrl" || key == "posterUrl" || key == "url" || key == "imageUrl" {
					if raw, ok := child.(string); ok && screenshotMediaURL(raw) && !seen[raw] {
						seen[raw] = true
						urls = append(urls, raw)
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(snapshot)
	client := screenshotMediaClient()
	defer client.CloseIdleConnections()
	budget := 32 << 20
	warnings := []string{}
	// Bound the whole asset phase as well as individual downloads.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for i, raw := range urls {
		if i >= 64 || budget <= 0 || ctx.Err() != nil {
			warnings = append(warnings, "Some media could not be loaded within the export resource limit.")
			break
		}
		data, mime, err := downloadScreenshotMedia(ctx, client, raw, budget)
		if err != nil {
			warnings = append(warnings, "A captured media preview is unavailable; its position is marked in the screenshot.")
			continue
		}
		budget -= len(data)
		media[raw] = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	}
	return media, warnings
}

func downloadScreenshotMedia(ctx context.Context, client *http.Client, raw string, budget int) ([]byte, string, error) {
	if !screenshotMediaURL(raw) {
		return nil, "", errors.New("media URL rejected")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/webp,image/gif")
	response, err := client.Do(req)
	if err != nil {
		return nil, "", errors.New("media unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", errors.New("media unavailable")
	}
	limit := min(budget, 8<<20)
	data, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, "", errors.New("media size exceeded")
	}
	mime := http.DetectContentType(data)
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/gif" && mime != "image/webp" {
		return nil, "", errors.New("unsupported raster media")
	}
	if mime != "image/webp" {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 32_000_000 {
			return nil, "", errors.New("media pixel limit exceeded")
		}
	}
	return data, mime, nil
}
