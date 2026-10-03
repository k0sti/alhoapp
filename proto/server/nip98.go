package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// NIP-98 HTTP auth: the client signs a kind-27235 event naming the URL, the
// method and (for requests with a body) the SHA-256 of the body. This is how a
// member proves they hold the key behind an npub, both at registration and on
// every authenticated API call.

const nip98Window = 60 * time.Second

type nip98Verifier struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newNIP98Verifier() *nip98Verifier {
	return &nip98Verifier{seen: make(map[string]time.Time)}
}

// Verify checks the Authorization header of r and returns the signer's pubkey.
// body is the raw request body, or nil when there is none.
func (v *nip98Verifier) Verify(r *http.Request, body []byte) (string, error) {
	header := r.Header.Get("Authorization")
	encoded, ok := strings.CutPrefix(header, "Nostr ")
	if !ok {
		return "", errors.New("missing Nostr authorization")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("authorization is not base64")
	}
	var evt nostr.Event
	if err := json.Unmarshal(raw, &evt); err != nil {
		return "", errors.New("authorization is not a nostr event")
	}
	if evt.Kind != 27235 {
		return "", errors.New("authorization event has the wrong kind")
	}
	if ok, _ := evt.CheckSignature(); !ok {
		return "", errors.New("authorization signature is invalid")
	}
	age := time.Since(evt.CreatedAt.Time())
	if age > nip98Window || age < -nip98Window {
		return "", errors.New("authorization has expired; check your device clock")
	}
	if evt.Tags.GetFirst([]string{"method", r.Method}) == nil {
		return "", errors.New("authorization was signed for another method")
	}
	u := evt.Tags.GetFirst([]string{"u", ""})
	if u == nil {
		return "", errors.New("authorization names no URL")
	}
	signed, err := url.Parse((*u)[1])
	if err != nil || signed.Host != r.Host || signed.RequestURI() != r.URL.RequestURI() {
		return "", errors.New("authorization was signed for another URL")
	}
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		if evt.Tags.GetFirst([]string{"payload", hex.EncodeToString(sum[:])}) == nil {
			return "", errors.New("authorization does not match the request body")
		}
	}
	if !v.markSeen(evt.ID) {
		return "", errors.New("authorization was already used")
	}
	return evt.PubKey, nil
}

func (v *nip98Verifier) markSeen(id string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := time.Now()
	for k, t := range v.seen {
		if now.Sub(t) > 2*nip98Window {
			delete(v.seen, k)
		}
	}
	if _, dup := v.seen[id]; dup {
		return false
	}
	v.seen[id] = now
	return true
}
