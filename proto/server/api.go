package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nbd-wtf/go-nostr"
	"golang.org/x/time/rate"
)

type API struct {
	Members   *MemberStore
	Relay     *Relay
	Mailer    Mailer
	PublicURL string
	Admins    []string

	auth    *nip98Verifier
	limitMu sync.Mutex
	limits  map[string]*rate.Limiter
}

func (a *API) Register(mux *http.ServeMux) {
	a.auth = newNIP98Verifier()
	a.limits = make(map[string]*rate.Limiter)

	mux.HandleFunc("GET /api/config", a.config)
	mux.HandleFunc("POST /api/register", a.limited(a.register))
	mux.HandleFunc("POST /api/login/email", a.limited(a.loginEmail))
	mux.HandleFunc("GET /api/me", a.me)
	mux.HandleFunc("POST /api/me/confirmation", a.resendConfirmation)
	mux.HandleFunc("GET /api/verify", a.verify)
	mux.HandleFunc("GET /api/admin/members", a.adminOnly(a.listMembers))
	mux.HandleFunc("DELETE /api/admin/members/{pubkey}", a.adminOnly(a.deleteMember))
}

type apiError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiError{Error: code, Message: message})
}

// readBody returns the raw body so that the NIP-98 payload hash can be checked
// against exactly the bytes that were signed.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil {
		fail(w, http.StatusRequestEntityTooLarge, "bad_request", "Request body is too large.")
		return nil, false
	}
	return body, true
}

func (a *API) authenticate(w http.ResponseWriter, r *http.Request, body []byte) (string, bool) {
	pubkey, err := a.auth.Verify(r, body)
	if err != nil {
		fail(w, http.StatusUnauthorized, "unauthorized", err.Error())
		return "", false
	}
	return pubkey, true
}

func (a *API) limited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		a.limitMu.Lock()
		l, ok := a.limits[ip]
		if !ok {
			l = rate.NewLimiter(rate.Every(6*time.Second), 10)
			a.limits[ip] = l
		}
		a.limitMu.Unlock()
		if !l.Allow() {
			fail(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Wait a minute and try again.")
			return
		}
		next(w, r)
	}
}

func (a *API) adminOnly(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pubkey, ok := a.authenticate(w, r, nil)
		if !ok {
			return
		}
		if !slices.Contains(a.Admins, pubkey) {
			fail(w, http.StatusForbidden, "forbidden", "Administrators only.")
			return
		}
		next(w, r, pubkey)
	}
}

func (a *API) config(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"relayPubkey":        a.Relay.Pubkey,
		"communityGroup":     CommunityGroup,
		"announcementsGroup": AnnouncementsGroup,
	})
}

type memberView struct {
	*Member
	KeyStorage string `json:"keyStorage"`
	IsAdmin    bool   `json:"isAdmin"`
}

func (a *API) view(m *Member) memberView {
	return memberView{Member: m, KeyStorage: m.KeyStorage(), IsAdmin: slices.Contains(a.Admins, m.Pubkey)}
}

type registerRequest struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Ncryptsec string `json:"ncryptsec"`
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	pubkey, ok := a.authenticate(w, r, body)
	if !ok {
		return
	}
	var req registerRequest
	if err := json.Unmarshal(body, &req); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "Malformed request.")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Email = strings.TrimSpace(req.Email)
	if n := utf8.RuneCountInString(req.Name); n == 0 || n > 64 {
		fail(w, http.StatusBadRequest, "invalid_name", "Give a name or handle of at most 64 characters.")
		return
	}
	if addr, err := mail.ParseAddress(req.Email); err != nil || addr.Address != req.Email {
		fail(w, http.StatusBadRequest, "invalid_email", "Give a valid email address.")
		return
	}
	if req.Ncryptsec != "" && (!strings.HasPrefix(req.Ncryptsec, "ncryptsec1") || len(req.Ncryptsec) > 512) {
		fail(w, http.StatusBadRequest, "invalid_key", "The encrypted key is not a valid ncryptsec.")
		return
	}
	// The relay knows administrators who never registered, so check it as
	// well as the member records: one key must never back two members.
	if a.Relay.IsMember(pubkey) || a.Members.Exists(r.Context(), pubkey) {
		fail(w, http.StatusConflict, "duplicate_npub", ErrDuplicatePubkey.Error())
		return
	}

	m := &Member{Pubkey: pubkey, Name: req.Name, Email: req.Email, Ncryptsec: req.Ncryptsec}
	token, err := a.Members.Create(r.Context(), m)
	switch {
	case errors.Is(err, ErrDuplicatePubkey):
		fail(w, http.StatusConflict, "duplicate_npub", err.Error())
		return
	case errors.Is(err, ErrDuplicateEmail):
		fail(w, http.StatusConflict, "duplicate_email", err.Error())
		return
	case err != nil:
		log.Printf("register %s: %v", pubkey, err)
		fail(w, http.StatusInternalServerError, "internal", "Registration failed.")
		return
	}
	if err := a.Relay.AddMember(r.Context(), pubkey); err != nil {
		log.Printf("add %s to groups: %v", pubkey, err)
		a.Members.Delete(context.Background(), pubkey)
		fail(w, http.StatusInternalServerError, "internal", "Registration failed.")
		return
	}
	a.sendConfirmation(m, token)
	writeJSON(w, http.StatusCreated, a.view(m))
}

