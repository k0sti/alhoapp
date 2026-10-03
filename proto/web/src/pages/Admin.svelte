<script lang="ts">
  import { AlertDialog } from "bits-ui";
  import Avatar from "../components/Avatar.svelte";
  import { api, type Member } from "../lib/api";
  import { shortNpub } from "../lib/format";
  import { session } from "../lib/session.svelte";

  let members = $state<Member[] | null>(null);
  let error = $state("");
  let retiring = $state<Member | null>(null);
  let dialogOpen = $state(false);

  async function load() {
    try {
      members = await api.members(session.signer!);
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    }
  }
  load();

  const verifiedCount = $derived(members?.filter((m) => m.verified).length ?? 0);
  const dateFmt = new Intl.DateTimeFormat(undefined, { dateStyle: "medium" });

  function askRetire(m: Member) {
    retiring = m;
    dialogOpen = true;
  }

  async function retire() {
    if (!retiring) return;
    try {
      await api.deleteMember(session.signer!, retiring.pubkey);
      members = members?.filter((m) => m.pubkey !== retiring!.pubkey) ?? null;
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      dialogOpen = false;
    }
  }
</script>

<main class="page stack">
  <h1>Members</h1>
  {#if error}<p class="notice error">{error}</p>{/if}
  {#if members}
    <p class="muted">
      {members.length} members · {verifiedCount} confirmed their email ({members.length ? Math.round((verifiedCount / members.length) * 100) : 0}% reachable by email)
    </p>
    <div class="card table-wrap">
      <table>
        <thead>
          <tr><th>Member</th><th>Email</th><th>Confirmed</th><th>Key</th><th>Joined</th><th><span class="sr-only">Actions</span></th></tr>
        </thead>
        <tbody>
          {#each members as m (m.pubkey)}
            <tr>
              <td>
                <div class="who">
                  <Avatar pubkey={m.pubkey} size={28} />
                  <span>{m.name}<br /><span class="mono muted">{shortNpub(m.pubkey)}</span></span>
                </div>
              </td>
              <td>{m.email}</td>
              <td>
                {#if m.verified}<span class="badge ok">yes</span>{:else}<span class="badge warn">no</span>{/if}
              </td>
              <td>{m.keyStorage === "server" ? "on server" : "self-hosted"}</td>
              <td class="small">{dateFmt.format(new Date(m.createdAt))}</td>
              <td><button class="linkish small danger" onclick={() => askRetire(m)}>Retire…</button></td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {:else if !error}
    <p class="muted">Loading…</p>
  {/if}
</main>

<AlertDialog.Root bind:open={dialogOpen}>
  <AlertDialog.Portal>
    <AlertDialog.Overlay class="dialog-overlay" />
    <AlertDialog.Content class="dialog card stack">
      <AlertDialog.Title>Retire {retiring?.name}'s membership?</AlertDialog.Title>
      <AlertDialog.Description class="small">
        This deletes the member record and removes the key from the community groups. It is the first step of identity
        migration: afterwards the person can register again with a new identity. Their old messages stay on the board.
      </AlertDialog.Description>
      <div class="actions">
        <AlertDialog.Cancel class="btn secondary">Cancel</AlertDialog.Cancel>
        <AlertDialog.Action class="btn danger" onclick={retire}>Retire membership</AlertDialog.Action>
      </div>
    </AlertDialog.Content>
  </AlertDialog.Portal>
</AlertDialog.Root>

<style>
  .table-wrap {
    overflow-x: auto;
    padding: 0;
    /* keeps the visually hidden header inside the scroll area */
    position: relative;
  }
  table {
    width: 100%;
    border-collapse: collapse;
  }
  th,
  td {
    text-align: left;
    padding: 10px 14px;
    border-bottom: 1px solid var(--line);
    vertical-align: middle;
  }
  th {
    font-size: 0.8rem;
    color: var(--muted);
    font-weight: 600;
  }
  tbody tr:last-child td {
    border-bottom: none;
  }
  .who {
    display: flex;
    gap: 10px;
    align-items: center;
  }
  .danger {
    color: var(--danger);
  }
  .actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }
  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
  }
</style>
