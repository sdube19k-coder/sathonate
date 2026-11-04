import { expect, it } from "vitest";
import { APIError } from "./api";
import { readWithRecovery } from "./read-recovery";

it("recovers initial workspace reads after a temporary connection failure", async () => {
  let requests = 0;
  const waits: number[] = [];
  const value = await readWithRecovery(
    async () => {
      if (++requests < 3) throw new APIError("Unavailable", "connection");
      return { project: "server-authorized-project" };
    },
    async (ms) => {
      waits.push(ms);
    },
  );
  expect(value.project).toBe("server-authorized-project");
  expect(requests).toBe(3);
  expect(waits).toEqual([250, 750]);
});

it("does not retry rejected authentication, authorization or other failures", async () => {
  for (const error of [
    new APIError("Expired", "authentication", [], 401),
    new APIError("Forbidden", "authorization", [], 403),
    new APIError("Server error", "request", [], 500),
    new Error("Unexpected application failure"),
  ]) {
    let requests = 0;
    await expect(
      readWithRecovery(async () => {
        requests++;
        throw error;
      }),
    ).rejects.toBe(error);
    expect(requests).toBe(1);
  }
});

it("bounds connection retries and propagates the final failure", async () => {
  const error = new APIError("Still unavailable", "connection");
  let requests = 0;
  await expect(
    readWithRecovery(
      async () => {
        requests++;
        throw error;
      },
      async () => {},
    ),
  ).rejects.toBe(error);
  expect(requests).toBe(3);
});
