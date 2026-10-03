// Command server runs the Alhovuori community prototype: a private NIP-29
// relay (relay29 on khatru), the member API, and the web app, on one port.
package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fiatjaf/eventstore/badger"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	addr := env("LISTEN", ":8080")
	publicURL := strings.TrimSuffix(env("PUBLIC_URL", "http://localhost:5173"), "/")
	dataDir := env("DATA_DIR", "data")
	staticDir := env("STATIC_DIR", "../web/dist")

	u, err := url.Parse(publicURL)
	if err != nil || u.Host == "" {
		log.Fatalf("PUBLIC_URL %q is not a URL", publicURL)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		log.Fatal(err)
	}

	secretKey := relaySecretKey(dataDir)
	admins := parseAdmins(os.Getenv("ADMIN_NPUBS"))

	db := &badger.BadgerBackend{Path: filepath.Join(dataDir, "events")}
	if err := db.Init(); err != nil {
		log.Fatalf("open event store: %v", err)
	}
	defer db.Close()

	members, err := OpenMemberStore(filepath.Join(dataDir, "members.db"))
	if err != nil {
		log.Fatalf("open member store: %v", err)
	}
	defer members.Close()

	relay := NewRelay(u.Host, secretKey, db)
	// Clients connect to /relay (any path upgrades), and NIP-42 auth events
	// must name exactly this URL.
	relay.ServiceURL = strings.Replace(publicURL, "http", "ws", 1) + "/relay"
	if err := relay.Bootstrap(context.Background(), admins); err != nil {
		log.Fatalf("bootstrap groups: %v", err)
	}

	api := &API{Members: members, Relay: relay, Mailer: mailerFromEnv(), PublicURL: publicURL, Admins: admins}
	mux := relay.Router()
	api.Register(mux)
	mux.Handle("/", spaHandler(staticDir))

	log.Printf("relay pubkey %s, %d admin(s), listening on %s, public URL %s", relay.Pubkey, len(admins), addr, publicURL)
	log.Fatal(http.ListenAndServe(addr, relay))
}

// relaySecretKey reads RELAY_SECRET_KEY, or a key persisted in the data
// directory, generating one on first start.
func relaySecretKey(dataDir string) string {
	if sk := os.Getenv("RELAY_SECRET_KEY"); sk != "" {
		return sk
	}
	path := filepath.Join(dataDir, "relay.key")
	if b, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(b))
	}
	sk := nostr.GeneratePrivateKey()
	if err := os.WriteFile(path, []byte(sk), 0o600); err != nil {
		log.Fatalf("save relay key: %v", err)
	}
	return sk
}

// parseAdmins accepts a comma-separated list of npubs or hex public keys.
func parseAdmins(list string) []string {
	var out []string
	for _, s := range strings.Split(list, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "npub1") {
			_, v, err := nip19.Decode(s)
			if err != nil {
				log.Fatalf("ADMIN_NPUBS: %q: %v", s, err)
			}
			s = v.(string)
		}
		if !nostr.IsValid32ByteHex(s) {
			log.Fatalf("ADMIN_NPUBS: %q is not a public key", s)
		}
		out = append(out, s)
	}
	return out
}

func mailerFromEnv() Mailer {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		log.Print("SMTP_HOST not set: confirmation mail will be written to the log")
		return LogMailer{}
	}
	port, err := strconv.Atoi(env("SMTP_PORT", "587"))
	if err != nil {
		log.Fatalf("SMTP_PORT: %v", err)
	}
	from := os.Getenv("SMTP_FROM")
	if from == "" {
		log.Fatal("SMTP_FROM must be set when SMTP_HOST is")
	}
	return &SMTPMailer{
		Host: host, Port: port, From: from,
		Username: os.Getenv("SMTP_USERNAME"), Password: os.Getenv("SMTP_PASSWORD"),
	}
}

// spaHandler serves the built web app, answering with index.html for any
// path that is not a file.
func spaHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Directories get the app too, so the file server never lists them.
		if info, err := os.Stat(filepath.Join(dir, filepath.Clean("/"+r.URL.Path))); err != nil || info.IsDir() {
			index := filepath.Join(dir, "index.html")
			if _, err := os.Stat(index); err != nil {
				http.Error(w, "web app not built: run `npm run build` in proto/web", http.StatusNotFound)
				return
			}
			http.ServeFile(w, r, index)
			return
		}
		files.ServeHTTP(w, r)
	})
}
