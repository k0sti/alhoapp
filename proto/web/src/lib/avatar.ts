import { createAvatar } from "@dicebear/core";
import * as lorelei from "@dicebear/lorelei";

// Members without a profile picture get a generated face seeded from their
// public key, so it is stable and distinct per key. Lorelei (by Lisa
// Wischofsky) is released under CC0.
const cache = new Map<string, string>();

export function generatedAvatar(pubkey: string): string {
  let uri = cache.get(pubkey);
  if (!uri) {
    uri = createAvatar(lorelei, { seed: pubkey, backgroundColor: ["e8efe6", "efe8dc", "e3e8ef", "efe3e6"] }).toDataUri();
    cache.set(pubkey, uri);
  }
  return uri;
}
