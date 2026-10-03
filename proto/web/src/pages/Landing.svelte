<script lang="ts">
  import { Tabs } from "bits-ui";
  import { navigate } from "../lib/router.svelte";
  import { EmailLoginError, NotMemberError, begin, extensionSigner, loginWithEmail, nsecSigner } from "../lib/session.svelte";

  let tab = $state("nostr");
  let busy = $state<"" | "extension" | "nsec" | "email">("");
  let error = $state<{ text: string; action?: "register-key" | "use-nostr" | "no-recovery" } | null>(null);

  let nsec = $state("");
  let email = $state("");
  let password = $state("");

  async function attempt(kind: typeof busy, run: () => Promise<void>) {
    busy = kind;
    error = null;
    try {
      await run();
      navigate("/home");
    } catch (err) {
      if (err instanceof NotMemberError) {
        error = { text: "This key is not registered with the community yet.", action: "register-key" };
      } else if (err instanceof EmailLoginError) {
        error = {
          text: err.message,
          action: err.code === "self_hosted" ? "use-nostr" : err.code === "wrong_password" ? "no-recovery" : undefined,
        };
      } else {
        error = { text: err instanceof Error ? err.message : String(err) };
      }
    } finally {
      busy = "";
    }
  }

  const loginExtension = () => attempt("extension", async () => begin(await extensionSigner(), "extension"));
  const loginNsec = (e: SubmitEvent) => {
    e.preventDefault();
    attempt("nsec", () => begin(nsecSigner(nsec), "nsec"));
  };
  const loginEmail = (e: SubmitEvent) => {
    e.preventDefault();
    attempt("email", () => loginWithEmail(email.trim(), password));
  };
</script>

<main class="page">
  <section class="intro">
    <h1>Alhovuori community</h1>
    <p>
      A shared space for the people of Alhovuori. In this first stage every member receives their own digital identity — a
      Nostr key that belongs to them, not to us — and the first service is a single message board for the whole community.
    </p>
    <p class="muted">
      Important announcements are also sent by email, so we ask everyone for an email address when they join.
    </p>
  </section>

  <div class="columns">
    <section class="card" aria-labelledby="login-h">
      <h2 id="login-h">Log in</h2>
      <Tabs.Root bind:value={tab}>
        <Tabs.List class="tabs">
          <Tabs.Trigger value="nostr">With Nostr</Tabs.Trigger>
          <Tabs.Trigger value="email">With email</Tabs.Trigger>
        </Tabs.List>

        <Tabs.Content value="nostr" class="stack">
          <p class="muted small">Sign in with the key you hold yourself.</p>
          <button class="btn block" onclick={loginExtension} disabled={!!busy}>
            {busy === "extension" ? "Waiting for extension…" : "Log in with browser extension"}
          </button>
          <details>
            <summary class="small">Or paste your secret key (nsec)</summary>
            <form class="stack" onsubmit={loginNsec}>
              <p class="muted small">The key stays in this browser tab and is forgotten when you close it or log out.</p>
              <label class="field">
                <span>Secret key</span>
                <input type="password" bind:value={nsec} placeholder="nsec1…" autocomplete="off" required />
              </label>
              <button class="btn secondary block" disabled={!!busy || !nsec}>Log in with nsec</button>
            </form>
          </details>
        </Tabs.Content>

        <Tabs.Content value="email">
          <form class="stack" onsubmit={loginEmail}>
            <p class="muted small">For members whose key is kept on the server, protected by their password.</p>
            <label class="field">
              <span>Email</span>
              <input type="email" bind:value={email} autocomplete="email" required />
            </label>
            <label class="field">
              <span>Password</span>
              <input type="password" bind:value={password} autocomplete="current-password" required />
            </label>
            <button class="btn block" disabled={!!busy}>{busy === "email" ? "Unlocking your key…" : "Log in"}</button>
          </form>
        </Tabs.Content>
      </Tabs.Root>

      {#if error}
        <div class="notice error small" role="alert">
          <p>{error.text}</p>
          {#if error.action === "register-key"}
            <button class="linkish" onclick={() => navigate("/register/existing")}>Register with this key</button>
          {:else if error.action === "use-nostr"}
            <button class="linkish" onclick={() => (tab = "nostr")}>Log in with Nostr instead</button>
          {:else if error.action === "no-recovery"}
            <p>
              Keys cannot be recovered without the password. If you have lost it, an administrator can retire your old
              membership so that you can <a href="#/register/new">register a new identity</a>.
            </p>
          {/if}
        </div>
      {/if}
    </section>

    <section class="card" aria-labelledby="register-h">
      <h2 id="register-h">Register</h2>
      <div class="stack">
        <a class="choice" href="#/register/existing">
          <strong>I already have a Nostr account</strong>
          <span class="muted small">Join with your existing key. You keep the key; we only store your public key.</span>
        </a>
        <a class="choice" href="#/register/new">
          <strong>Create a new account</strong>
          <span class="muted small">We generate a key for you. Keep it yourself, or let us store it locked with your password.</span>
        </a>
      </div>
    </section>
  </div>
</main>

<style>
  .intro {
    max-width: 680px;
    margin-bottom: 28px;
  }
  .intro h1 {
    font-size: 2rem;
  }
  .columns {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 20px;
    align-items: start;
  }
  @media (max-width: 720px) {
    .columns {
      grid-template-columns: 1fr;
    }
  }
  :global(.tabs) {
    display: flex;
    gap: 4px;
    border-bottom: 1px solid var(--line);
    margin-bottom: 14px;
  }
  :global(.tabs [role="tab"]) {
    background: none;
    border: none;
    padding: 8px 12px;
    color: var(--muted);
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
  }
  :global(.tabs [role="tab"][data-state="active"]) {
    color: var(--text);
    border-bottom-color: var(--accent);
    font-weight: 600;
  }
  details summary {
    cursor: pointer;
    color: var(--muted);
    margin-top: 4px;
  }
  details form {
    margin-top: 10px;
  }
  .notice {
    margin-top: 16px;
  }
  .notice p {
    margin: 0 0 6px;
  }
  .choice {
    display: block;
    border: 1px solid var(--line);
    border-radius: 8px;
    padding: 14px;
    color: inherit;
    text-decoration: none;
  }
  .choice:hover {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .choice span {
    display: block;
    margin-top: 2px;
  }
</style>
