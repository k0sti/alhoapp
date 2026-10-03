<script lang="ts">
  import { DropdownMenu } from "bits-ui";
  import Avatar from "./Avatar.svelte";
  import DisplayName from "./DisplayName.svelte";
  import { route, navigate } from "../lib/router.svelte";
  import { logout, session } from "../lib/session.svelte";

  const links = $derived([
    { path: "/home", label: "Home" },
    { path: "/board", label: "Message board" },
    ...(session.member?.isAdmin ? [{ path: "/admin", label: "Members" }] : []),
  ]);
</script>

<header>
  <div class="inner">
    <a class="brand" href="#/home">Alhovuori</a>
    <nav aria-label="Main">
      {#each links as link (link.path)}
        <a href="#{link.path}" aria-current={route.path === link.path ? "page" : undefined}>{link.label}</a>
      {/each}
    </nav>
    {#if session.pubkey}
      <DropdownMenu.Root>
        <DropdownMenu.Trigger class="avatar-btn" aria-label="Your profile and account">
          <Avatar pubkey={session.pubkey} size={36} />
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content class="menu" align="end" sideOffset={6}>
            <div class="who">
              <strong><DisplayName pubkey={session.pubkey} fallback={session.member?.name} /></strong>
              <div class="muted small">{session.member?.email}</div>
            </div>
            <DropdownMenu.Item onSelect={() => navigate("/profile")}>Your profile</DropdownMenu.Item>
            <DropdownMenu.Item onSelect={logout}>Log out</DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
    {/if}
  </div>
</header>

<style>
  header {
    background: var(--surface);
    border-bottom: 1px solid var(--line);
    position: sticky;
    top: 0;
    z-index: 10;
  }
  .inner {
    max-width: 960px;
    margin: 0 auto;
    padding: 8px 16px;
    display: flex;
    align-items: center;
    gap: 16px;
  }
  .brand {
    font-weight: 700;
    color: var(--text);
    text-decoration: none;
    font-size: 1.1rem;
  }
  nav {
    display: flex;
    gap: 4px;
    flex: 1;
    overflow-x: auto;
  }
  nav a {
    color: var(--muted);
    text-decoration: none;
    padding: 6px 10px;
    border-radius: 6px;
    white-space: nowrap;
  }
  nav a[aria-current="page"] {
    color: var(--text);
    background: var(--surface-2);
  }
  :global(.avatar-btn) {
    border: none;
    background: none;
    padding: 0;
    border-radius: 50%;
  }
  .who {
    padding: 8px 10px 10px;
    border-bottom: 1px solid var(--line);
    margin-bottom: 4px;
  }
</style>
