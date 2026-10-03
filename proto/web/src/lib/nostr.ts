import { EventStore } from "applesauce-core";
import { ProfileFactory } from "applesauce-core/factories";
import { RelayPool } from "applesauce-relay";
import type { ISigner } from "applesauce-signers";
import { filter, lastValueFrom, toArray, type Subscription } from "rxjs";

// Group ids on the community relay; they match proto/server/relay.go.
export const COMMUNITY_GROUP = "alhovuori";
export const ANNOUNCEMENTS_GROUP = "announcements";

/** The relay is served by the same origin as the app (proxied by Vite in development). */
export const RELAY_URL = `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/relay`;

export const eventStore = new EventStore();
export const pool = new RelayPool();
export const relay = pool.relay(RELAY_URL);

let authSub: Subscription | undefined;

/** Answers the relay's NIP-42 challenges with the member's signer. The groups
 *  are private: the relay refuses reads with "auth-required" until this
 *  succeeds, and applesauce-relay then retries those subscriptions. */
export function authenticateWith(signer: ISigner) {
  authSub?.unsubscribe();
  authSub = relay.challenge$.pipe(filter((c): c is string => !!c)).subscribe(() => {
    relay.authenticate(signer).catch((err) => console.warn("relay auth failed", err));
  });
}

/** Keeps the store fed with every member profile on the relay. */
export function syncProfiles(): Subscription {
  return pool.subscription([RELAY_URL], { kinds: [0] }, { eventStore }).subscribe();
}

/** Keeps the store fed with chat messages of one group. */
export function syncGroup(group: string, limit = 500): Subscription {
  return pool.subscription([RELAY_URL], { kinds: [9], "#h": [group], limit }, { eventStore }).subscribe();
}

export async function publish(event: Parameters<typeof relay.publish>[0]) {
  const res = await relay.publish(event);
  if (!res.ok) throw new Error(res.message || "The relay refused the message.");
  eventStore.add(event);
  return event;
}

/** Makes sure the member has a kind-0 profile on the community relay, so that
 *  others see their name rather than a bare npub. Registration supplies the
 *  name; members who already have a profile are left alone. */
export async function ensureProfile(signer: ISigner, pubkey: string, name: string) {
  const existing = await lastValueFrom(pool.request([RELAY_URL], { kinds: [0], authors: [pubkey] }, { eventStore }).pipe(toArray()));
  if (existing.length > 0) return;
  await publish(await ProfileFactory.create().name(name).sign(signer));
}
