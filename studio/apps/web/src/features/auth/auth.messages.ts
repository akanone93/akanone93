// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: AGPL-3.0-only

import type { I18n } from "@lingui/core";

/** Messages of the sign-in and invitation pages (STU-012). */
export function authMessages(i18n: I18n) {
  return {
    signInTitle: i18n._({ id: "auth.signin.title", message: "Sign in", comment: "Heading of the sign-in page." }),
    email: i18n._({ id: "auth.field.email", message: "Email", comment: "Label of the email field." }),
    password: i18n._({ id: "auth.field.password", message: "Password", comment: "Label of the password field." }),
    submit: i18n._({ id: "auth.signin.submit", message: "Sign in", comment: "Button that submits the password." }),
    submitting: i18n._({
      id: "auth.signin.submitting",
      message: "Signing in",
      comment: "Button label while the sign-in request runs.",
    }),
    emailInvalid: i18n._({
      id: "auth.error.email",
      message: "Enter a valid email address.",
      comment: "Validation error of the email field.",
    }),
    passwordRequired: i18n._({
      id: "auth.error.password",
      message: "Enter your password.",
      comment: "Validation error of the password field.",
    }),
    rejected: i18n._({
      id: "auth.error.rejected",
      message: "The email or password is not correct.",
      comment: "Shown when the server refuses the credentials.",
    }),
    failed: i18n._({
      id: "auth.error.failed",
      message: "Sign-in failed. Try again in a moment.",
      comment: "Shown when the sign-in request fails for another reason.",
    }),
    factorTitle: i18n._({
      id: "auth.factor.title",
      message: "Second factor",
      comment: "Heading of the second-factor step.",
    }),
    code: i18n._({
      id: "auth.factor.code",
      message: "Authentication code",
      comment: "Label of the field for the one-time code.",
    }),
    codeInvalid: i18n._({
      id: "auth.error.code",
      message: "Enter the 6-digit code.",
      comment: "Validation error of the one-time code.",
    }),
    codeRejected: i18n._({
      id: "auth.error.code-rejected",
      message: "The code is not correct or has expired.",
      comment: "Shown when the server refuses the one-time code.",
    }),
    verify: i18n._({ id: "auth.factor.verify", message: "Verify", comment: "Button that submits the one-time code." }),
    passkey: i18n._({
      id: "auth.factor.passkey",
      message: "Use a passkey",
      comment: "Button that answers the second factor with a passkey.",
    }),
    passkeyFailed: i18n._({
      id: "auth.error.passkey",
      message: "The passkey could not be used.",
      comment: "Shown when the passkey prompt fails or is cancelled.",
    }),
    providers: i18n._({
      id: "auth.providers",
      message: "Single sign-on",
      comment: "Accessible name of the group of single sign-on buttons.",
    }),
    continueWith: (name: string) =>
      i18n._({
        id: "auth.provider.continue",
        message: "Continue with {name}",
        values: { name },
        comment: "Button that starts single sign-on; name is the identity provider.",
      }),
    inviteTitle: i18n._({
      id: "auth.invite.title",
      message: "Accept your invitation",
      comment: "Heading of the invitation page.",
    }),
    displayName: i18n._({
      id: "auth.invite.name",
      message: "Your name",
      comment: "Label of the display-name field on the invitation page.",
    }),
    nameRequired: i18n._({
      id: "auth.error.name",
      message: "Enter your name.",
      comment: "Validation error of the display-name field.",
    }),
    newPassword: i18n._({
      id: "auth.invite.password",
      message: "Choose a password",
      comment: "Label of the new-password field on the invitation page.",
    }),
    passwordShort: i18n._({
      id: "auth.error.password-short",
      message: "Use at least 12 characters.",
      comment: "Validation error of the new password.",
    }),
    accept: i18n._({
      id: "auth.invite.accept",
      message: "Accept invitation",
      comment: "Button that accepts the invitation.",
    }),
    inviteFailed: i18n._({
      id: "auth.error.invite",
      message: "The invitation is invalid or has expired.",
      comment: "Shown when the invitation is refused.",
    }),
    wordmark: i18n._({
      id: "auth.hero.wordmark",
      message: "Plux Studio",
      comment: "Product name in the corner of the sign-in page; translate only if the product name is localised.",
    }),
    heroLineOne: i18n._({
      id: "auth.hero.line-one",
      message: "Design it here.",
      comment: "First line of the large headline of the sign-in page. Keep it short: about 16 characters.",
    }),
    heroLineTwo: i18n._({
      id: "auth.hero.line-two",
      message: "Live everywhere.",
      comment: "Second line of the large headline: the screens reach every device. About 16 characters.",
    }),
    heroLead: i18n._({
      id: "auth.hero.lead",
      message:
        "Studio builds the screens your Flutter apps download, verify and render natively, with no store release.",
      comment: "Sentence under the headline of the sign-in page, saying what Plux Studio does.",
    }),
    sceneLabel: i18n._({
      id: "auth.hero.scene",
      message: "Sample app screens built with Plux, turning one by one",
      comment: "Accessible description of the animated picture beside the sign-in form.",
    }),
    factSigned: i18n._({
      id: "auth.fact.signed",
      message: "Signed releases",
      comment: "One of three short facts under the sign-in form: every release is signed.",
    }),
    factInstant: i18n._({
      id: "auth.fact.instant",
      message: "Updates without a store release",
      comment: "One of three short facts under the sign-in form.",
    }),
    factNative: i18n._({
      id: "auth.fact.native",
      message: "Rendered natively by Flutter",
      comment: "One of three short facts under the sign-in form.",
    }),
    storyLiveTitle: i18n._({
      id: "auth.story.live.title",
      message: "Updated live",
      comment: "One of five short stories under the sample screen on the sign-in page. Its title.",
    }),
    storyLiveBody: i18n._({
      id: "auth.story.live.body",
      message: "Change a screen in Studio and every app shows it the next time it opens. No store release.",
      comment: "One of five short stories under the sample screen on the sign-in page. Its sentence.",
    }),
    storyTestTitle: i18n._({
      id: "auth.story.test.title",
      message: "A/B tests",
      comment: "One of five short stories under the sample screen on the sign-in page. Its title.",
    }),
    storyTestBody: i18n._({
      id: "auth.story.test.body",
      message: "Show two versions to two halves of your users, then keep the one that works.",
      comment: "One of five short stories under the sample screen on the sign-in page. Its sentence.",
    }),
    storySignedTitle: i18n._({
      id: "auth.story.signed.title",
      message: "Signed releases",
      comment: "One of five short stories under the sample screen on the sign-in page. Its title.",
    }),
    storySignedBody: i18n._({
      id: "auth.story.signed.body",
      message: "Every release is signed in Studio and verified on the device before it is shown.",
      comment: "One of five short stories under the sample screen on the sign-in page. Its sentence.",
    }),
    storyOfflineTitle: i18n._({
      id: "auth.story.offline.title",
      message: "Works offline",
      comment: "One of five short stories under the sample screen on the sign-in page. Its title.",
    }),
    storyOfflineBody: i18n._({
      id: "auth.story.offline.body",
      message: "Screens and data stay on the device, so the app keeps working without a connection.",
      comment: "One of five short stories under the sample screen on the sign-in page. Its sentence.",
    }),
    storyRolloutTitle: i18n._({
      id: "auth.story.rollout.title",
      message: "Staged rollouts",
      comment: "One of five short stories under the sample screen on the sign-in page. Its title.",
    }),
    storyRolloutBody: i18n._({
      id: "auth.story.rollout.body",
      message: "Release to a few devices first, watch how it does, then to everyone.",
      comment: "One of five short stories under the sample screen on the sign-in page. Its sentence.",
    }),
    loanTitle: i18n._({
      id: "auth.scene.title",
      message: "Loan calculator",
      comment: "Text in a sample app screen drawn beside the sign-in form.",
    }),
    loanBorrow: i18n._({
      id: "auth.scene.borrow",
      message: "You borrow",
      comment: "Text in a sample app screen drawn beside the sign-in form.",
    }),
    loanTerm: i18n._({
      id: "auth.scene.loan-term",
      message: "Term",
      comment: "Text in a sample app screen drawn beside the sign-in form. Label of the loan duration.",
    }),
    loanMonths: (months: string) =>
      i18n._({
        id: "auth.scene.loan-months",
        message: "{months} months",
        comment: "Text in a sample app screen drawn beside the sign-in form. {months} is a number.",
        values: { months },
      }),
    loanRate: i18n._({
      id: "auth.scene.loan-rate",
      message: "Rate",
      comment: "Text in a sample app screen drawn beside the sign-in form. Label of the interest rate.",
    }),
    loanMonthly: i18n._({
      id: "auth.scene.monthly",
      message: "Monthly",
      comment: "Text in a sample app screen drawn beside the sign-in form.",
    }),
    loanApply: i18n._({
      id: "auth.scene.apply",
      message: "Apply",
      comment: "Text in a sample app screen drawn beside the sign-in form. A button.",
    }),
    loanLive: i18n._({
      id: "auth.scene.loan-live",
      message: "Live",
      comment: "Text in a sample app screen drawn beside the sign-in form. Small badge saying the screen just updated.",
    }),
    shopTitle: i18n._({
      id: "auth.scene.shop-title",
      message: "New in",
      comment: "Text in a sample app screen drawn beside the sign-in form. Heading of a shop screen.",
    }),
    shopProduct: i18n._({
      id: "auth.scene.shop-product",
      message: "Linen shirt",
      comment: "Text in a sample app screen drawn beside the sign-in form. A product name.",
    }),
    shopColour: i18n._({
      id: "auth.scene.shop-colour",
      message: "Sand",
      comment: "Text in a sample app screen drawn beside the sign-in form. A colour name of the product.",
    }),
    shopAdd: i18n._({
      id: "auth.scene.shop-add",
      message: "Add to bag",
      comment: "Text in a sample app screen drawn beside the sign-in form. Button of variant A of an A/B test.",
    }),
    shopBuy: i18n._({
      id: "auth.scene.shop-buy",
      message: "Buy now",
      comment: "Text in a sample app screen drawn beside the sign-in form. Button of variant B of an A/B test.",
    }),
    shopVariantA: i18n._({
      id: "auth.scene.shop-variant-a",
      message: "Variant A",
      comment:
        "Text in a sample app screen drawn beside the sign-in form. Badge naming the first version in an A/B test.",
    }),
    shopVariantB: i18n._({
      id: "auth.scene.shop-variant-b",
      message: "Variant B",
      comment:
        "Text in a sample app screen drawn beside the sign-in form. Badge naming the second version in an A/B test.",
    }),
    bankBalance: i18n._({
      id: "auth.scene.bank-balance",
      message: "Balance",
      comment: "Text in a sample app screen drawn beside the sign-in form. Label above an account balance.",
    }),
    bankSend: i18n._({
      id: "auth.scene.bank-send",
      message: "Send",
      comment: "Text in a sample app screen drawn beside the sign-in form. A button that sends money.",
    }),
    bankRequest: i18n._({
      id: "auth.scene.bank-request",
      message: "Request",
      comment: "Text in a sample app screen drawn beside the sign-in form. A button that requests money.",
    }),
    bankToday: i18n._({
      id: "auth.scene.bank-today",
      message: "Today",
      comment: "Text in a sample app screen drawn beside the sign-in form. Heading of the list of recent payments.",
    }),
    bankCoffee: i18n._({
      id: "auth.scene.bank-coffee",
      message: "Corner Café",
      comment: "Text in a sample app screen drawn beside the sign-in form. Name of a shop in a payment list.",
    }),
    bankSalary: i18n._({
      id: "auth.scene.bank-salary",
      message: "Salary",
      comment: "Text in a sample app screen drawn beside the sign-in form. A payment in a payment list.",
    }),
    bankRent: i18n._({
      id: "auth.scene.bank-rent",
      message: "Rent",
      comment: "Text in a sample app screen drawn beside the sign-in form. A payment in a payment list.",
    }),
    bankVerified: i18n._({
      id: "auth.scene.bank-verified",
      message: "Verified",
      comment:
        "Text in a sample app screen drawn beside the sign-in form. Seal saying the screen's release signature was checked.",
    }),
    deliveryTitle: i18n._({
      id: "auth.scene.delivery-title",
      message: "On its way",
      comment: "Text in a sample app screen drawn beside the sign-in form. Title of a delivery tracking screen.",
    }),
    deliveryEta: (minutes: string) =>
      i18n._({
        id: "auth.scene.delivery-eta",
        message: "{minutes} min",
        comment:
          "Text in a sample app screen drawn beside the sign-in form. Minutes until the delivery arrives; {minutes} is a number.",
        values: { minutes },
      }),
    deliveryCourier: i18n._({
      id: "auth.scene.delivery-courier",
      message: "Sam is 1.2 km away",
      comment: "Text in a sample app screen drawn beside the sign-in form. Line about the courier.",
    }),
    deliveryTrack: i18n._({
      id: "auth.scene.delivery-track",
      message: "Track order",
      comment: "Text in a sample app screen drawn beside the sign-in form. A button.",
    }),
    deliveryOffline: i18n._({
      id: "auth.scene.delivery-offline",
      message: "Offline. Showing your saved order.",
      comment:
        "Text in a sample app screen drawn beside the sign-in form. Bar that slides in when the phone loses its connection.",
    }),
    onboardTitle: i18n._({
      id: "auth.scene.onboard-title",
      message: "Fresh, every morning",
      comment: "Text in a sample app screen drawn beside the sign-in form. Title of a welcome screen.",
    }),
    onboardBody: i18n._({
      id: "auth.scene.onboard-body",
      message: "Groceries at your door in 30 minutes.",
      comment: "Text in a sample app screen drawn beside the sign-in form. Sentence on a welcome screen.",
    }),
    onboardStart: i18n._({
      id: "auth.scene.onboard-start",
      message: "Get started",
      comment: "Text in a sample app screen drawn beside the sign-in form. A button.",
    }),
    onboardRollout: (percent: string) =>
      i18n._({
        id: "auth.scene.onboard-rollout",
        message: "Live on {percent} of devices",
        comment:
          "Text in a sample app screen drawn beside the sign-in form. Badge of a staged rollout; {percent} is a formatted percentage.",
        values: { percent },
      }),
  };
}
