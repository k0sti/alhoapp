<script lang="ts">
  import { Checkbox, Popover } from "bits-ui";
  import { tick } from "svelte";
  import Avatar from "../components/Avatar.svelte";
  import Composer from "../components/Composer.svelte";
  import DisplayName from "../components/DisplayName.svelte";
  import { AVATAR, FULL, LINE, buildRows, firstLine, lower, raise, type BoardMessage, type Rung } from "../lib/board";
  import { when } from "../lib/format";
  import { COMMUNITY_GROUP, eventStore, syncGroup } from "../lib/nostr";
  import { observe } from "../lib/observe.svelte";
  import { session } from "../lib/session.svelte";

  const me = session.pubkey!;

  $effect(() => {
    const sub = syncGroup(COMMUNITY_GROUP);
    return () => sub.unsubscribe();
  });

  const timeline = observe(() => eventStore.timeline({ kinds: [9], "#h": [COMMUNITY_GROUP] }), []);
  const messages = $derived<BoardMessage[]>(timeline.current);

  // Rungs the member set by hand last for the browser session, so an expanded
  // message never collapses again on its own. Filters persist across sessions.
  const RUNGS_KEY = "alho:board-rungs";
  const FILTER_KEY = `alho:board-filtered:${me}`;

  function load<T>(storage: () => Storage, key: string, fallback: T): T {
    try {
      return JSON.parse(storage().getItem(key) ?? "null") ?? fallback;
    } catch {
      return fallback;
    }
  }
  function save(storage: () => Storage, key: string, value: unknown) {
    try {
      storage().setItem(key, JSON.stringify(value));
    } catch {}
  }

  let overrides = $state(new Map<string, Rung>(Object.entries(load(() => sessionStorage, RUNGS_KEY, {}))));
  let filtered = $state(new Set<string>(load(() => localStorage, FILTER_KEY, [])));

  const rows = $derived(buildRows(messages, { overrides, filtered }));

  function setRung(id: string, rung: Rung) {
    overrides = new Map(overrides).set(id, rung);
    save(() => sessionStorage, RUNGS_KEY, Object.fromEntries(overrides));
  }

  function toggleFilter(pubkey: string, on: boolean) {
    const next = new Set(filtered);
    if (on) next.add(pubkey);
    else next.delete(pubkey);
    filtered = next;
    save(() => localStorage, FILTER_KEY, [...next]);
  }

  // Everyone who has posted, most recent first, for the filter list.
  const participants = $derived(
    [...new Map([...messages].sort((a, b) => b.created_at - a.created_at).map((m) => [m.pubkey, m])).keys()].filter((p) => p !== me),
  );

  // Follow new messages only when the reader is already at the bottom; never
  // move the view while they are reading further up.
  let initialScrollDone = false;
  function atBottom() {
    return window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 120;
  }
  function scrollToEnd() {
    window.scrollTo({ top: document.documentElement.scrollHeight });
  }
  $effect.pre(() => {
    messages.length;
    const follow = !initialScrollDone || atBottom();
    tick().then(() => {
      if (follow && messages.length) {
        scrollToEnd();
        initialScrollDone = true;
      }
    });
  });
</script>

