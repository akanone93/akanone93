// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the installation administration area (STU-116). */
export function installationMessages(i18n: I18n) {
  return {
    navLabel: i18n._({
      id: "installation.navLabel",
      message: "Installation",
      comment: "Text in the installation administration area.",
    }),
    navHealth: i18n._({
      id: "installation.navHealth",
      message: "Health",
      comment: "Text in the installation administration area.",
    }),
    navOrganisations: i18n._({
      id: "installation.navOrganisations",
      message: "Organisations",
      comment: "Text in the installation administration area.",
    }),
    navStorage: i18n._({
      id: "installation.navStorage",
      message: "Storage",
      comment: "Text in the installation administration area.",
    }),
    navJobs: i18n._({
      id: "installation.navJobs",
      message: "Jobs",
      comment: "Text in the installation administration area.",
    }),
    navBackups: i18n._({
      id: "installation.navBackups",
      message: "Backups",
      comment: "Text in the installation administration area.",
    }),
    navIssuers: i18n._({
      id: "installation.navIssuers",
      message: "CI issuers",
      comment: "Text in the installation administration area.",
    }),
    navAudit: i18n._({
      id: "installation.navAudit",
      message: "Audit chain",
      comment: "Text in the installation administration area.",
    }),
    loading: i18n._({
      id: "installation.loading",
      message: "Loading",
      comment: "Text in the installation administration area.",
    }),
    loadFailed: i18n._({
      id: "installation.loadFailed",
      message: "This could not be loaded.",
      comment: "Text in the installation administration area.",
    }),
    nothing: i18n._({
      id: "installation.nothing",
      message: "Nothing to show.",
      comment: "Text in the installation administration area.",
    }),
    healthTitle: i18n._({
      id: "installation.healthTitle",
      message: "Installation health",
      comment: "Text in the installation administration area.",
    }),
    healthCaption: i18n._({
      id: "installation.healthCaption",
      message: "Whether each part of the installation answers.",
      comment: "Text in the installation administration area.",
    }),
    colCheck: i18n._({
      id: "installation.colCheck",
      message: "Check",
      comment: "Text in the installation administration area.",
    }),
    colResult: i18n._({
      id: "installation.colResult",
      message: "Result",
      comment: "Text in the installation administration area.",
    }),
    healthy: i18n._({
      id: "installation.healthy",
      message: "Healthy",
      comment: "Text in the installation administration area.",
    }),
    unhealthy: i18n._({
      id: "installation.unhealthy",
      message: "Failing",
      comment: "Text in the installation administration area.",
    }),
    organisationsTitle: i18n._({
      id: "installation.organisationsTitle",
      message: "Organisations",
      comment: "Text in the installation administration area.",
    }),
    organisationsCaption: i18n._({
      id: "installation.organisationsCaption",
      message: "The organisations you belong to.",
      comment: "Text in the installation administration area.",
    }),
    colName: i18n._({
      id: "installation.colName",
      message: "Name",
      comment: "Text in the installation administration area.",
    }),
    colKey: i18n._({
      id: "installation.colKey",
      message: "Key",
      comment: "Text in the installation administration area.",
    }),
    colCreated: i18n._({
      id: "installation.colCreated",
      message: "Created",
      comment: "Text in the installation administration area.",
    }),
    storageTitle: i18n._({
      id: "installation.storageTitle",
      message: "Storage use",
      comment: "Text in the installation administration area.",
    }),
    storageCaption: i18n._({
      id: "installation.storageCaption",
      message: "How much space the database and the object store use.",
      comment: "Text in the installation administration area.",
    }),
    colArea: i18n._({
      id: "installation.colArea",
      message: "Area",
      comment: "Text in the installation administration area.",
    }),
    colBytes: i18n._({
      id: "installation.colBytes",
      message: "Size",
      comment: "Text in the installation administration area.",
    }),
    jobsTitle: i18n._({
      id: "installation.jobsTitle",
      message: "Job queues",
      comment: "Text in the installation administration area.",
    }),
    jobsCaption: i18n._({
      id: "installation.jobsCaption",
      message: "Background work by queue and state.",
      comment: "Text in the installation administration area.",
    }),
    colQueue: i18n._({
      id: "installation.colQueue",
      message: "Queue",
      comment: "Text in the installation administration area.",
    }),
    colState: i18n._({
      id: "installation.colState",
      message: "State",
      comment: "Text in the installation administration area.",
    }),
    colCount: i18n._({
      id: "installation.colCount",
      message: "Jobs",
      comment: "Text in the installation administration area.",
    }),
    backupsTitle: i18n._({
      id: "installation.backupsTitle",
      message: "Backups",
      comment: "Text in the installation administration area.",
    }),
    backupsCaption: i18n._({
      id: "installation.backupsCaption",
      message: "Backup status of the installation.",
      comment: "Text in the installation administration area.",
    }),
    backupsNote: i18n._({
      id: "installation.backupsNote",
      message:
        "The server records no backups yet. Take backups of the database and the object store with your own tooling and follow the runbook.",
      comment: "Text in the installation administration area.",
    }),
    issuersTitle: i18n._({
      id: "installation.issuersTitle",
      message: "Trusted CI issuers",
      comment: "Text in the installation administration area.",
    }),
    issuersCaption: i18n._({
      id: "installation.issuersCaption",
      message: "The identity providers whose CI workloads may exchange a token.",
      comment: "Text in the installation administration area.",
    }),
    colKind: i18n._({
      id: "installation.colKind",
      message: "Kind",
      comment: "Text in the installation administration area.",
    }),
    issuer: i18n._({
      id: "installation.issuer",
      message: "Issuer",
      comment: "Text in the installation administration area.",
    }),
    audience: i18n._({
      id: "installation.audience",
      message: "Audience",
      comment: "Text in the installation administration area.",
    }),
    subjectPattern: i18n._({
      id: "installation.subjectPattern",
      message: "Subject pattern",
      comment: "Text in the installation administration area.",
    }),
    yes: i18n._({ id: "installation.yes", message: "Yes", comment: "Text in the installation administration area." }),
    no: i18n._({ id: "installation.no", message: "No", comment: "Text in the installation administration area." }),
    auditTitle: i18n._({
      id: "installation.auditTitle",
      message: "Installation audit chain",
      comment: "Text in the installation administration area.",
    }),
    auditCaption: i18n._({
      id: "installation.auditCaption",
      message: "Changes at installation level, newest first.",
      comment: "Text in the installation administration area.",
    }),
    colTime: i18n._({
      id: "installation.colTime",
      message: "Time",
      comment: "Text in the installation administration area.",
    }),
    colActor: i18n._({
      id: "installation.colActor",
      message: "Actor",
      comment: "Text in the installation administration area.",
    }),
    colAction: i18n._({
      id: "installation.colAction",
      message: "Action",
      comment: "Text in the installation administration area.",
    }),
    colResource: i18n._({
      id: "installation.colResource",
      message: "Resource",
      comment: "Text in the installation administration area.",
    }),
  };
}
