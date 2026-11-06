// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { afterEach, describe, expect, mock, test } from "bun:test";
import { createStudioI18n } from "@plux/i18n";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { I18nProvider } from "../../i18n-provider.tsx";
import { loadSourceCatalogue } from "../../test-harness.tsx";
import { FormFieldPanel } from "./form-field-panel.tsx";
import { formsOf } from "./forms-model.ts";
import { FormPreviewContext, FormsSection } from "./forms-section.tsx";

afterEach(cleanup);

const wrap = (node: ReactNode) =>
  render(
    <I18nProvider studio={createStudioI18n({ load: async () => loadSourceCatalogue(), pseudo: false })}>
      {node}
    </I18nProvider>,
  );
const page = {
  forms: [{ id: "f", name: "signup", fields: [{ name: "email", type: "string", validators: [{ kind: "email" }] }] }],
};

describe("PC-26 form editors [STA-020]", () => {
  test("the forms section adds forms and fields, renames a field and previews a form's errors", async () => {
    const change = mock((_r: (d: Record<string, unknown>) => void, _f: string) => undefined);
    const set = mock((_p: string, _f: string, _m: string | undefined) => undefined);
    wrap(
      <FormPreviewContext.Provider value={{ modeOf: () => undefined, set }}>
        <FormsSection pageId="p" page={page} canEdit change={change} newId={() => "id"} />
      </FormPreviewContext.Provider>,
    );
    const name = (await screen.findAllByRole("textbox", { name: "Field name" }))[0] as HTMLInputElement;
    expect(name.value).toBe("email");
    fireEvent.change(name, { target: { value: "mail" } });
    fireEvent.blur(name);
    expect(change.mock.calls.at(-1)?.[1]).toBe("field:0:0:name");
    fireEvent.click(screen.getAllByRole("radio")[1] as HTMLElement);
    expect(set).toHaveBeenLastCalledWith("p", "signup", "errors");
    fireEvent.click(screen.getByRole("button", { name: /Add a field/ }));
    fireEvent.click(screen.getByRole("button", { name: /Add a form/ }));
    const draft: Record<string, unknown> = structuredClone(page);
    change.mock.calls.at(-1)?.[0](draft);
    expect(formsOf(draft).map((f) => f.name)).toEqual(["signup", "form"]);
  });

  test("an input binds to a form field from the field panel", async () => {
    const bind = mock((_f: string, _x: string) => undefined);
    wrap(
      <FormFieldPanel node={{ id: "tf", type: "TextField" }} pageId="p" forms={formsOf(page)} canEdit bind={bind} />,
    );
    fireEvent.click(await screen.findByRole("combobox"));
    fireEvent.click(await screen.findByRole("option", { name: "signup.email" }));
    expect(bind).toHaveBeenLastCalledWith("signup", "email");
  });
});
