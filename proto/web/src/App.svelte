<script lang="ts">
  import TopBar from "./components/TopBar.svelte";
  import Admin from "./pages/Admin.svelte";
  import Board from "./pages/Board.svelte";
  import Home from "./pages/Home.svelte";
  import Landing from "./pages/Landing.svelte";
  import Profile from "./pages/Profile.svelte";
  import Register from "./pages/Register.svelte";
  import { syncProfiles } from "./lib/nostr";
  import { navigate, route } from "./lib/router.svelte";
  import { restoreSession, session } from "./lib/session.svelte";

  restoreSession();

  const PUBLIC = ["/", "/register/existing", "/register/new", "/verified", "/verify-failed"];
  const MEMBER = ["/home", "/board", "/profile", "/admin", "/verified", "/verify-failed"];
  const loggedIn = $derived(!!session.member);

  // Member pages need a session; members are sent home from anywhere else.
  $effect(() => {
    if (session.restoring) return;
    if (!loggedIn && !PUBLIC.includes(route.path)) navigate("/");
    else if (loggedIn && !MEMBER.includes(route.path)) navigate("/home");
  });

  // Every member profile is kept in the event store for names and avatars.
  $effect(() => {
    if (!loggedIn) return;
    const sub = syncProfiles();
    return () => sub.unsubscribe();
  });
</script>

{#if session.restoring}
  <main class="page"><p class="muted">Loading…</p></main>
{:else if loggedIn}
  <TopBar />
  {#if route.path === "/home"}
    <Home />
  {:else if route.path === "/board"}
    <Board />
  {:else if route.path === "/profile"}
    <Profile />
  {:else if route.path === "/admin" && session.member?.isAdmin}
    <Admin />
  {:else if route.path === "/verified" || route.path === "/verify-failed"}
    {@render verifyNotice()}
  {/if}
{:else if route.path === "/register/existing" || route.path === "/register/new"}
  {#key route.path}
    <Register route={route.path === "/register/new" ? "new" : "existing"} />
  {/key}
{:else if route.path === "/verified" || route.path === "/verify-failed"}
  {@render verifyNotice()}
{:else}
  <Landing />
{/if}

{#snippet verifyNotice()}
  <main class="page">
    <div class="card stack" style="max-width: 520px">
      {#if route.path === "/verified"}
        <h1>Email confirmed</h1>
        <p>Thank you — your email address is confirmed. Community announcements will reach you there.</p>
      {:else}
        <h1>Link not valid</h1>
        <p>This confirmation link is not valid. You can ask for a new one from the member home page.</p>
      {/if}
      <a class="btn" href={loggedIn ? "#/home" : "#/"}>Continue</a>
    </div>
  </main>
{/snippet}
