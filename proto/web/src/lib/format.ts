import { nip19 } from "nostr-tools";

export function npub(pubkey: string) {
  return nip19.npubEncode(pubkey);
}

export function shortNpub(pubkey: string) {
  const n = npub(pubkey);
  return `${n.slice(0, 10)}…${n.slice(-4)}`;
}

const timeFmt = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit" });
const dateFmt = new Intl.DateTimeFormat(undefined, { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });

export function when(unixSeconds: number) {
  const d = new Date(unixSeconds * 1000);
  return d.toDateString() === new Date().toDateString() ? timeFmt.format(d) : dateFmt.format(d);
}
