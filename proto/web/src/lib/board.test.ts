import { describe, expect, it } from "vitest";
import { AVATAR, FULL, LINE, buildRows, firstLine, lower, raise, type BoardMessage, type Rung, type Row } from "./board";

let seq = 0;
function msg(pubkey: string, created_at: number, content = `${pubkey}@${created_at}`): BoardMessage {
  return { id: `id${String(++seq).padStart(4, "0")}`, pubkey, created_at, content };
}

function view(overrides: [string, Rung][] = [], filtered: string[] = []) {
  return { overrides: new Map(overrides), filtered: new Set(filtered) };
}

/** Compact picture of the rows: "a2" = message by a at FULL, "[b,c]" = faces. */
function shape(rows: Row[]): string[] {
  return rows.map((r) => (r.kind === "faces" ? `[${r.messages.map((m) => m.pubkey).join(",")}]` : `${r.message.pubkey}${r.rung}`));
}

describe("buildRows", () => {
  it("shows each person's latest message in full and earlier ones as single lines", () => {
    const rows = buildRows([msg("a", 1), msg("b", 2), msg("a", 3), msg("c", 4)], view());
    expect(shape(rows)).toEqual(["a1", "b2", "a2", "c2"]);
  });

  it("collapses a person's previous message when they post again", () => {
    const first = [msg("a", 1), msg("b", 2)];
    expect(shape(buildRows(first, view()))).toEqual(["a2", "b2"]);
    expect(shape(buildRows([...first, msg("a", 3)], view()))).toEqual(["a1", "b2", "a2"]);
  });

  it("keeps a message the member expanded expanded after its author posts again", () => {
    const a1 = msg("a", 1);
    const all = [a1, msg("a", 2), msg("a", 3)];
    expect(shape(buildRows(all, view([[a1.id, FULL]])))).toEqual(["a2", "a1", "a2"]);
  });

  it("keeps chronological order regardless of input order or rungs", () => {
    const messages = [msg("c", 30), msg("a", 10), msg("b", 20)];
    const rows = buildRows(messages, view([[messages[1].id, LINE]]));
    expect(rows.map((r) => (r.kind === "message" ? r.message.created_at : -1))).toEqual([10, 20, 30]);
  });

  it("breaks timestamp ties by id so order is stable", () => {
    const x = msg("x", 5);
    const y = msg("y", 5);
    expect(shape(buildRows([y, x], view()))).toEqual(["x2", "y2"]);
  });

  it("drops filtered people to avatar only and merges consecutive ones into a row of faces", () => {
    const rows = buildRows([msg("a", 1), msg("f", 2), msg("g", 3), msg("f", 4), msg("a", 5), msg("g", 6)], view([], ["f", "g"]));
    expect(shape(rows)).toEqual(["a1", "[f,g,f]", "a2", "[g]"]);
  });

  it("lets a face be raised out of its row, splitting the row", () => {
    const f1 = msg("f", 1);
    const f2 = msg("f", 2);
    const f3 = msg("f", 3);
    const rows = buildRows([f1, f2, f3], view([[f2.id, LINE]], ["f"]));
    expect(shape(rows)).toEqual(["[f]", "f1", "[f]"]);
  });

  it("lets the member collapse an unfiltered message to avatar only", () => {
    const b = msg("b", 2);
    expect(shape(buildRows([msg("a", 1), b], view([[b.id, AVATAR]])))).toEqual(["a2", "[b]"]);
  });
});

describe("rung ladder", () => {
  it("raises one rung at a time and stops at full", () => {
    expect(raise(AVATAR)).toBe(LINE);
    expect(raise(LINE)).toBe(FULL);
    expect(raise(FULL)).toBe(FULL);
  });

  it("lowers one rung at a time and stops at avatar", () => {
    expect(lower(FULL)).toBe(LINE);
    expect(lower(LINE)).toBe(AVATAR);
    expect(lower(AVATAR)).toBe(AVATAR);
  });
});

describe("firstLine", () => {
  it("uses the first non-empty line", () => {
    expect(firstLine("\n  hello there \nsecond")).toBe("hello there");
    expect(firstLine("")).toBe("");
  });
});
