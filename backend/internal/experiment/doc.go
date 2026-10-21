// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

// Package experiment administers feature flags and experiments and analyses
// experiments (ABT-001, ABT-003, ABT-004, ABT-005, ABT-006, ABT-007,
// ADR-0079).
//
// # Flags
//
// A flag belongs to an environment and has a type (bool, number, string or
// json with a JSON Schema), a default and an ordered list of per-segment
// values; the first segment a device belongs to wins. The flag's definition
// in the app document supplies the default a device uses before it holds a
// control document; the values administered here reach devices through the
// control document of every channel of the environment, with no release.
// A flag's type never changes (PLX-8203): the flag is deleted and made again.
//
// # Experiments
//
// An experiment is created from an experiment document
// (schema/json/experiment.schema.json), the same file the project holds so
// that the compiler builds its variants into the release. Creating it only
// registers it. Starting it writes its allocation into the control documents
// of the environment; stopping it takes the allocation out. Experiments that
// share a layer are mutually exclusive: each holds a range of the layer's
// 10,000 buckets while it runs, and a start that finds no free range of the
// requested size is refused (PLX-8200). A holdout share of the environment's
// subjects, kept out of every experiment, is set separately. Assignment
// itself happens on the device and is specified in schema/control.fbs.
//
// # Analysis
//
// Analyse reads, per variant, the sufficient statistics of the exposure and
// metric events from an [Observations] source and compares each variant with
// the control through package stats: confidence sequences valid under
// continuous monitoring, a sample-ratio check (PLX-8202) and CUPED over the 14
// days before each subject's first exposure. A result is refused until every
// variant has the experiment's minimum sample size (PLX-8201). The default
// source reads the raw events, not the rollups: rollups hold counts and sums
// per bucket, while the variance of a mean and a covariate per subject need
// per-subject data.
//
// # Promotion
//
// Promote hands the winner to the compiler's PromoteWinner pass and replaces
// the draft with its result through the document service, then starts a
// publish job through the release service: the release is made by the normal
// publish path, with every gate it has, and nothing is bypassed.
//
// # Seams
//
// [ApprovalGate] is where the approval engine (ABT-007) decides whether an
// experiment on a secure page or a money flow may start; [EngineGate]
// implements it with the engine, rating the launch from the draft, and
// [AllowAll] is the default where no engine is wired. [SegmentSource] supplies the rule of a segment.
// [Documents], [Publisher] and [ControlWriter] are the narrow views of the
// document and release services this package uses.
//
// Invariants: every call is scoped to the principal's organisation; every
// change to flags or experiments and the control documents it produces
// commit in one transaction; no limit is hard-coded.
package experiment
