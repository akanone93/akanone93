// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import { Icon, type IconName } from "@plux/ui";
import type { ReactNode } from "react";
import type { authMessages } from "./auth.messages.ts";

type Messages = ReturnType<typeof authMessages>;

/**
 * One sample app screen on the sign-in page (U16.2). `base` lies on the screen; `layers` stand
 * above it from the lowest up. Each app has its own
 * brand colour (`app`), as real apps do; the story says which Plux feature the screen shows.
 */
export interface SampleScreen {
  readonly key: string;
  readonly app: "loan" | "shop" | "bank" | "delivery" | "fresh";
  readonly base: ReactNode;
  readonly layers: readonly ReactNode[];
  /** The caption under the screen: its tag plays the story's moment while the screen is in front. */
  readonly story: { readonly tag: ReactNode; readonly title: string; readonly body: string };
}

function money(locale: string, value: number, digits = 2): string {
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: "EUR",
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(value);
}

function percent(locale: string, value: number, digits = 0): string {
  return new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: digits }).format(value);
}

/** The status bar every screen shares: the time and three quiet indicators. */
function StatusBar({ locale }: { locale: string }) {
  const time = new Intl.DateTimeFormat(locale, { hour: "numeric", minute: "2-digit" }).format(
    new Date(2026, 0, 1, 9, 41),
  );
  return (
    <div className="ss-status">
      <span>{time}</span>
      <span className="ss-status-icons">
        <i />
        <i />
        <i />
      </span>
    </div>
  );
}

/** A tab bar of four quiet icons, the first one active. */
function TabBar({ icons }: { icons: readonly IconName[] }) {
  return (
    <div className="ss-tabs">
      {icons.map((icon, i) => (
        <span key={icon} data-active={i === 0}>
          <Icon name={icon} size="tab" />
        </span>
      ))}
    </div>
  );
}

/** Two values in one place: the first shows, then the second takes over (the screen's live moment). */
function Swap({ from, to, className }: { from: ReactNode; to: ReactNode; className?: string }) {
  return (
    <span className={`ss-swap ${className ?? ""}`}>
      <span className="ss-swap-from">{from}</span>
      <span className="ss-swap-to" aria-hidden="true">
        {to}
      </span>
    </span>
  );
}

