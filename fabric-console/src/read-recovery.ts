import { APIError } from "./api";

// Only caller-supplied reads belong here. Mutations require their recorded
// operation/idempotency protocol, and authorization failures are terminal.
export async function readWithRecovery<T>(
  read: () => Promise<T>,
  wait: (milliseconds: number) => Promise<void> = (milliseconds) =>
    new Promise((resolve) => setTimeout(resolve, milliseconds)),
): Promise<T> {
  for (let attempt = 0; ; attempt++) {
    try {
      return await read();
    } catch (error) {
      if (
        !(error instanceof APIError) ||
        error.code !== "connection" ||
        attempt >= 2
      )
        throw error;
      await wait(attempt === 0 ? 250 : 750);
    }
  }
}
