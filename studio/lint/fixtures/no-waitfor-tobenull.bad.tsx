import { expect } from "bun:test";
import { waitFor } from "@testing-library/react";

export async function bad(find: () => HTMLElement | null): Promise<void> {
  await waitFor(() => expect(find()).toBeNull());
  await waitFor(() => {
    expect(find()).toBeNull();
  });
}
