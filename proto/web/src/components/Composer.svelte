<script lang="ts">
  import { EventFactory } from "applesauce-core/factories";
  import { TagOperations } from "applesauce-core/operations";
  import { publish } from "../lib/nostr";
  import { session } from "../lib/session.svelte";

  let {
    group,
    placeholder = "Write a message…",
    label = "Send",
    onposted,
  }: { group: string; placeholder?: string; label?: string; onposted?: () => void } = $props();

  let text = $state("");
  let sending = $state(false);
  let error = $state("");

  async function send(e: SubmitEvent) {
    e.preventDefault();
    const content = text.trim();
    if (!content || !session.signer) return;
    sending = true;
    error = "";
    try {
      // NIP-29 group chat message: kind 9 tagged with the group id.
      const event = await EventFactory.fromKind(9)
        .content(content)
        .modifyPublicTags(TagOperations.setSingletonTag(["h", group]))
        .sign(session.signer);
      await publish(event);
      text = "";
      onposted?.();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      sending = false;
    }
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) (e.currentTarget as HTMLTextAreaElement).form?.requestSubmit();
  }
</script>

<form class="composer" onsubmit={send}>
  <textarea bind:value={text} {placeholder} rows="2" aria-label={placeholder} {onkeydown}></textarea>
  <div class="row">
    {#if error}<span class="error small">{error}</span>{:else}<span class="muted small">Ctrl+Enter to send</span>{/if}
    <button class="btn" disabled={sending || !text.trim()}>{sending ? "Sending…" : label}</button>
  </div>
</form>

<style>
  .composer textarea {
    min-height: 64px;
  }
  .row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    margin-top: 8px;
  }
  .error {
    color: var(--danger);
  }
</style>
