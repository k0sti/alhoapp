<script lang="ts">
  import Avatar from "../components/Avatar.svelte";
  import Composer from "../components/Composer.svelte";
  import DisplayName from "../components/DisplayName.svelte";
  import { api } from "../lib/api";
  import { when } from "../lib/format";
  import { ANNOUNCEMENTS_GROUP, eventStore, syncGroup } from "../lib/nostr";
  import { observe } from "../lib/observe.svelte";
  import { session } from "../lib/session.svelte";

  // The newest announcement is shown prominently, with a few earlier ones below.
  const EARLIER_SHOWN = 2;

  $effect(() => {
    const sub = syncGroup(ANNOUNCEMENTS_GROUP, 10);
    return () => sub.unsubscribe();
  });

  const timeline = observe(() => eventStore.timeline({ kinds: [9], "#h": [ANNOUNCEMENTS_GROUP] }), []);
  const announcements = $derived([...timeline.current].sort((a, b) => b.created_at - a.created_at));
  const latest = $derived(announcements[0]);
  const earlier = $derived(announcements.slice(1, 1 + EARLIER_SHOWN));

  const flash = session.flash;
  session.flash = "";

  let resent = $state<"" | "sending" | "sent" | "failed">("");
  async function resend() {
    if (!session.signer) return;
    resent = "sending";
    try {
      await api.resendConfirmation(session.signer);
      resent = "sent";
    } catch {
      resent = "failed";
    }
  }
</script>

<main class="page stack">
  {#if flash}<div class="notice" role="status">{flash}</div>{/if}

  <section>
    <h1>Hello, <DisplayName pubkey={session.pubkey!} fallback={session.member?.name} /></h1>
    <p class="muted status">
      This is the first stage of the Alhovuori community system. Your membership comes with your own Nostr identity, and
      for now the one service is the community message board. More will follow.
    </p>
  </section>

  {#if session.member && !session.member.verified && session.member.email}
    <div class="notice warn small">
      Please confirm your email address — we sent a link to <strong>{session.member.email}</strong>.
      {#if resent === "sent"}
        Sent again.
      {:else if resent === "failed"}
        Could not send; try again later.
      {:else}
        <button class="linkish" onclick={resend} disabled={resent === "sending"}>Send the link again</button>
      {/if}
    </div>
  {/if}

  <section class="card announcement" aria-labelledby="ann-h">
    <h2 id="ann-h" class="small muted label">Latest announcement</h2>
    {#if latest}
      <div class="byline">
        <Avatar pubkey={latest.pubkey} size={32} />
        <span><strong><DisplayName pubkey={latest.pubkey} /></strong> <span class="muted small">· {when(latest.created_at)}</span></span>
      </div>
      <p class="content">{latest.content}</p>
    {:else}
      <p class="muted">No announcements yet.</p>
    {/if}

    {#if earlier.length}
      <ul class="earlier">
        {#each earlier as a (a.id)}
          <li><span class="muted small">{when(a.created_at)}</span> {a.content}</li>
        {/each}
      </ul>
    {/if}

    {#if session.member?.isAdmin}
      <details class="post">
        <summary class="small">Post an announcement</summary>
        <Composer group={ANNOUNCEMENTS_GROUP} placeholder="Write an announcement…" label="Post announcement" />
      </details>
    {/if}
  </section>

  <a class="card board-link" href="#/board">
    <strong>Message board →</strong>
    <span class="muted small">The community conversation, one board for everyone.</span>
  </a>
</main>

<style>
  .status {
    max-width: 640px;
  }
  .label {
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .announcement {
    border-left: 4px solid var(--accent);
  }
  .byline {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .content {
    font-size: 1.15rem;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    margin: 12px 0 0;
  }
  .earlier {
    list-style: none;
    padding: 12px 0 0;
    margin: 16px 0 0;
    border-top: 1px solid var(--line);
    display: grid;
    gap: 6px;
  }
  .earlier li {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .post {
    margin-top: 16px;
  }
  .post summary {
    cursor: pointer;
    color: var(--accent);
    margin-bottom: 8px;
  }
  .board-link {
    display: block;
    text-decoration: none;
    color: inherit;
  }
  .board-link span {
    display: block;
  }
  .board-link:hover {
    border-color: var(--accent);
  }
</style>
