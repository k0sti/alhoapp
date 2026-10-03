package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fiatjaf/eventstore/slicestore"
	"github.com/nbd-wtf/go-nostr"
)

type testEnv struct {
	t      *testing.T
	srv    *httptest.Server
	wsURL  string
	admin  string // secret key
	mailer *recordingMailer
}

type recordingMailer struct {
	mu   sync.Mutex
	sent []string
}

func (m *recordingMailer) Send(to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, to+"\n"+body)
	return nil
}

func (m *recordingMailer) all() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.sent...)
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	db := &slicestore.SliceStore{}
	db.Init()
	members, err := OpenMemberStore(filepath.Join(t.TempDir(), "members.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { members.Close() })

	admin := nostr.GeneratePrivateKey()
	adminPub, _ := nostr.GetPublicKey(admin)

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")

	relay := NewRelay(host, nostr.GeneratePrivateKey(), db)
	relay.ServiceURL = "ws://" + host + "/relay"
	relay.SetRouter(mux)
	if err := relay.Bootstrap(context.Background(), []string{adminPub}); err != nil {
		t.Fatal(err)
	}
	mailer := &recordingMailer{}
	api := &API{Members: members, Relay: relay, Mailer: mailer, PublicURL: srv.URL, Admins: []string{adminPub}}
	api.Register(mux)
	srv.Config.Handler = relay

	return &testEnv{t: t, srv: srv, wsURL: relay.ServiceURL, admin: admin, mailer: mailer}
}

// call makes an API request, signing it with NIP-98 when sk is set.
func (e *testEnv) call(method, path, sk string, body any) (int, map[string]any) {
	e.t.Helper()
	url := e.srv.URL + path
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, url, bytes.NewReader(raw))
	if sk != "" {
		tags := nostr.Tags{{"u", url}, {"method", method}}
		if raw != nil {
			sum := sha256.Sum256(raw)
			tags = append(tags, nostr.Tag{"payload", hex.EncodeToString(sum[:])})
		}
		evt := nostr.Event{Kind: 27235, CreatedAt: nostr.Now(), Tags: tags}
		evt.Sign(sk)
		j, _ := json.Marshal(evt)
		req.Header.Set("Authorization", "Nostr "+base64.StdEncoding.EncodeToString(j))
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// connect opens a relay connection, authenticating with sk unless it is empty.
// Note: go-nostr's client stores the AUTH challenge without locking, so
// `go test -race` reports races inside go-nostr here; the server is not involved.
func (e *testEnv) connect(sk string) *nostr.Relay {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rl, err := nostr.RelayConnect(ctx, e.wsURL)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { rl.Close() })
	if sk != "" {
		// The challenge arrives right after connecting.
		var authErr error
		for i := 0; i < 20; i++ {
			if authErr = rl.Auth(ctx, func(evt *nostr.Event) error { return evt.Sign(sk) }); authErr == nil {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		if authErr != nil {
			e.t.Fatalf("auth: %v", authErr)
		}
	}
	return rl
}

func post(rl *nostr.Relay, sk string, kind int, content string, tags nostr.Tags) error {
	evt := nostr.Event{Kind: kind, CreatedAt: nostr.Now(), Tags: tags, Content: content}
	evt.Sign(sk)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return rl.Publish(ctx, evt)
}

// query returns the stored events matching f, or the relay's reason for refusing.
func query(t *testing.T, rl *nostr.Relay, f nostr.Filter) ([]*nostr.Event, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub, err := rl.Subscribe(ctx, nostr.Filters{f})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsub()
	var events []*nostr.Event
	for {
		select {
		case evt := <-sub.Events:
			events = append(events, evt)
		case <-sub.EndOfStoredEvents:
			return events, ""
		case reason := <-sub.ClosedReason:
			return nil, reason
		case <-ctx.Done():
			t.Fatal("query timed out")
		}
	}
}

func board() nostr.Tags         { return nostr.Tags{{"h", CommunityGroup}} }
func announcements() nostr.Tags { return nostr.Tags{{"h", AnnouncementsGroup}} }

func TestRegistrationAndLogin(t *testing.T) {
	e := newTestEnv(t)
	alice, bob := nostr.GeneratePrivateKey(), nostr.GeneratePrivateKey()

	status, body := e.call("POST", "/api/register", alice, map[string]string{"name": "Alice", "email": "alice@example.com", "ncryptsec": "ncryptsec1qexample"})
	if status != http.StatusCreated || body["keyStorage"] != "server" {
		t.Fatalf("register: %d %v", status, body)
	}
	status, body = e.call("POST", "/api/register", alice, map[string]string{"name": "Alice 2", "email": "alice2@example.com"})
	if status != http.StatusConflict || body["error"] != "duplicate_npub" {
		t.Errorf("duplicate npub: %d %v", status, body)
	}
	status, body = e.call("POST", "/api/register", bob, map[string]string{"name": "Bob", "email": "ALICE@example.com"})
	if status != http.StatusConflict || body["error"] != "duplicate_email" {
		t.Errorf("duplicate email: %d %v", status, body)
	}
	status, _ = e.call("POST", "/api/register", "", map[string]string{"name": "Eve", "email": "eve@example.com"})
	if status != http.StatusUnauthorized {
		t.Errorf("unsigned register: %d", status)
	}
	status, body = e.call("POST", "/api/register", e.admin, map[string]string{"name": "Admin", "email": "admin@example.com"})
	if status != http.StatusConflict || body["error"] != "duplicate_npub" {
		t.Errorf("admin key already on relay should count as registered: %d %v", status, body)
	}

	status, body = e.call("POST", "/api/register", bob, map[string]string{"name": "Bob", "email": "bob@example.com"})
	if status != http.StatusCreated || body["keyStorage"] != "self" {
		t.Fatalf("register self-hosted: %d %v", status, body)
	}

	status, body = e.call("POST", "/api/login/email", "", map[string]string{"email": "alice@example.com"})
	if status != http.StatusOK || body["ncryptsec"] != "ncryptsec1qexample" {
		t.Errorf("email login: %d %v", status, body)
	}
	status, body = e.call("POST", "/api/login/email", "", map[string]string{"email": "bob@example.com"})
	if status != http.StatusConflict || body["error"] != "self_hosted" {
		t.Errorf("email login for self-hosted key: %d %v", status, body)
	}

	status, body = e.call("GET", "/api/me", alice, nil)
	if status != http.StatusOK || body["verified"] != false {
		t.Errorf("me: %d %v", status, body)
	}
	status, body = e.call("GET", "/api/me", nostr.GeneratePrivateKey(), nil)
	if status != http.StatusNotFound || body["error"] != "not_member" {
		t.Errorf("me for stranger: %d %v", status, body)
	}
}

func TestEmailConfirmation(t *testing.T) {
	e := newTestEnv(t)
	alice := nostr.GeneratePrivateKey()
	e.call("POST", "/api/register", alice, map[string]string{"name": "Alice", "email": "alice@example.com"})

	var link string
	for i := 0; i < 50 && link == ""; i++ {
		for _, mail := range e.mailer.all() {
			if i := strings.Index(mail, e.srv.URL+"/api/verify?token="); i >= 0 {
				link = strings.Fields(mail[i:])[0]
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if link == "" {
		t.Fatal("no confirmation mail sent")
	}
	if status, _ := e.call("GET", strings.TrimPrefix(link, e.srv.URL), "", nil); status != http.StatusSeeOther {
		t.Fatalf("verify: %d", status)
	}
	if _, body := e.call("GET", "/api/me", alice, nil); body["verified"] != true {
		t.Errorf("member not verified after following link: %v", body)
	}

	// The flag is visible to administrators.
	status, _ := e.call("GET", "/api/admin/members", alice, nil)
	if status != http.StatusForbidden {
		t.Errorf("members list for non-admin: %d", status)
	}
}

func TestNIP98Rejections(t *testing.T) {
	e := newTestEnv(t)
	alice := nostr.GeneratePrivateKey()
	url := e.srv.URL + "/api/me"

	send := func(evt nostr.Event) int {
		evt.Sign(alice)
		j, _ := json.Marshal(evt)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Nostr "+base64.StdEncoding.EncodeToString(j))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	good := func() nostr.Event {
		return nostr.Event{Kind: 27235, CreatedAt: nostr.Now(), Tags: nostr.Tags{{"u", url}, {"method", "GET"}}}
	}

	old := good()
	old.CreatedAt -= 300
	wrongURL := good()
	wrongURL.Tags[0][1] = e.srv.URL + "/api/admin/members"
	wrongMethod := good()
	wrongMethod.Tags[1][1] = "POST"
	wrongKind := good()
	wrongKind.Kind = 1
	for name, evt := range map[string]nostr.Event{"expired": old, "other URL": wrongURL, "other method": wrongMethod, "wrong kind": wrongKind} {
		if status := send(evt); status != http.StatusUnauthorized {
			t.Errorf("%s: got %d, want 401", name, status)
		}
	}

	replayed := good()
	replayed.Sign(alice)
	j, _ := json.Marshal(replayed)
	for i, want := range []int{http.StatusNotFound, http.StatusUnauthorized} {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Nostr "+base64.StdEncoding.EncodeToString(j))
		res, _ := http.DefaultClient.Do(req)
		res.Body.Close()
		if res.StatusCode != want {
			t.Errorf("replay attempt %d: got %d, want %d", i, res.StatusCode, want)
		}
	}
}

func TestRelayPolicies(t *testing.T) {
	e := newTestEnv(t)
	alice, stranger := nostr.GeneratePrivateKey(), nostr.GeneratePrivateKey()
	alicePub, _ := nostr.GetPublicKey(alice)
	e.call("POST", "/api/register", alice, map[string]string{"name": "Alice", "email": "alice@example.com"})

	ca := e.connect(alice)
	if err := post(ca, alice, 9, "hello", board()); err != nil {
		t.Errorf("member post: %v", err)
	}
	if err := post(ca, alice, 0, `{"name":"Alice"}`, nil); err != nil {
		t.Errorf("member profile: %v", err)
	}
	if err := post(ca, alice, 9, "fake", announcements()); err == nil {
		t.Error("member posted an announcement")
	}
	if err := post(ca, alice, 9007, "", nostr.Tags{{"h", "new"}}); err == nil {
		t.Error("member created a group")
	}
	if evts, reason := query(t, ca, nostr.Filter{Kinds: []int{9}, Tags: nostr.TagMap{"h": {CommunityGroup}}}); reason != "" || len(evts) != 1 {
		t.Errorf("member read board: %d events, %q", len(evts), reason)
	}
	if evts, reason := query(t, ca, nostr.Filter{Kinds: []int{0}}); reason != "" || len(evts) != 1 || evts[0].PubKey != alicePub {
		t.Errorf("member read profiles: %d events, %q", len(evts), reason)
	}

	cadmin := e.connect(e.admin)
	if err := post(cadmin, e.admin, 9, "Welcome!", announcements()); err != nil {
		t.Errorf("admin announcement: %v", err)
	}

	cs := e.connect(stranger)
	if err := post(cs, stranger, 9, "spam", board()); err == nil {
		t.Error("non-member posted")
	}
	if err := post(cs, stranger, 0, `{"name":"Spam"}`, nil); err == nil {
		t.Error("non-member published a profile")
	}
	if _, reason := query(t, cs, nostr.Filter{Kinds: []int{9}, Tags: nostr.TagMap{"h": {CommunityGroup}}}); reason == "" {
		t.Error("non-member read the board")
	}
	if _, reason := query(t, cs, nostr.Filter{Kinds: []int{0}}); reason == "" {
		t.Error("non-member read profiles")
	}
	anon := e.connect("")
	if _, reason := query(t, anon, nostr.Filter{Kinds: []int{9}, Tags: nostr.TagMap{"h": {CommunityGroup}}}); !strings.HasPrefix(reason, "auth-required") {
		t.Errorf("unauthenticated read: %q", reason)
	}

	// Retiring a member (identity migration) removes the key from the groups.
	if status, _ := e.call("DELETE", "/api/admin/members/"+alicePub, e.admin, nil); status != http.StatusNoContent {
		t.Fatalf("retire: %d", status)
	}
	if err := post(e.connect(alice), alice, 9, "still here?", board()); err == nil {
		t.Error("retired member could still post")
	}
}
