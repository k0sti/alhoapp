<script lang="ts">
  import { eventStore } from "../lib/nostr";
  import { observe } from "../lib/observe.svelte";
  import { shortNpub } from "../lib/format";

  let { pubkey, fallback }: { pubkey: string; fallback?: string } = $props();

  const profile = observe(() => eventStore.profile(pubkey), undefined);
  const name = $derived(profile.current?.display_name || profile.current?.name || fallback || shortNpub(pubkey));
</script>

{name}
