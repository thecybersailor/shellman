import { describe, expect, it } from "vitest";
import { resolveAPIContext } from "./api_context";

describe("resolveAPIContext", () => {
  it("resolves worker origin and turn uuid from query", () => {
    const out = resolveAPIContext(
      "http://127.0.0.1:5173/t/u1?worker_origin=http%3A%2F%2F127.0.0.1%3A8787&turn_uuid=u1"
    );
    expect(out.baseOrigin).toBe("http://127.0.0.1:8787");
    expect(out.turnUUID).toBe("u1");
  });

  it("falls back to current origin in local mode", () => {
    const out = resolveAPIContext("http://127.0.0.1:4621/");
    expect(out.baseOrigin).toBe("http://127.0.0.1:4621");
    expect(out.turnUUID).toBe("");
  });
});
