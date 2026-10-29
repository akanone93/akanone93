import { expect } from "bun:test";
import { waitFor } from "@testing-library/react";

export async function good(find: () => HTMLElement | null): Promise<void> {
  await waitFor(() => expect(find() === null).toBe(true));
  expect(find()).toBeNull();
}
