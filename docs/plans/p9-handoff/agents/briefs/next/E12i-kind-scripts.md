<!--
SPDX-FileCopyrightText: 2026 Plux contributors
SPDX-License-Identifier: Apache-2.0
-->

## Your task: P9 E12i — the two kind scripts that failed in CI (`DEP-004`, `DEP-040`)

CI run 39721074609 (no Docker here; follow the scripts carefully):
1. `make e2e-kind` (`deploy/helm/e2e-kind.sh`): `helm test` reports `Phase: Succeeded`, then fails with `Error: unable to get pod logs for plux-test: pods "plux-test" not found` — the test pod has a hook delete policy that removes it before `helm test --logs` reads it (grep `helm.sh/hook-delete-policy` in `deploy/helm/plux/templates/tests/`). Make the logs readable (e.g. `before-hook-creation` only, or drop `--logs` and read the pod log yourself before deletion) without leaving pods behind across runs.
2. `make airgap-kind` (`deploy/helm/airgap-kind.sh`, step "images"): `Error: chart requires kubeVersion: >=1.30.0-0 which is incompatible with Kubernetes v1.20.0`, followed by kubectl `connection refused` on localhost:8080 — helm and kubectl run without the kind cluster's kubeconfig (or before the cluster exists), so helm falls back to its default version. Make every helm/kubectl call in the script use the kind cluster's context (as `e2e-kind.sh` does), and fail early with a clear message if the cluster is not reachable.
Apply the same fixes to `deploy/helm/restore-drill.sh` if it shares the pattern (its seed step also hit a not-ready port-forward on 127.0.0.1:18081: wait for the forward to accept connections before using it).
Done when: `make helm-lint` passes and `bash -n` on the three scripts passes; report exactly what is untested.
