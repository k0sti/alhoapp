<script lang="ts">
  import { Checkbox, RadioGroup } from "bits-ui";
  import { PrivateKeySigner, type ISigner } from "applesauce-signers";
  import { generateSecretKey, getPublicKey, nip19 } from "nostr-tools";
  import { ApiError, api } from "../lib/api";
  import { navigate } from "../lib/router.svelte";
  import { shortNpub } from "../lib/format";
  import {
    extensionSigner,
    nsecSigner,
    passwordSignerFor,
    session,
    startSession,
    takePendingKey,
    type LoginMethod,
  } from "../lib/session.svelte";

  let { route }: { route: "existing" | "new" } = $props();

  // Both routes collect the same details first; only key handling differs.
  let step = $state<"details" | "key">("details");
  let name = $state("");
  let email = $state("");
  let busy = $state(false);
  let error = $state<{ text: string; login?: boolean } | null>(null);

  // Existing account: the key the member brings.
  const pending = takePendingKey();
  let existing = $state<{ signer: ISigner; method: LoginMethod; pubkey: string } | null>(null);
  if (pending) pending.signer.getPublicKey().then((pubkey) => (existing = { ...pending, pubkey }));
  let nsec = $state("");

  // New account: a fresh key, kept by the member or stored encrypted.
  const secretKey = generateSecretKey();
  const newNsec = nip19.nsecEncode(secretKey);
  let storage = $state("server");
  let password = $state("");
  let password2 = $state("");
  let savedKey = $state(false);
  let copied = $state(false);

  function next(e: SubmitEvent) {
    e.preventDefault();
    error = null;
    step = "key";
  }

  async function chooseExtension() {
    error = null;
    try {
      const signer = await extensionSigner();
      existing = { signer, method: "extension", pubkey: await signer.getPublicKey() };
    } catch (err) {
      error = { text: err instanceof Error ? err.message : String(err) };
    }
  }

  async function chooseNsec(e: SubmitEvent) {
    e.preventDefault();
    error = null;
    try {
      const signer = nsecSigner(nsec);
      existing = { signer, method: "nsec", pubkey: await signer.getPublicKey() };
      nsec = "";
    } catch (err) {
      error = { text: err instanceof Error ? err.message : String(err) };
    }
  }

  async function copyKey() {
    await navigator.clipboard.writeText(newNsec);
    copied = true;
  }

  const passwordProblem = $derived(
    storage !== "server"
      ? ""
      : password.length < 10
        ? "Use at least 10 characters."
        : password !== password2
          ? "The passwords do not match."
          : "",
  );

  async function register(signer: ISigner, method: LoginMethod, ncryptsec?: string) {
    busy = true;
    error = null;
    try {
      const member = await api.register(signer, { name: name.trim(), email: email.trim(), ncryptsec });
      startSession(signer, method, member);
      session.flash = `Welcome, ${member.name}! We sent a confirmation link to ${member.email}.`;
      navigate("/home");
    } catch (err) {
      if (err instanceof ApiError && err.code === "duplicate_npub") {
        error = { text: "This key already belongs to a member, so it cannot be registered twice.", login: true };
      } else if (err instanceof ApiError && (err.code === "duplicate_email" || err.code === "invalid_email" || err.code === "invalid_name")) {
        error = { text: err.message };
        step = "details";
      } else {
        error = { text: err instanceof Error ? err.message : String(err) };
      }
    } finally {
      busy = false;
    }
  }

  async function submitExisting() {
    if (existing) await register(existing.signer, existing.method);
  }

  async function submitNew(e: SubmitEvent) {
    e.preventDefault();
    if (storage === "server") {
      if (passwordProblem) return;
      busy = true;
      let signer;
      try {
        signer = await passwordSignerFor(secretKey, password);
      } catch (err) {
        error = { text: err instanceof Error ? err.message : String(err) };
        busy = false;
        return;
      }
      await register(signer, "password", signer.ncryptsec);
    } else {
      await register(new PrivateKeySigner(secretKey), "nsec");
    }
  }
</script>