func (a *API) sendConfirmation(m *Member, token string) {
	subject, body := confirmationMail(m.Name, a.PublicURL+"/api/verify?token="+token)
	go func() {
		if err := a.Mailer.Send(m.Email, subject, body); err != nil {
			log.Printf("confirmation mail to %s: %v", m.Email, err)
		}
	}()
}

// loginEmail hands out the encrypted key for an email address. Decryption
// happens in the browser, so the password never reaches the server. Anyone who
// knows a member's email can fetch the ncryptsec, which makes the scrypt work
// factor the protection against offline guessing.
func (a *API) loginEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "Malformed request.")
		return
	}
	m, err := a.Members.ByEmail(r.Context(), strings.TrimSpace(req.Email))
	if errors.Is(err, ErrNotFound) {
		fail(w, http.StatusNotFound, "not_member", "No member is registered with this email address.")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, "internal", "Login failed.")
		return
	}
	if m.Ncryptsec == "" {
		fail(w, http.StatusConflict, "self_hosted", "This member keeps their own key. Log in with Nostr instead.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"pubkey": m.Pubkey, "ncryptsec": m.Ncryptsec})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	pubkey, ok := a.authenticate(w, r, nil)
	if !ok {
		return
	}
	m, err := a.Members.ByPubkey(r.Context(), pubkey)
	if errors.Is(err, ErrNotFound) {
		if slices.Contains(a.Admins, pubkey) {
			// Configured administrators may have no member record.
			writeJSON(w, http.StatusOK, memberView{Member: &Member{Pubkey: pubkey, Name: "Administrator"}, KeyStorage: "self", IsAdmin: true})
			return
		}
		fail(w, http.StatusNotFound, "not_member", "This key is not registered. Register to join the community.")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, "internal", "Could not load member.")
		return
	}
	writeJSON(w, http.StatusOK, a.view(m))
}

func (a *API) resendConfirmation(w http.ResponseWriter, r *http.Request) {
	pubkey, ok := a.authenticate(w, r, nil)
	if !ok {
		return
	}
	m, err := a.Members.ByPubkey(r.Context(), pubkey)
	if err != nil {
		fail(w, http.StatusNotFound, "not_member", "This key is not registered.")
		return
	}
	if m.Verified {
		writeJSON(w, http.StatusOK, a.view(m))
		return
	}
	token, err := a.Members.VerifyToken(r.Context(), pubkey)
	if err != nil {
		fail(w, http.StatusInternalServerError, "internal", "Could not send confirmation.")
		return
	}
	a.sendConfirmation(m, token)
	writeJSON(w, http.StatusOK, a.view(m))
}

func (a *API) verify(w http.ResponseWriter, r *http.Request) {
	target := a.PublicURL + "/#/verified"
	if err := a.Members.Verify(r.Context(), r.URL.Query().Get("token")); err != nil {
		target = a.PublicURL + "/#/verify-failed"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (a *API) listMembers(w http.ResponseWriter, r *http.Request, _ string) {
	members, err := a.Members.List(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, "internal", "Could not list members.")
		return
	}
	views := make([]memberView, len(members))
	for i, m := range members {
		views[i] = a.view(m)
	}
	writeJSON(w, http.StatusOK, views)
}

// deleteMember is the first half of identity migration: the old record is
// removed and the key leaves the groups, so the person can register anew.
func (a *API) deleteMember(w http.ResponseWriter, r *http.Request, admin string) {
	pubkey := r.PathValue("pubkey")
	if !nostr.IsValid32ByteHex(pubkey) {
		fail(w, http.StatusBadRequest, "bad_request", "Not a public key.")
		return
	}
	if err := a.Members.Delete(r.Context(), pubkey); errors.Is(err, ErrNotFound) {
		fail(w, http.StatusNotFound, "not_member", "No such member.")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, "internal", "Could not delete member.")
		return
	}
	if err := a.Relay.RemoveMember(r.Context(), pubkey); err != nil {
		log.Printf("remove %s from groups: %v", pubkey, err)
	}
	log.Printf("admin %s deleted member %s", admin, pubkey)
	w.WriteHeader(http.StatusNoContent)
}
