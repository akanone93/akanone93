// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the analytics area (STU-114, ANL-020, ANL-040). */
export function analyticsMessages(i18n: I18n) {
  return {
    overviewTitle: i18n._({
      id: "analytics.overviewTitle",
      message: "Analytics",
      comment: "Heading of the analytics overview.",
    }),
    overviewCaption: i18n._({
      id: "analytics.overviewCaption",
      message:
        "What happened in your apps over the chosen days. Personal data is redacted before it reaches this page.",
      comment: "Caption under the analytics heading.",
    }),
    navUsage: i18n._({
      id: "analytics.navUsage",
      message: "Usage",
      comment: "Link to the usage page.",
    }),
    navFunnels: i18n._({
      id: "analytics.navFunnels",
      message: "Funnels",
      comment: "Link to the funnels page.",
    }),
    navErrors: i18n._({
      id: "analytics.navErrors",
      message: "Errors",
      comment: "Link to the errors page.",
    }),
    navReplays: i18n._({
      id: "analytics.navReplays",
      message: "Replays",
      comment: "Link to the session replays of the app.",
    }),
    replaysTitle: i18n._({
      id: "analytics.replaysTitle",
      message: "Session replays",
      comment: "Heading of the list of session replays.",
    }),
    replaysCaption: i18n._({
      id: "analytics.replaysCaption",
      message:
        "Structural recordings of sessions that users agreed to share. They hold screens, layout, taps and non-sensitive values, never text or images.",
      comment: "Caption under the session replays heading.",
    }),
    noReplays: i18n._({
      id: "analytics.noReplays",
      message: "No replays were recorded in this environment.",
      comment: "Shown when the list of replays is empty.",
    }),
    replayStarted: i18n._({
      id: "analytics.replayStarted",
      message: "Started",
      comment: "Column: when a replay started.",
    }),
    replayRoute: i18n._({
      id: "analytics.replayRoute",
      message: "Screen",
      comment: "Column: the first screen of a replay.",
    }),
    replayDuration: i18n._({
      id: "analytics.replayDuration",
      message: "Duration (ms)",
      comment: "Column: how long a replay lasted.",
    }),
    replayEvents: i18n._({
      id: "analytics.replayEvents",
      message: "Events",
      comment: "Column: number of events in a replay.",
    }),
    replayTruncated: i18n._({
      id: "analytics.replayTruncated",
      message: "Cut at a limit",
      comment: "Column: whether the recording stopped at a limit.",
    }),
    backReplays: i18n._({
      id: "analytics.backReplays",
      message: "Back to replays",
      comment: "Link back to the list of replays.",
    }),
    replayTitle: i18n._({
      id: "analytics.replayTitle",
      message: "Session replay",
      comment: "Heading of one session replay.",
    }),
    replayTimeline: i18n._({
      id: "analytics.replayTimeline",
      message: "Timeline",
      comment: "Label of the slider that moves through a replay.",
    }),
    replayPlay: i18n._({
      id: "analytics.replayPlay",
      message: "Play",
      comment: "Button that plays a replay.",
    }),
    replayPause: i18n._({
      id: "analytics.replayPause",
      message: "Pause",
      comment: "Button that pauses a replay.",
    }),
    replayCanvas: i18n._({
      id: "analytics.replayCanvas",
      message: "Replay canvas",
      comment: "Label of the drawing that shows the recorded layout.",
    }),
    replayMasked: i18n._({
      id: "analytics.replayMasked",
      message: "Hidden",
      comment: "Label on a node whose content is never recorded.",
    }),
    replayState: i18n._({
      id: "analytics.replayState",
      message: "State so far",
      comment: "Heading of the table of state entries at the chosen moment.",
    }),
    replayStateName: i18n._({
      id: "analytics.replayStateName",
      message: "Entry",
      comment: "Column: the name of a state entry.",
    }),
    replayStateType: i18n._({
      id: "analytics.replayStateType",
      message: "Type",
      comment: "Column: the type of a state entry.",
    }),
    replayStateValue: i18n._({
      id: "analytics.replayStateValue",
      message: "Value",
      comment: "Column: the value of a state entry; empty when it is not recorded.",
    }),
    navPerformance: i18n._({
      id: "analytics.navPerformance",
      message: "Performance",
      comment: "Link to the performance page.",
    }),
    navOverview: i18n._({
      id: "analytics.navOverview",
      message: "Overview",
      comment: "Link back to the analytics overview.",
    }),
    rangeFrom: i18n._({
      id: "analytics.rangeFrom",
      message: "From",
      comment: "Label of the first day of the range.",
    }),
    rangeTo: i18n._({
      id: "analytics.rangeTo",
      message: "To",
      comment: "Label of the last day of the range.",
    }),
    appColumn: i18n._({
      id: "analytics.appColumn",
      message: "App",
      comment: "Column heading: the app.",
    }),
    sessions: i18n._({
      id: "analytics.sessions",
      message: "Sessions",
      comment: "Label: number of sessions.",
    }),
    screenViews: i18n._({
      id: "analytics.screenViews",
      message: "Screen views",
      comment: "Label: number of screen views.",
    }),
    errorsLabel: i18n._({
      id: "analytics.errorsLabel",
      message: "Errors",
      comment: "Label: number of errors.",
    }),
    firingAlerts: i18n._({
      id: "analytics.firingAlerts",
      message: "Firing alerts",
      comment: "Label: number of alerts that are firing.",
    }),
    screenViewsChart: i18n._({
      id: "analytics.screenViewsChart",
      message: "Screen views per day",
      comment: "Accessible name of the daily screen views chart.",
    }),
    dayColumn: i18n._({
      id: "analytics.dayColumn",
      message: "Day",
      comment: "Column heading: a calendar day.",
    }),
    noData: i18n._({
      id: "analytics.noData",
      message: "No data in this range.",
      comment: "Shown when the range holds no events.",
    }),
    usageTitle: i18n._({
      id: "analytics.usageTitle",
      message: "Usage",
      comment: "Heading of the usage page.",
    }),
    usageCaption: i18n._({
      id: "analytics.usageCaption",
      message: "Screen views by plugin and page. Choose a plugin, then a page, to drill down.",
      comment: "Caption of the usage page.",
    }),
    pluginColumn: i18n._({
      id: "analytics.pluginColumn",
      message: "Plugin",
      comment: "Column heading: the plugin.",
    }),
    pageColumn: i18n._({
      id: "analytics.pageColumn",
      message: "Page",
      comment: "Column heading: the page.",
    }),
    viewsColumn: i18n._({
      id: "analytics.viewsColumn",
      message: "Views",
      comment: "Column heading: number of views.",
    }),
    allPlugins: i18n._({
      id: "analytics.allPlugins",
      message: "All plugins",
      comment: "Link that clears the plugin drill-down.",
    }),
    pluginsChart: i18n._({
      id: "analytics.pluginsChart",
      message: "Screen views by plugin",
      comment: "Accessible name of the plugin bars.",
    }),
    pagesChart: i18n._({
      id: "analytics.pagesChart",
      message: "Screen views by page",
      comment: "Accessible name of the page bars.",
    }),
    dailyChart: i18n._({
      id: "analytics.dailyChart",
      message: "Screen views of the page per day",
      comment: "Accessible name of the chart of one page.",
    }),
    funnelsTitle: i18n._({
      id: "analytics.funnelsTitle",
      message: "Funnels",
      comment: "Heading of the funnels page.",
    }),
    funnelsCaption: i18n._({
      id: "analytics.funnelsCaption",
      message: "Funnels count the sessions that reach each step, doing the steps in order.",
      comment: "Caption of the funnels page.",
    }),
    noFunnels: i18n._({
      id: "analytics.noFunnels",
      message: "This app has no funnels yet.",
      comment: "Shown when the app has no funnel.",
    }),
    funnelName: i18n._({
      id: "analytics.funnelName",
      message: "Name",
      comment: "Label of the funnel name field.",
    }),
    funnelSteps: i18n._({
      id: "analytics.funnelSteps",
      message: "Steps, one event name per line",
      comment: "Label of the funnel steps field.",
    }),
    createFunnel: i18n._({
      id: "analytics.createFunnel",
      message: "Create funnel",
      comment: "Button that creates a funnel.",
    }),
    deleteFunnel: i18n._({
      id: "analytics.deleteFunnel",
      message: "Delete",
      comment: "Button that deletes a funnel.",
    }),
    stepsColumn: i18n._({
      id: "analytics.stepsColumn",
      message: "Steps",
      comment: "Column heading: number of steps.",
    }),
    stepColumn: i18n._({
      id: "analytics.stepColumn",
      message: "Step",
      comment: "Column heading: a funnel step.",
    }),
    eventsColumn: i18n._({
      id: "analytics.eventsColumn",
      message: "Events",
      comment: "Column heading: number of events.",
    }),
    sessionsColumn: i18n._({
      id: "analytics.sessionsColumn",
      message: "Sessions",
      comment: "Column heading: number of sessions that reached a funnel step.",
    }),
    conversionColumn: i18n._({
      id: "analytics.conversionColumn",
      message: "Of first step",
      comment: "Column heading: share of the first step.",
    }),
    funnelChart: i18n._({
      id: "analytics.funnelChart",
      message: "Sessions per funnel step",
      comment: "Accessible name of the funnel bars.",
    }),
    backFunnels: i18n._({
      id: "analytics.backFunnels",
      message: "Back to funnels",
      comment: "Link back to the funnels list.",
    }),
    errorsTitle: i18n._({
      id: "analytics.errorsTitle",
      message: "Errors and crashes",
      comment: "Heading of the errors page.",
    }),
    errorsCaption: i18n._({
      id: "analytics.errorsCaption",
      message: "Errors grouped by fingerprint, newest activity first.",
      comment: "Caption of the errors page.",
    }),
    noErrors: i18n._({
      id: "analytics.noErrors",
      message: "No errors in this range.",
      comment: "Shown when there are no error groups.",
    }),
    codeColumn: i18n._({
      id: "analytics.codeColumn",
      message: "Code",
      comment: "Column heading: error code.",
    }),
    routeColumn: i18n._({
      id: "analytics.routeColumn",
      message: "Route",
      comment: "Column heading: route.",
    }),
    devicesColumn: i18n._({
      id: "analytics.devicesColumn",
      message: "Devices",
      comment: "Column heading: number of devices.",
    }),
    lastSeenColumn: i18n._({
      id: "analytics.lastSeenColumn",
      message: "Last seen",
      comment: "Column heading: when last seen.",
    }),
    trendColumn: i18n._({
      id: "analytics.trendColumn",
      message: "Trend",
      comment: "Column heading: trend by day.",
    }),
    fatalLabel: i18n._({
      id: "analytics.fatalLabel",
      message: "Fatal",
      comment: "Label of a crash.",
    }),
    trendLabel: i18n._({
      id: "analytics.trendLabel",
      message: "Events per day",
      comment: "Accessible name of a trend line.",
    }),
    errorTitle: i18n._({
      id: "analytics.errorTitle",
      message: "Error",
      comment: "Heading of one error group.",
    }),
    errorNotFound: i18n._({
      id: "analytics.errorNotFound",
      message: "No such error group.",
      comment: "Shown when the fingerprint is unknown.",
    }),
    occurrences: i18n._({
      id: "analytics.occurrences",
      message: "Occurrences",
      comment: "Heading of the occurrences table.",
    }),
    timeColumn: i18n._({
      id: "analytics.timeColumn",
      message: "Time",
      comment: "Column heading: time.",
    }),
    eventColumn: i18n._({
      id: "analytics.eventColumn",
      message: "Event",
      comment: "Column heading: event identifier.",
    }),
    symbolicated: i18n._({
      id: "analytics.symbolicated",
      message: "Symbolicated path",
      comment: "Heading of the symbolicated path.",
    }),
    nodePathLabel: i18n._({
      id: "analytics.nodePathLabel",
      message: "Node",
      comment: "Label of the node path.",
    }),
    jsonPathLabel: i18n._({
      id: "analytics.jsonPathLabel",
      message: "Document path",
      comment: "Label of the JSON path.",
    }),
    sourceFileLabel: i18n._({
      id: "analytics.sourceFileLabel",
      message: "Source file",
      comment: "Label of the source file.",
    }),
    framesLabel: i18n._({
      id: "analytics.framesLabel",
      message: "Frames",
      comment: "Label of the stack frames.",
    }),
    openNode: i18n._({
      id: "analytics.openNode",
      message: "Open node",
      comment: "Link that opens the plugin canvas at the failing node.",
    }),
    backErrors: i18n._({
      id: "analytics.backErrors",
      message: "Back to errors",
      comment: "Link back to the errors list.",
    }),
    performanceTitle: i18n._({
      id: "analytics.performanceTitle",
      message: "Performance",
      comment: "Heading of the performance page.",
    }),
    performanceCaption: i18n._({
      id: "analytics.performanceCaption",
      message: "Render time percentiles per day, in milliseconds.",
      comment: "Caption of the performance page.",
    }),
    performanceChart: i18n._({
      id: "analytics.performanceChart",
      message: "Render time percentiles per day",
      comment: "Accessible name of the percentile chart.",
    }),
  };
}

export type AnalyticsMessages = ReturnType<typeof analyticsMessages>;
