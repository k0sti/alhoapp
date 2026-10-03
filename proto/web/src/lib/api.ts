import type { ISigner } from "applesauce-signers";

export interface Member {
  pubkey: string;
  name: string;
  email: string;
  verified: boolean;
  createdAt: string;
  verifiedAt?: string;
  keyStorage: "server" | "self";
  isAdmin: boolean;
}

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
  }
}

async function sha256Hex(text: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

/** NIP-98 HTTP auth: proves to the server that we hold the key. */
async function authorization(signer: ISigner, url: string, method: string, body?: string) {
  const tags = [
    ["u", url],
    ["method", method],
  ];
  if (body) tags.push(["payload", await sha256Hex(body)]);
  const event = await signer.signEvent({ kind: 27235, created_at: Math.floor(Date.now() / 1000), tags, content: "" });
  return "Nostr " + btoa(JSON.stringify(event));
}

async function call<T>(method: string, path: string, opts: { signer?: ISigner; json?: unknown } = {}): Promise<T> {
  const url = new URL(path, location.origin).toString();
  const body = opts.json === undefined ? undefined : JSON.stringify(opts.json);
  const headers: Record<string, string> = {};
  if (body) headers["Content-Type"] = "application/json";
  if (opts.signer) headers.Authorization = await authorization(opts.signer, url, method, body);

  let res: Response;
  try {
    res = await fetch(url, { method, headers, body });
  } catch {
    throw new ApiError(0, "network", "Could not reach the community server.");
  }
  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, data.error ?? "unknown", data.message ?? `Request failed (${res.status}).`);
  return data as T;
}

export const api = {
  register: (signer: ISigner, details: { name: string; email: string; ncryptsec?: string }) =>
    call<Member>("POST", "/api/register", { signer, json: details }),
  me: (signer: ISigner) => call<Member>("GET", "/api/me", { signer }),
  loginEmail: (email: string) => call<{ pubkey: string; ncryptsec: string }>("POST", "/api/login/email", { json: { email } }),
  resendConfirmation: (signer: ISigner) => call<Member>("POST", "/api/me/confirmation", { signer }),
  members: (signer: ISigner) => call<Member[]>("GET", "/api/admin/members", { signer }),
  deleteMember: (signer: ISigner, pubkey: string) => call<void>("DELETE", `/api/admin/members/${pubkey}`, { signer }),
};
