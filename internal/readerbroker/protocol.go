// Package readerbroker is an explicit-click-only Windows native messaging
// contract. It has no source-cookie, collection, profile or runtime-control API.
package readerbroker

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"time"
)

const ExtensionID = "dlibmmlopdahibfniinemhnghlifiple"
const HostName = "com.akubrowser.reader_activation"
const PipeName = `\\.\pipe\AkuBrowser.reader-activation.11122`
const Lifetime = 5 * time.Second

type Request struct {
	RequestID string `json:"requestId"`
	Source    string `json:"source"`
	URL       string `json:"url"`
}
type Target struct {
	HWND     uint64    `json:"hwnd"`
	PID      uint32    `json:"pid"`
	Property string    `json:"property"`
	Value    uint64    `json:"value"`
	Expires  time.Time `json:"expires"`
	Action   string    `json:"action"`
}
type Reply struct {
	OK       bool    `json:"ok"`
	Message  string  `json:"message,omitempty"`
	Ticket   string  `json:"ticket,omitempty"`
	Target   *Target `json:"target,omitempty"`
	Applied  bool    `json:"applied,omitempty"`
	Readback bool    `json:"readback,omitempty"`
}

var requestPattern = regexp.MustCompile(`^broker_[a-f0-9]{32}$`)

func (r Request) Validate() error {
	if !requestPattern.MatchString(r.RequestID) || len(r.URL) > 4096 {
		return errors.New("invalid reader request")
	}
	u, err := url.Parse(r.URL)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return errors.New("invalid reader URL")
	}
	host := u.Hostname()
	valid := false
	switch r.Source {
	case "x":
		valid = host == "x.com" || host == "www.x.com"
	case "linkedin":
		valid = host == "www.linkedin.com" || host == "linkedin.com"
	case "facebook":
		valid = host == "www.facebook.com" || host == "facebook.com"
	case "instagram":
		valid = host == "www.instagram.com" || host == "instagram.com"
	}
	if !valid || u.Port() != "" {
		return errors.New("reader source mismatch")
	}
	return nil
}
func Secret() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func Read(r io.Reader, v any) error {
	var n uint32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return err
	}
	if n == 0 || n > 8192 {
		return errors.New("reader frame size rejected")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
func Write(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > 8192 {
		return errors.New("reader frame too large")
	}
	if err = binary.Write(w, binary.LittleEndian, uint32(len(b))); err != nil {
		return err
	}
	n, err := w.Write(b)
	if err == nil && n != len(b) {
		return io.ErrShortWrite
	}
	return err
}

// Exchange discloses no native target until the fresh opaque ticket has been
// consumed on the authenticated connection and the caller is revalidated.
func Exchange(ctx context.Context, rw io.ReadWriter, target Target, authorize func() error) (Reply, error) {
	now := time.Now()
	if target.HWND == 0 || target.PID == 0 || target.Value == 0 || target.Action == "" || !now.Before(target.Expires) || target.Expires.Sub(now) > Lifetime {
		return Reply{}, errors.New("reader binding expired or invalid")
	}
	ticket := Secret()
	if err := Write(rw, Reply{OK: true, Ticket: ticket}); err != nil {
		return Reply{}, err
	}
	var claim Reply
	if err := Read(rw, &claim); err != nil {
		return Reply{}, err
	}
	if subtle.ConstantTimeCompare([]byte(claim.Ticket), []byte(ticket)) != 1 || ctx.Err() != nil || !time.Now().Before(target.Expires) {
		return Reply{}, errors.New("reader ticket expired or invalid")
	}
	if err := authorize(); err != nil {
		return Reply{}, err
	}
	if err := Write(rw, Reply{OK: true, Target: &target}); err != nil {
		return Reply{}, err
	}
	var result Reply
	if err := Read(rw, &result); err != nil {
		return Reply{}, err
	}
	if ctx.Err() != nil || !time.Now().Before(target.Expires) {
		return Reply{}, errors.New("reader result expired")
	}
	return result, nil
}
