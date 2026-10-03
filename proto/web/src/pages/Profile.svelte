<script lang="ts">
  import { ProfileFactory } from "applesauce-core/factories";
  import { isValidProfile } from "applesauce-core/helpers";
  import Avatar from "../components/Avatar.svelte";
  import { npub } from "../lib/format";
  import { eventStore, publish } from "../lib/nostr";
  import { observe } from "../lib/observe.svelte";
  import { logout, session } from "../lib/session.svelte";

  const pubkey = session.pubkey!;
  const member = $derived(session.member!);
  const profileEvent = observe(() => eventStore.replaceable(0, pubkey), undefined);
  const profile = observe(() => eventStore.profile(pubkey), undefined);

  let editing = $state(false);
  let name = $state("");
  let about = $state("");
  let picture = $state("");
  let saving = $state(false);
  let error = $state("");
  let copied = $state(false);

  function edit() {
    name = profile.current?.name ?? member.name;
    about = profile.current?.about ?? "";
    picture = profile.current?.picture ?? "";
    error = "";
    editing = true;
  }

  async function save(e: SubmitEvent) {
    e.preventDefault();
    if (!session.signer) return;
    saving = true;
    error = "";
    try {
      const current = profileEvent.current;
      const base = isValidProfile(current) ? ProfileFactory.modify(current) : ProfileFactory.create();
      const event = await base.update({ name: name.trim(), about: about.trim(), picture: picture.trim() || undefined }).sign(session.signer);
      await publish(event);
      editing = false;
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      saving = false;
    }
  }

  async function copyNpub() {
    await navigator.clipboard.writeText(npub(pubkey));
    copied = true;
  }

  const loginLabel = { extension: "Nostr browser extension", nsec: "secret key (nsec)", password: "email and password" };
</script>

<main class="page narrow stack">
  <div class="card head">
    <Avatar {pubkey} size={88} />
    <div class="body">
      <h1>{profile.current?.display_name || profile.current?.name || member.name}</h1>
      {#if profile.current?.about}<p class="about">{profile.current.about}</p>{/if}
      {#if !profile.current?.picture}<p class="muted small">Your face is generated from your key until you set a picture.</p>{/if}
      {#if !editing}<button class="btn secondary" onclick={edit}>Edit profile</button>{/if}
    </div>
  </div>

  {#if editing}
    <form class="card stack" onsubmit={save}>
      <h2>Edit profile</h2>
      <p class="muted small">Your profile is published as Nostr metadata on the community relay.</p>
      <label class="field"><span>Name</span><input type="text" bind:value={name} maxlength="64" required /></label>
      <label class="field"><span>About</span><textarea bind:value={about} rows="3"></textarea></label>
      <label class="field"><span>Picture URL</span><input type="url" bind:value={picture} placeholder="https://…" /></label>
      {#if error}<p class="notice error small">{error}</p>{/if}
      <div class="actions">
        <button class="btn" disabled={saving}>{saving ? "Saving…" : "Save"}</button>
        <button type="button" class="btn secondary" onclick={() => (editing = false)}>Cancel</button>
      </div>
    </form>
  {/if}

  <section class="card">
    <h2>Account</h2>
    <dl>
      <dt>Public key</dt>
      <dd>
        <span class="mono">{npub(pubkey)}</span>
        <button class="linkish small" onclick={copyNpub}>{copied ? "copied" : "copy"}</button>
      </dd>
      {#if member.email}
        <dt>Email</dt>
        <dd>
          {member.email}
          {#if member.verified}<span class="badge ok">confirmed</span>{:else}<span class="badge warn">not confirmed</span>{/if}
        </dd>
      {/if}
      <dt>Your key</dt>
      <dd>
        {#if member.keyStorage === "server"}
          Stored on the community server, encrypted with your password.
        {:else}
          Kept by you. The server only knows your public key.
        {/if}
      </dd>
      {#if session.method}
        <dt>Logged in with</dt>
        <dd>{loginLabel[session.method]}</dd>
      {/if}
    </dl>
    <button class="btn secondary" onclick={logout}>Log out</button>
  </section>
</main>

<style>
  .narrow {
    max-width: 640px;
  }
  .head {
    display: flex;
    gap: 20px;
    align-items: flex-start;
  }
  .body {
    min-width: 0;
  }
  .about {
    white-space: pre-wrap;
  }
  .actions {
    display: flex;
    gap: 8px;
  }
  dl {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 8px 16px;
    margin: 0 0 16px;
  }
  dt {
    font-weight: 600;
  }
  dd {
    margin: 0;
    min-width: 0;
  }
  @media (max-width: 520px) {
    dl {
      grid-template-columns: 1fr;
    }
    dd {
      margin-bottom: 8px;
    }
  }
</style>
