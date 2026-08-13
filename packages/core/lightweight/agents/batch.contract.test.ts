import { describe, expect, it, vi } from "vitest";

import { runBatchOperation } from "./list";

describe("agent batch partial authorization contract", () => {
  it("returns per-item success and failure without aborting the batch", async () => {
    const operation = vi.fn(async (item: { id: string }) => {
      if (item.id === "forbidden") {
        throw new Error("forbidden");
      }
    });
    const summary = await runBatchOperation(
      [{ id: "allowed" }, { id: "forbidden" }, { id: "allowed-2" }],
      operation,
    );
    expect(summary).toEqual({
      succeeded: 2,
      failed: 1,
      results: [
        { id: "allowed", ok: true },
        { id: "forbidden", ok: false, error: "forbidden" },
        { id: "allowed-2", ok: true },
      ],
    });
    expect(operation).toHaveBeenCalledTimes(3);
  });
});