<main class="page narrow">
  <p><a href="#/">← Back</a></p>
  <div class="card stack">
    <h1>{route === "existing" ? "Register with your Nostr account" : "Create a new account"}</h1>
    <ol class="steps small muted">
      <li class:current={step === "details"}>Your details</li>
      <li class:current={step === "key"}>{route === "existing" ? "Your key" : "Your new key"}</li>
    </ol>

    {#if step === "details"}
      <form class="stack" onsubmit={next}>
        <label class="field">
          <span>Name or handle</span>
          <input type="text" bind:value={name} maxlength="64" autocomplete="nickname" required />
        </label>
        <label class="field">
          <span>Email</span>
          <input type="email" bind:value={email} autocomplete="email" required />
          <small class="muted">Required. We use it for important announcements and send a confirmation link.</small>
        </label>
        <button class="btn">Continue</button>
      </form>
    {:else if route === "existing"}
      <div class="stack">
        {#if existing}
          <div class="notice">
            Registering the key <span class="mono">{shortNpub(existing.pubkey)}</span>
            <button class="linkish small" onclick={() => (existing = null)}>use another key</button>
          </div>
          <p class="muted small">You keep your key. The community stores only your public key.</p>
          <button class="btn" onclick={submitExisting} disabled={busy}>{busy ? "Registering…" : "Register"}</button>
        {:else}
          <p>Prove that you hold the key by signing in with it.</p>
          <button class="btn block" onclick={chooseExtension}>Use browser extension</button>
          <details>
            <summary class="small">Or paste your secret key (nsec)</summary>
            <form class="stack" onsubmit={chooseNsec}>
              <p class="muted small">The key is only used in this browser tab; it is never sent to the server.</p>
              <input type="password" bind:value={nsec} placeholder="nsec1…" autocomplete="off" aria-label="Secret key" required />
              <button class="btn secondary">Use this key</button>
            </form>
          </details>
        {/if}
        <button class="linkish small" onclick={() => (step = "details")}>← Edit details</button>
      </div>
    {:else}
      <form class="stack" onsubmit={submitNew}>
        <p>Who should keep your key?</p>
        <RadioGroup.Root bind:value={storage} class="options">
          <RadioGroup.Item value="server" class="option">
            {#snippet children({ checked })}
              <span class="radio" class:on={checked}></span>
              <span>
                <strong>The community server, locked with my password</strong>
                <span class="muted small">Log in with email and password. The key is encrypted in this browser before it is stored; the server cannot use it.</span>
              </span>
            {/snippet}
          </RadioGroup.Item>
          <RadioGroup.Item value="self" class="option">
            {#snippet children({ checked })}
              <span class="radio" class:on={checked}></span>
              <span>
                <strong>I keep it myself</strong>
                <span class="muted small">Log in with a Nostr extension or by pasting the key. Email login will not be available.</span>
              </span>
            {/snippet}
          </RadioGroup.Item>
        </RadioGroup.Root>

        {#if storage === "server"}
          <label class="field">
            <span>Password</span>
            <input type="password" bind:value={password} autocomplete="new-password" required />
          </label>
          <label class="field">
            <span>Password again</span>
            <input type="password" bind:value={password2} autocomplete="new-password" required />
          </label>
          {#if password && passwordProblem}<p class="small warn-text">{passwordProblem}</p>{/if}
          <p class="notice warn small">There is no password recovery. If you forget it, your key is gone and you will need a new identity.</p>
        {:else}
          <div class="stack">
            <p class="small">This is your secret key. Save it somewhere safe, such as a password manager — anyone with it can act as you, and we cannot recover it.</p>
            <div class="keybox mono">{newNsec}</div>
            <button type="button" class="btn secondary" onclick={copyKey}>{copied ? "Copied" : "Copy key"}</button>
            <label class="check-row">
              <Checkbox.Root bind:checked={savedKey} class="checkbox">
                {#snippet children({ checked })}{checked ? "✓" : ""}{/snippet}
              </Checkbox.Root>
              <span class="small">I have saved my secret key somewhere safe.</span>
            </label>
          </div>
        {/if}

        <button class="btn" disabled={busy || (storage === "server" ? !!passwordProblem : !savedKey)}>
          {busy ? (storage === "server" ? "Encrypting your key…" : "Registering…") : "Create account"}
        </button>
        <button type="button" class="linkish small" onclick={() => (step = "details")}>← Edit details</button>
      </form>
    {/if}

    {#if error}
      <div class="notice error small" role="alert">
        {error.text}
        {#if error.login}<a href="#/">Log in instead</a>{/if}
      </div>
    {/if}
  </div>
</main>

<style>
  .narrow {
    max-width: 560px;
  }
  .steps {
    display: flex;
    gap: 20px;
    padding-left: 18px;
    margin: 0;
  }
  .steps .current {
    color: var(--text);
    font-weight: 600;
  }
  :global(.options) {
    display: grid;
    gap: 8px;
  }
  :global(.option) {
    display: flex;
    gap: 12px;
    text-align: left;
    align-items: flex-start;
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: 8px;
    padding: 12px;
    color: inherit;
  }
  :global(.option[data-state="checked"]) {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  :global(.option) span span {
    display: block;
    margin-top: 2px;
  }
  .radio {
    width: 18px;
    height: 18px;
    flex: none;
    border-radius: 50%;
    border: 2px solid var(--muted);
    margin-top: 2px;
  }
  .radio.on {
    border: 6px solid var(--accent);
  }
  .keybox {
    background: var(--surface-2);
    border-radius: 8px;
    padding: 12px;
    user-select: all;
  }
  .warn-text {
    color: var(--warn);
    margin: 0;
  }
</style>
