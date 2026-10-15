// Shared by every spec. The specs share one seeded server (backend/tests/e2e),
// so each spec creates what it changes under a unique code and only reads or
// touches its own part of the seeded studies.
import { expect } from "@playwright/test";

export const API = `http://127.0.0.1:${process.env.E2E_API_PORT || "4010"}`;

let seeded;
// The identifiers and credentials the server seeded.
export async function seed(request) {
  if (!seeded) {
    const response = await request.get(`${API}/e2e/seed`);
    expect(response.ok()).toBeTruthy();
    seeded = await response.json();
  }
  return seeded;
}

// A code no other spec and no earlier run uses.
export const unique = (prefix) => `${prefix}-${Date.now().toString(36).toUpperCase()}${Math.floor(Math.random() * 1296).toString(36).toUpperCase()}`;

export async function login(request, email, password) {
  const response = await request.post(`${API}/api/auth/login`, { data: { email, password } });
  expect(response.ok(), `login ${email}`).toBeTruthy();
  return (await response.json()).token;
}

// Calls the API as the given session, for setting up what a spec needs.
export function apiAs(request, token) {
  return async (method, path, body) => {
    const response = await request.fetch(`${API}/api${path}`, { method, data: body, headers: { Authorization: `Bearer ${token}` } });
    if (!response.ok()) throw new Error(`${method} ${path}: ${response.status()} ${await response.text()}`);
    return response.status() === 204 ? null : response.json();
  };
}

// Signs the page in as "admin" or a seeded role (LAB_MANAGER, RESEARCHER,
// TECHNICIAN, VIEWER) without the login form, in English, and returns the
// seed with an API client of that session.
export async function signIn(page, request, who = "admin") {
  const data = await seed(request);
  const account = who === "admin" ? { email: data.adminEmail, password: data.adminPassword } : data.accounts[who];
  const token = await login(request, account.email, account.password);
  await page.addInitScript((value) => {
    localStorage.setItem("misko_token", value);
    localStorage.setItem("misko.locale", "en");
  }, token);
  return { data, token, api: apiAs(request, token) };
}

// The text control behind a test hook: Naive UI puts data-test either on the
// input itself (input-props) or on the wrapper around it.
export function field(scope, testId) {
  const id = `[data-test="${testId}"]`;
  return scope.locator(`input${id}, textarea${id}, ${id} input, ${id} textarea`).first();
}

export async function fill(scope, testId, value) {
  const input = field(scope, testId);
  await input.fill(String(value));
}

// Options of the open select menu; a menu that is fading out after a choice is
// left out, so the next select can be used right away.
const options = (page) =>
  page.locator(".n-base-select-menu:not(.fade-in-scale-up-transition-leave-active) .n-base-select-option").filter({ visible: true });

// Naive UI's select is not a native <select>: open it, then click the option.
// Options built with selectOption carry their value in data-value. The menu is
// a virtual list, so an option far down a filterable list is found by typing
// `search` first.
export async function chooseValue(page, testId, value, { scope = page, search } = {}) {
  await scope.getByTestId(testId).first().click();
  if (search) await page.keyboard.type(search);
  await options(page).filter({ has: page.locator(`[data-value="${value}"]`) }).first().click();
}

// Picks an option by its visible text (a string or a RegExp).
export async function chooseText(page, testId, text, { scope = page } = {}) {
  await scope.getByTestId(testId).first().click();
  await options(page).filter({ hasText: text }).first().click();
}

// Confirms the open NPopconfirm: its positive button is the last one.
export async function confirm(page) {
  await page.locator(".n-popconfirm__action").filter({ visible: true }).locator(".n-button").last().click();
}

// Calibration of the fixture camera: a 320x240 view of a 50 cm square arena.
const FIT = [
  { pixel: [52, 28], world: [0, 0] },
  { pixel: [268, 36], world: [50, 0] },
  { pixel: [292, 222], world: [50, 50] },
  { pixel: [30, 214], world: [0, 50] },
];
const CHECK = [
  { pixel: [160.9466, 116.0652], world: [25, 25] },
  { pixel: [85.7359, 172.3152], world: [10, 40] },
  { pixel: [227.6114, 66.2066], world: [40, 10] },
];

export async function fillCalibration(form) {
  await form.getByTestId("camera-id").fill("e2e-camera");
  await form.getByTestId("frame-width").fill("320");
  await form.getByTestId("frame-height").fill("240");
  await form.getByTestId("crop-width").fill("320");
  await form.getByTestId("crop-height").fill("240");
  for (const [kind, points] of [["fit", FIT], ["check", CHECK]]) {
    for (const [index, p] of points.entries()) {
      await form.getByTestId(`${kind}-${index}-pixel-x`).fill(String(p.pixel[0]));
      await form.getByTestId(`${kind}-${index}-pixel-y`).fill(String(p.pixel[1]));
      await form.getByTestId(`${kind}-${index}-world-x`).fill(String(p.world[0]));
      await form.getByTestId(`${kind}-${index}-world-y`).fill(String(p.world[1]));
    }
  }
}
