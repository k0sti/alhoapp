# Alhovuori community system: prototype

This is the first stage of the Alhovuori community system (Linear CRO-24). It covers registration, login, the member home view and the community message board. The design is in the spec doc "Alhovuori Community System — Prototype Design" and in the Miro board "Alhovuori — Login & Registration Flow".

```
proto/
  server/   Go: private NIP-29 relay (relay29 on khatru), member API, serves the web app
  web/      Svelte 5 + TypeScript SPA (Applesauce, nostr-tools, Bits UI)
```

## Running locally

You need Go 1.24+ and Node 20+.

```bash
cd proto/web && npm install
```

In one terminal, start the server. `ADMIN_NPUBS` takes the npub(s) that may post announcements and open the member list:

```bash
cd proto/server && ADMIN_NPUBS=npub1... go run .
```

In another terminal, start the web app:

```bash
cd proto/web && npm run dev
```

Then open http://localhost:5173. Vite proxies `/api` and the relay websocket (`/relay`) to the server on :8080, so the app sees a single origin.

For a production-style run, build the app (`npm run build` in `proto/web`) and run the server alone. It serves `../web/dist` (`STATIC_DIR`) on `LISTEN`, so set `PUBLIC_URL` to the address people use.

### Server configuration (environment)

| Variable | Default | Purpose |
| --- | --- | --- |
| `LISTEN` | `:8080` | Listen address |
| `PUBLIC_URL` | `http://localhost:5173` | Public origin. Used for confirmation links, the NIP-42 relay URL (`<PUBLIC_URL>/relay`, ws/wss) and NIP-98 checks |
| `DATA_DIR` | `data` | Event store (Badger), member database (SQLite), relay key |
| `STATIC_DIR` | `../web/dist` | Built web app |
| `ADMIN_NPUBS` | | Comma-separated npubs or hex keys of administrators |
| `RELAY_SECRET_KEY` | generated | Relay key, hex. Stored in `DATA_DIR/relay.key` if not given |
| `SMTP_HOST`, `SMTP_PORT` (587), `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM` | | Confirmation mail. Port 465 uses implicit TLS; other ports use STARTTLS. Without `SMTP_HOST`, mail is written to the server log |

### Tests

```bash
cd proto/server && go test ./...
cd proto/web && npm test && npm run check
```

The Go tests run the whole server in-process. They cover registration, duplicate checks, email login, NIP-98, email confirmation, the relay's access rules and member retirement. The web tests cover the message board's collapse rules (`src/lib/board.ts`).

## How it fits together

**Identity.** Every member is a Nostr keypair. The server never sees a usable secret key.

- *Existing account*: the member proves they hold the key, and the server stores only the public key.
- *New account, server-stored*: the browser generates the key and encrypts it with the password using NIP-49 (`ncryptsec`, scrypt `LOG_N = 16`, see `SCRYPT_LOG_N`). The server stores the opaque string. Email login fetches it and decrypts it in the browser.
- *New account, self-hosted*: the browser generates the key, shows the nsec once, and stores only the public key.

**Proving key ownership.** Every authenticated API call carries a NIP-98 event signed by the member's key. The server checks the kind, signature, time window, URL, method and body hash, and rejects replays. Registration uses the same mechanism, so nobody can register a key they don't hold.

**Duplicate keys.** The `members` table has the public key as primary key. Registration also checks the relay's group membership, which covers administrators who have no member record. Login with a key that has no record returns `not_member`, and the app offers registration with that key.

**Relay.** relay29 hosts two closed, private NIP-29 groups. `alhovuori` is the message board; in `announcements` only administrators may post. Membership comes only from registration: the server adds the key with a relay-signed `put-user`. Join requests and group creation are refused. Reads need NIP-42 auth as a member. Members may also publish and read kind-0 profiles, which have no group tag. See `server/relay.go`.

**Email.** Email is mandatory and unique (case-insensitive). A confirmation link sets `verified`, which administrators see in the member list. Confirming does not gate access.

**Identity migration.** There is no key recovery. Administrators can *retire* a membership from the member list. This deletes the record and removes the key from both groups, so the person can register a new identity. Old messages stay. The full migration flow is still to be specified.

**Message board** (`web/src/lib/board.ts`, `web/src/pages/Board.svelte`):

- Messages stay in chronological order.
- Each person's latest message shows in full; their earlier ones collapse to one line.
- Clicking a message raises it one rung (face → line → full), and the "–" control lowers it one rung.
- Rungs set by hand last for the browser session (`sessionStorage`), so an expanded message never re-collapses.
- Filtered people (kept in `localStorage`) drop to faces, and consecutive faces merge into one row.
- The view follows new messages only when the reader is already at the bottom.

## Decisions and deviations

- **Login sessions.** Extension logins are restored after a reload. Password and nsec logins keep the key in memory only, so a reload means logging in again.
- **Registration form.** On the new-account route, "who keeps the key" and the password sit on one step. The diagram has "set password → generate key → self-host?", but asking for a password that self-hosting never uses would be confusing.
- **Avatars.** Members without a picture get a DiceBear *Lorelei* face seeded from their public key. Lorelei is CC0.
- **Announcements on home.** The newest is shown prominently with up to two earlier ones (`EARLIER_SHOWN`).
- **Nostr login.** Supports a NIP-07 extension or pasting an nsec. NIP-46 bunkers are not wired up yet; Applesauce's `NostrConnectSigner` would fit.

## Open questions and known limitations

- **Offline guessing.** Anyone who knows a member's email can fetch their `ncryptsec`, and the endpoint also reveals whether an email is registered. Per-IP rate limiting is in place, but scrypt's work factor is the real protection. This ties into the spec's open question on protecting the stored key further.
- **Scrypt blocks the page.** Encrypting and decrypting runs on the main thread, taking about 0.5–1 s on a laptop and more on weak phones. A Web Worker would keep the UI responsive.
- **Bundle size.** The JS bundle is about 230 kB gzipped, mostly Applesauce, nostr-tools and RxJS. It is not yet code-split.
- **Retired members.** Retiring a member does not delete their messages or profile from the relay.
