import { ExtensionSigner, PasswordSigner, PrivateKeySigner, type ISigner } from "applesauce-signers";
import { encrypt as nip49Encrypt } from "nostr-tools/nip49";
import { ApiError, api, type Member } from "./api";
import { authenticateWith, ensureProfile } from "./nostr";

// NIP-49 scrypt work factor for keys stored on the server. 2^16 iterations
// take well under a second on a laptop and a few seconds on a weak phone. The
// ncryptsec is handed to anyone who knows the member's email, so this is the
// only thing slowing down offline password guessing.
export const SCRYPT_LOG_N = 16;

export type LoginMethod = "extension" | "nsec" | "password";

const METHOD_KEY = "alho:login-method";

class Session {
  signer = $state<ISigner | null>(null);
  member = $state<Member | null>(null);
  method = $state<LoginMethod | null>(null);
  /** A one-off message for the next page, e.g. after registering. */
  flash = $state("");
  /** Set while restoring a previous session on page load. */
  restoring = $state(true);

  get pubkey() {
    return this.member?.pubkey ?? null;
  }
}

export const session = new Session();

/** Raised when a key signs in that has no member record. The signer is kept so
 *  that registration can carry on without asking for the key again. */
export class NotMemberError extends Error {
  constructor(
    readonly signer: ISigner,
    readonly method: LoginMethod,
  ) {
    super("This key is not registered yet.");
  }
}

/** A key that was offered at login but is not registered; registration with
 *  an existing account picks it up. */
export let pendingKey: { signer: ISigner; method: LoginMethod } | null = null;

export function takePendingKey() {
  const key = pendingKey;
  pendingKey = null;
  return key;
}

export async function extensionSigner(): Promise<ISigner> {
  if (!window.nostr) throw new Error("No Nostr browser extension was found. Install one (e.g. Alby or nos2x) or paste your nsec.");
  const signer = new ExtensionSigner();
  await signer.getPublicKey();
  return signer;
}

export function nsecSigner(nsec: string): ISigner {
  try {
    return PrivateKeySigner.fromKey(nsec.trim());
  } catch {
    throw new Error("That is not a valid nsec.");
  }
}

/** Starts a session for a signer whose key belongs to a member. */
export async function begin(signer: ISigner, method: LoginMethod) {
  let member: Member;
  try {
    member = await api.me(signer);
  } catch (err) {
    if (err instanceof ApiError && err.code === "not_member") {
      pendingKey = { signer, method };
      throw new NotMemberError(signer, method);
    }
    throw err;
  }
  startSession(signer, method, member);
}

export function startSession(signer: ISigner, method: LoginMethod, member: Member) {
  session.signer = signer;
  session.member = member;
  session.method = method;
  authenticateWith(signer);
  ensureProfile(signer, member.pubkey, member.name).catch((err) => console.warn("could not publish profile", err));
  // Only an extension can be restored after a reload: password and nsec
  // sessions keep the key in memory and never write it anywhere.
  try {
    if (method === "extension") sessionStorage.setItem(METHOD_KEY, method);
  } catch {}
}

export class EmailLoginError extends Error {
  constructor(
    readonly code: "not_member" | "self_hosted" | "wrong_password",
    message: string,
  ) {
    super(message);
  }
}

/** Fetches the encrypted key for an email address and decrypts it here, in
 *  the browser; the password never leaves this device. */
export async function loginWithEmail(email: string, password: string) {
  let stored: { pubkey: string; ncryptsec: string };
  try {
    stored = await api.loginEmail(email);
  } catch (err) {
    if (err instanceof ApiError && (err.code === "not_member" || err.code === "self_hosted")) {
      throw new EmailLoginError(err.code, err.message);
    }
    throw err;
  }
  // Let the "decrypting" state paint before scrypt blocks the main thread.
  await new Promise((r) => setTimeout(r, 30));
  let signer: PasswordSigner;
  try {
    signer = await PasswordSigner.fromNcryptsec(stored.ncryptsec, password);
  } catch {
    throw new EmailLoginError("wrong_password", "The password does not decrypt your key.");
  }
  if ((await signer.getPublicKey()) !== stored.pubkey) {
    throw new EmailLoginError("wrong_password", "The stored key does not match this account.");
  }
  await begin(signer, "password");
}

/** Wraps a freshly generated secret key in a NIP-49 password signer. */
export async function passwordSignerFor(secretKey: Uint8Array, password: string) {
  await new Promise((r) => setTimeout(r, 30));
  const signer = new PasswordSigner();
  signer.key = secretKey;
  signer.ncryptsec = nip49Encrypt(secretKey, password, SCRYPT_LOG_N);
  return signer;
}

export async function restoreSession() {
  try {
    if (sessionStorage.getItem(METHOD_KEY) === "extension") {
      // Extensions inject window.nostr shortly after load.
      for (let i = 0; i < 10 && !window.nostr; i++) await new Promise((r) => setTimeout(r, 100));
      if (window.nostr) await begin(await extensionSigner(), "extension");
    }
  } catch (err) {
    console.warn("could not restore session", err);
  } finally {
    session.restoring = false;
  }
}

export function logout() {
  try {
    sessionStorage.removeItem(METHOD_KEY);
  } catch {}
  // A reload is the simplest way to drop the relay's authentication and
  // every key held in memory.
  location.hash = "#/";
  location.reload();
}

export async function refreshMember() {
  if (session.signer) session.member = await api.me(session.signer);
}
