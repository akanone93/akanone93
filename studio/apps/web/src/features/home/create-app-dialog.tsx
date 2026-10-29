// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { ReactElement } from "react";
import { NewAppWizard, type NewAppWizardProps } from "../new-app/index.ts";

/** Creates an app in the organisation (needs app.manage): the new-app wizard (STU-024), which opens the app's canvas. */
export function CreateAppDialog(props: NewAppWizardProps): ReactElement {
  return <NewAppWizard {...props} />;
}
