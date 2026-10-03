<script lang="ts">
  import { getProfilePicture } from "applesauce-core/helpers";
  import { generatedAvatar } from "../lib/avatar";
  import { eventStore } from "../lib/nostr";
  import { observe } from "../lib/observe.svelte";

  let { pubkey, size = 40 }: { pubkey: string; size?: number } = $props();

  const profile = observe(() => eventStore.profile(pubkey), undefined);
  let failed = $state(false);
  const src = $derived((!failed && getProfilePicture(profile.current)) || generatedAvatar(pubkey));
</script>

<img
  class="avatar"
  {src}
  alt=""
  width={size}
  height={size}
  style:width="{size}px"
  style:height="{size}px"
  onerror={() => (failed = true)}
/>

<style>
  .avatar {
    border-radius: 50%;
    object-fit: cover;
    background: var(--surface-2);
    flex: none;
    display: block;
  }
</style>
