---
title: Usage
description: How to use the Mişko panel day to day, from first login to running tests.
---

This page is the practical guide to using Mişko once it is installed. It is
written for everyone in the lab, not only developers. If you have not installed
Mişko yet, start with [Installation](../installation/).

## The big picture (non-technical)

Think of Mişko as the logbook for your behavioral experiments. Instead of
spreadsheets and scattered video files, every test is recorded in one place:
**which animal was tested, in which apparatus, by whom, on which device, and
what the result was**. The heavy video and tracking work is handled by a
separate camera system; Mişko keeps the organized summary so you can find and
compare results later.

A normal session looks like this:

1. An administrator sets up the lab: users, subjects (mice), and devices.
2. An operator creates a test by choosing a scenario, a subject, and a device.
3. The test moves through its lifecycle: it starts as **pending**, becomes
   **running** when the experiment is underway, and ends as **done** or
   **failed**.
4. The result summary is stored and visible on the dashboard.

## First login

There is no public sign-up. After installation a single **superadmin** account
exists, and its password was printed once to the backend log (see
[Installation](../installation/#get-the-first-login-password)).

1. Open the panel (default [http://localhost:8080](http://localhost:8080)).
2. Log in with `ADMIN_EMAIL` and the generated password.
3. Change the password immediately from your account.
4. Create the rest of the team from the **Users** screen.

## Users and roles

Users are managed internally by an administrator from the **Users** screen.
There is no self-registration, which keeps the system closed and suitable for a
single lab.

- An **ADMIN** can manage users and all lab data.
- Other users work with the lab data according to their role.
- When you create a user, you may leave the password blank and let the system
  generate a strong one; it is shown only once, so copy it before closing.

## Setting up the lab data

Before running tests, populate the building blocks. These live on their own
screens in the panel.

### Scenarios

Scenarios are based on four fixed apparatus types: `POOL`, `MAZE`, `STICK`, and
`PATH`. The catalog of scientific paradigms is defined in the system and stays
stable, so you select and configure rather than invent from scratch.

### Subjects

Subjects are the mice. Each subject has a code, sex, group, and notes. Use a
consistent coding scheme (for example `F-001`) so subjects are easy to find.

### Devices

Devices are the phones the tests run on (Android or iOS). Register each device
once so it can be picked when creating a test.

## Running a test

The test is the central record in Mişko. To run one:

1. Go to the **Tests** screen and create a new test.
2. Choose a **scenario**, a **subject**, an **operator**, and a **device**.
3. The test is created with status **pending**.
4. When the experiment starts, the status moves to **running**.
5. When it finishes, the status becomes **done** (or **failed** if something
   went wrong).

Once a test is done, its summary metrics and any artifact links are kept by
Mişko. The raw video and frame-by-frame data stay in the separate camera
service.

## The dashboard

The **Dashboard** is the home screen. It shows summary counts (how many
subjects, devices, tests) and the most recent tests, so you can see lab activity
at a glance.

## Where the heavy data lives

Mişko deliberately does not store raw video or computer-vision telemetry. That
data belongs to an independent camera service. Mişko stores only the **summary
metrics and artifact URLs** for each test. The contract between the two systems
is described in [Integration](../integration/).