<main class="page board">
  <header class="board-head">
    <h1>Message board</h1>
    <Popover.Root>
      <Popover.Trigger class="btn secondary">
        Filter people{filtered.size ? ` (${filtered.size})` : ""}
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content class="popover filter" align="end" sideOffset={6}>
          <p class="small muted">Filtered people's messages shrink to their face. Click a face to read the message.</p>
          {#if participants.length === 0}
            <p class="small">Nobody else has posted yet.</p>
          {/if}
          <ul>
            {#each participants as pubkey (pubkey)}
              <li>
                <label class="check-row person">
                  <Checkbox.Root checked={filtered.has(pubkey)} onCheckedChange={(on) => toggleFilter(pubkey, on)} class="checkbox">
                    {#snippet children({ checked })}{checked ? "✓" : ""}{/snippet}
                  </Checkbox.Root>
                  <Avatar {pubkey} size={24} />
                  <span><DisplayName {pubkey} /></span>
                </label>
              </li>
            {/each}
          </ul>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  </header>

  <ol class="rows" aria-label="Messages, oldest first">
    {#if rows.length === 0}
      <li class="muted empty">No messages yet. Say hello!</li>
    {/if}
    {#each rows as row (row.kind === "faces" ? `faces:${row.messages[0].id}` : row.message.id)}
      {#if row.kind === "faces"}
        <li class="faces">
          {#each row.messages as m (m.id)}
            <button class="face" onclick={() => setRung(m.id, raise(AVATAR))} title="{when(m.created_at)} — show message">
              <Avatar pubkey={m.pubkey} size={28} />
              <span class="sr-only">Show message by <DisplayName pubkey={m.pubkey} /> at {when(m.created_at)}</span>
            </button>
          {/each}
        </li>
      {:else if row.rung === LINE}
        <li class="line" class:mine={row.message.pubkey === me}>
          <button class="raise" onclick={() => setRung(row.message.id, raise(LINE))} aria-expanded="false" title="Show the whole message">
            <Avatar pubkey={row.message.pubkey} size={24} />
            <strong class="who"><DisplayName pubkey={row.message.pubkey} /></strong>
            <span class="text">{firstLine(row.message.content)}</span>
            <time class="muted small">{when(row.message.created_at)}</time>
          </button>
          <button class="collapse" onclick={() => setRung(row.message.id, lower(LINE))} aria-label="Collapse to face" title="Collapse to face">–</button>
        </li>
      {:else}
        <li class="full" class:mine={row.message.pubkey === me}>
          <Avatar pubkey={row.message.pubkey} size={40} />
          <div class="body">
            <div class="meta">
              <strong><DisplayName pubkey={row.message.pubkey} /></strong>
              <time class="muted small">{when(row.message.created_at)}</time>
            </div>
            <p>{row.message.content}</p>
          </div>
          <button class="collapse" onclick={() => setRung(row.message.id, lower(FULL))} aria-label="Collapse to one line" title="Collapse to one line">–</button>
        </li>
      {/if}
    {/each}
  </ol>

  <div class="compose card">
    <Composer group={COMMUNITY_GROUP} onposted={() => tick().then(scrollToEnd)} />
  </div>
</main>

<style>
  .board {
    max-width: 760px;
  }
  .board-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    margin-bottom: 12px;
  }
  .board-head h1 {
    margin: 0;
  }
  :global(.filter) {
    width: min(320px, calc(100vw - 32px));
  }
  :global(.filter) ul {
    list-style: none;
    padding: 0;
    margin: 8px 0 0;
    max-height: 300px;
    overflow-y: auto;
  }
  .person {
    align-items: center;
    padding: 6px 4px;
  }
  .rows {
    list-style: none;
    padding: 0;
    margin: 0 0 16px;
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 6px;
  }
  .empty {
    padding: 24px 0;
    text-align: center;
  }
  .full,
  .line {
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: var(--radius);
    position: relative;
  }
  .mine {
    border-color: color-mix(in srgb, var(--accent) 35%, var(--line));
  }
  .full {
    display: flex;
    gap: 12px;
    padding: 12px 40px 12px 12px;
  }
  .body {
    min-width: 0;
    flex: 1;
  }
  .meta {
    display: flex;
    gap: 8px;
    align-items: baseline;
  }
  .full p {
    margin: 4px 0 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .line {
    display: flex;
    align-items: center;
  }
  .raise {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 8px;
    background: none;
    border: none;
    padding: 6px 4px 6px 10px;
    color: inherit;
    text-align: left;
  }
  .who {
    flex: none;
    max-width: 30%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .text {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .raise time {
    flex: none;
  }
  /* The collapse control: a quiet "–" that stays visible on touch screens and
     brightens on hover or focus. */
  .collapse {
    flex: none;
    width: 28px;
    height: 28px;
    margin: 0 6px;
    border: 1px solid transparent;
    border-radius: 6px;
    background: none;
    color: var(--muted);
    font-size: 1.1rem;
    line-height: 1;
  }
  .full .collapse {
    position: absolute;
    top: 8px;
    right: 0;
  }
  .collapse:hover,
  .collapse:focus-visible {
    border-color: var(--line);
    background: var(--surface-2);
    color: var(--text);
  }
  .faces {
    display: flex;
    flex-wrap: wrap;
    gap: 2px;
    padding: 2px 6px;
  }
  .face {
    position: relative;
    border: none;
    background: none;
    padding: 2px;
    border-radius: 50%;
    opacity: 0.75;
  }
  .face:hover,
  .face:focus-visible {
    opacity: 1;
  }
  .compose {
    position: sticky;
    bottom: 12px;
    padding: 12px;
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
</style>
