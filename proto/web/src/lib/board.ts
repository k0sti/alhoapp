// Message board display logic, kept free of Svelte and Nostr so it can be
// tested on its own.
//
// Each message sits on one of three rungs: avatar only, a single line, or the
// full message. History collapses per person: everyone's latest message is
// shown in full and their earlier ones as single lines. Filtered people drop to
// avatar only, and runs of avatar-only messages merge into one row of faces.

export const AVATAR = 0;
export const LINE = 1;
export const FULL = 2;
export type Rung = typeof AVATAR | typeof LINE | typeof FULL;

export interface BoardMessage {
  id: string;
  pubkey: string;
  created_at: number;
  content: string;
}

export interface BoardView {
  /** Rungs the member set by hand this session. They win over the defaults,
   *  so an expanded message never re-collapses when its author posts again. */
  overrides: ReadonlyMap<string, Rung>;
  /** People the member has filtered. */
  filtered: ReadonlySet<string>;
}

export type Row =
  | { kind: "message"; message: BoardMessage; rung: typeof LINE | typeof FULL }
  | { kind: "faces"; messages: BoardMessage[] };

/** Oldest first; ties broken by id so the order is the same for everyone. */
export function chronological(messages: Iterable<BoardMessage>): BoardMessage[] {
  return [...messages].sort((a, b) => a.created_at - b.created_at || (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
}

/** The id of each author's most recent message. Expects chronological input. */
export function latestByAuthor(sorted: BoardMessage[]): Map<string, string> {
  const latest = new Map<string, string>();
  for (const m of sorted) latest.set(m.pubkey, m.id);
  return latest;
}

export function defaultRung(message: BoardMessage, latest: Map<string, string>, filtered: ReadonlySet<string>): Rung {
  if (filtered.has(message.pubkey)) return AVATAR;
  return latest.get(message.pubkey) === message.id ? FULL : LINE;
}

export function rungOf(message: BoardMessage, latest: Map<string, string>, view: BoardView): Rung {
  return view.overrides.get(message.id) ?? defaultRung(message, latest, view.filtered);
}

export function raise(rung: Rung): Rung {
  return Math.min(FULL, rung + 1) as Rung;
}

export function lower(rung: Rung): Rung {
  return Math.max(AVATAR, rung - 1) as Rung;
}

/** Lays the board out as rows: consecutive avatar-only messages share a row. */
export function buildRows(messages: Iterable<BoardMessage>, view: BoardView): Row[] {
  const sorted = chronological(messages);
  const latest = latestByAuthor(sorted);
  const rows: Row[] = [];
  for (const message of sorted) {
    const rung = rungOf(message, latest, view);
    if (rung === AVATAR) {
      const last = rows.at(-1);
      if (last?.kind === "faces") last.messages.push(message);
      else rows.push({ kind: "faces", messages: [message] });
    } else {
      rows.push({ kind: "message", message, rung });
    }
  }
  return rows;
}

/** The single-line form of a message: its first non-empty line. */
export function firstLine(content: string): string {
  return content.split("\n").find((line) => line.trim() !== "")?.trim() ?? "";
}
