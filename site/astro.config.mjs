// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// The documentation site (ADR-0033, DX-002): Starlight renders the
// Markdown of docs/ in place, published to GitHub Pages.
import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";

const repository = "https://github.com/nightCode42/plux3";

export default defineConfig({
  site: "https://nightcode42.github.io",
  base: "/plux3",
  integrations: [
    starlight({
      title: "Plux",
      description:
        "Server-driven UI and plugins for Flutter: design in Studio, publish signed bundles, render native widgets.",
      social: [{ icon: "github", label: "GitHub", href: repository }],
      sidebar: [
        {
          label: "Start here",
          items: [
            { slug: "index" },
            { slug: "guides/concepts" },
            { slug: "guides/first-release" },
            { slug: "guides/run-locally" },
            { slug: "guides/host-app" },
            { slug: "guides/routing" },
            { slug: "guides/typed-api" },
            { slug: "guides/no-code-apps" },
          ],
        },
        {
          label: "Studio guides",
          collapsed: true,
          items: [
            { slug: "guides/studio" },
            { slug: "guides/studio/canvas" },
            { slug: "guides/studio/publishing" },
            { slug: "guides/studio/functions" },
            { slug: "guides/studio/data" },
            { slug: "guides/studio/localisation" },
            { slug: "guides/studio/design-system" },
            { slug: "guides/studio/analytics" },
            { slug: "guides/studio/experiments" },
            { slug: "guides/studio/plux-test" },
            { slug: "guides/studio/security-and-admin" },
          ],
        },
        {
          label: "Security guides",
          collapsed: true,
          items: [
            { slug: "guides/security" },
            { slug: "guides/device-trust" },
            { slug: "guides/gateway" },
            { slug: "guides/sca" },
            { slug: "guides/configuration" },
            { slug: "guides/confidential-bundles" },
            { slug: "guides/secure-ui" },
          ],
        },
        {
          label: "Functions",
          collapsed: true,
          items: [
            { slug: "functions" },
            { slug: "guides/functions" },
            { slug: "functions/abi" },
            { slug: "guides/function-builds" },
            { slug: "guides/function-security" },
          ],
        },
        { label: "Reference", collapsed: true, items: [{ autogenerate: { directory: "reference" } }] },
        { label: "Operations", collapsed: true, items: [{ autogenerate: { directory: "operations" } }] },
        { label: "Runbooks", collapsed: true, items: [{ autogenerate: { directory: "runbooks" } }] },
        { label: "Security", collapsed: true, items: [{ autogenerate: { directory: "security" } }] },
        { label: "Benchmarks", collapsed: true, items: [{ autogenerate: { directory: "benchmarks" } }] },
        { label: "Architecture decisions", collapsed: true, items: [{ autogenerate: { directory: "adr" } }] },
        { label: "Engineering handbook", collapsed: true, items: [{ autogenerate: { directory: "engineering" } }] },
        {
          label: "More",
          collapsed: true,
          items: [
            { slug: "requirements" },
            { slug: "accessibility/layer2-screen-reader" },
            { slug: "compliance" },
            { slug: "legal/cla" },
          ],
        },
      ],
    }),
  ],
});