export function sampleScreens(m: Messages, locale: string): SampleScreen[] {
  return [
    {
      key: "loan",
      app: "loan",
      story: {
        tag: (
          <>
            <span className="ss-live-dot" />
            {m.loanLive}
          </>
        ),
        title: m.storyLiveTitle,
        body: m.storyLiveBody,
      },
      base: (
        <>
          <StatusBar locale={locale} />
          <TabBar icons={["house", "clock", "user", "gear"]} />
        </>
      ),
      layers: [
        <div key="h" className="ss-header">
          <span className="ss-title">{m.loanTitle}</span>
          <span className="ss-caption">{m.loanBorrow}</span>
          <Swap className="ss-figure" from={money(locale, 12000, 0)} to={money(locale, 18000, 0)} />
        </div>,
        <div key="c" className="ss-card ss-loan-card">
          <div className="ss-slider">
            <div className="ss-slider-fill" />
            <div className="ss-slider-thumb" />
          </div>
          <div className="ss-row">
            <span className="ss-caption">{m.loanTerm}</span>
            <span className="ss-value">{m.loanMonths("36")}</span>
          </div>
          <div className="ss-row">
            <span className="ss-caption">{m.loanRate}</span>
            <span className="ss-value">{percent(locale, 0.049, 1)}</span>
          </div>
          <div className="ss-row ss-row-total">
            <span className="ss-label">{m.loanMonthly}</span>
            <Swap className="ss-amount" from={money(locale, 359.12)} to={money(locale, 538.68)} />
          </div>
        </div>,
        <div key="a" className="ss-action">
          {m.loanApply}
        </div>,
      ],
    },
    {
      key: "shop",
      app: "shop",
      story: {
        tag: (
          <>
            <Icon name="flask" size="pill" />
            <Swap from={m.shopVariantA} to={m.shopVariantB} />
          </>
        ),
        title: m.storyTestTitle,
        body: m.storyTestBody,
      },
      base: (
        <>
          <StatusBar locale={locale} />
          <TabBar icons={["house", "magnifying-glass", "package", "user"]} />
        </>
      ),
      layers: [
        <div key="h" className="ss-header ss-header-row">
          <span className="ss-title ss-title-large">{m.shopTitle}</span>
          <span className="ss-icon-button">
            <Icon name="package" size="tab" />
          </span>
        </div>,
        <div key="p" className="ss-card ss-product">
          <div className="ss-product-image">
            <div className="ss-product-shape" />
          </div>
          <div className="ss-row">
            <span className="ss-label">{m.shopProduct}</span>
            <span className="ss-amount">{money(locale, 59, 0)}</span>
          </div>
          <div className="ss-row">
            <span className="ss-caption">{m.shopColour}</span>
            <span className="ss-swatches">
              <i data-swatch="1" />
              <i data-swatch="2" />
              <i data-swatch="3" />
            </span>
          </div>
        </div>,
        <Swap
          key="a"
          className="ss-action-swap"
          from={<span className="ss-action">{m.shopAdd}</span>}
          to={<span className="ss-action ss-action-alt">{m.shopBuy}</span>}
        />,
      ],
    },
    {
      key: "bank",
      app: "bank",
      story: {
        tag: (
          <span className="ss-stamp">
            <Icon name="check-circle" size="pill" />
            {m.bankVerified}
          </span>
        ),
        title: m.storySignedTitle,
        body: m.storySignedBody,
      },
      base: (
        <>
          <StatusBar locale={locale} />
          <TabBar icons={["house", "arrow-clockwise", "key", "user"]} />
        </>
      ),
      layers: [
        <div key="h" className="ss-header">
          <span className="ss-caption">{m.bankBalance}</span>
          <span className="ss-figure">{money(locale, 8240.5)}</span>
        </div>,
        <div key="k" className="ss-bank-card">
          <i className="ss-bank-chip" />
          <i className="ss-bank-wave" />
        </div>,
        <div key="l" className="ss-card ss-ledger">
          <div className="ss-pair">
            <span>{m.bankSend}</span>
            <span>{m.bankRequest}</span>
          </div>
          <span className="ss-caption">{m.bankToday}</span>
          <div className="ss-row">
            <span className="ss-label">{m.bankCoffee}</span>
            <span className="ss-value">{money(locale, -4.2)}</span>
          </div>
          <div className="ss-row">
            <span className="ss-label">{m.bankSalary}</span>
            <span className="ss-value ss-positive">{money(locale, 3200)}</span>
          </div>
          <div className="ss-row">
            <span className="ss-label">{m.bankRent}</span>
            <span className="ss-value">{money(locale, -950)}</span>
          </div>
        </div>,
      ],
    },
    {
      key: "delivery",
      app: "delivery",
      story: {
        tag: (
          <>
            <Icon name="wifi-slash" size="pill" />
            {m.tagOffline}
          </>
        ),
        title: m.storyOfflineTitle,
        body: m.storyOfflineBody,
      },
      base: <StatusBar locale={locale} />,
      layers: [
        <div key="h" className="ss-header ss-header-row">
          <span className="ss-title ss-title-large">{m.deliveryTitle}</span>
          <span className="ss-eta">{m.deliveryEta("12")}</span>
        </div>,
        <div key="m" className="ss-map">
          <svg viewBox="0 0 232 180" aria-hidden="true">
            <path d="M24 150 C 70 150, 60 90, 110 88 S 170 40, 206 32" />
          </svg>
          <i className="ss-map-home" />
          <i className="ss-map-pin" />
        </div>,
        <div key="c" className="ss-card ss-courier">
          <div className="ss-row">
            <span className="ss-avatar" />
            <span className="ss-label">{m.deliveryCourier}</span>
          </div>
          <div className="ss-action">{m.deliveryTrack}</div>
        </div>,
        <div key="s" className="ss-moment ss-offline">
          <Icon name="wifi-slash" size="pill" />
          {m.deliveryOffline}
        </div>,
      ],
    },
    {
      key: "fresh",
      app: "fresh",
      story: {
        tag: (
          <>
            <span className="ss-rollout-bar">
              <i />
            </span>
            <Swap from={percent(locale, 0.1)} to={percent(locale, 0.5)} />
          </>
        ),
        title: m.storyRolloutTitle,
        body: m.storyRolloutBody,
      },
      base: <StatusBar locale={locale} />,
      layers: [
        <div key="o" className="ss-orb">
          <i />
          <i />
          <i />
        </div>,
        <div key="t" className="ss-welcome">
          <span className="ss-title ss-title-large">{m.onboardTitle}</span>
          <span className="ss-caption">{m.onboardBody}</span>
          <span className="ss-dots">
            <i data-active="true" />
            <i />
            <i />
          </span>
        </div>,
        <div key="a" className="ss-action">
          {m.onboardStart}
        </div>,
      ],
    },
  ];
}
