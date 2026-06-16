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
**which animal was tested, under which scenario, by whom, and what the result
was**. The heavy video and tracking work is handled by a
separate camera system; Mişko keeps the organized summary so you can find and
compare results later.

A normal session looks like this:

1. An administrator sets up the lab: users, subjects (mice), environments and scenarios.
2. An operator creates a test by choosing a scenario and a subject.
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

There are five roles: `SUPERADMIN`, `LAB_MANAGER`, `RESEARCHER`, `TECHNICIAN`,
and `VIEWER`.

- A **SUPERADMIN** or **LAB_MANAGER** can manage users and configure the lab.
- Anyone with the `apparatus:write` permission (RESEARCHER and above) can create
  named environments from the read-only paradigm catalog.
- Other users work with the lab data according to their role.
- When you create a user, you may leave the password blank and let the system
  generate a strong one; it is shown only once, so copy it before closing.

## Configuring the lab

Mişko runs as a single laboratory per installation. The lab record (its name and
settings) is created by the installation wizard at first startup and is used for
branding across the panel.

The **Paradigms** screen is a read-only catalog of the scientific test types
defined in code. Each paradigm has its own detail page (open it by clicking a
card), where the operational contract is shown read-only: apparatus parameters,
zones, metrics, event types and quality control requirements.
The parameter set and their valid ranges are fixed in code and cannot be edited
here.

To put a paradigm to use, anyone with the `apparatus:write` permission
(RESEARCHER and above) creates an **environment** from it. An environment is a
named, persisted test setup based on a paradigm template, and a lab can hold many
environments for the same paradigm (for example two Morris water tanks, "Tank A"
and "Tank B"). From a paradigm detail page, use **Create environment from this
paradigm**, give it a name, and fill in the apparatus values within the
code-fixed allowed ranges. Manage environments from the **Environments** menu,
which supports create, edit and delete; on edit, the paradigm cannot be changed.
The apparatus values are locked at test time.

## Setting up the lab data

Before running tests, populate the building blocks. These live on their own
screens in the panel.

### Scenarios

A scenario is the reusable definition of an experiment. You build it once: give
it a name and select one or more **environments** (so a scenario can span one or
more paradigms; each environment's paradigm fixes which metrics it collects).
After that, running a test means choosing a subject and the scenario. A scenario
has no pass/fail criteria - results are data, interpreted later in analysis.

### Subjects

Subjects are the mice. Each subject has a code, sex, group, and notes. Use a
consistent coding scheme (for example `F-001`) so subjects are easy to find.

## Browsing and finding data

Every listing screen (Users, Subjects, Scenarios, Paradigms, Tests and
Environments) uses the same table, so the controls work the same way everywhere:

- **Search box** at the top filters the list by free text across the main
  columns (for example a subject code, a user email or a scenario name).
- **Sortable headers** show an arrow when active. Click a header to sort, click
  again to flip the direction.
- **Pagination** lives in the footer: choose how many rows per page (10, 20 or
  50), see the "from-to of total" summary, and move with Prev / Next.
- A leading **# column** numbers the rows and keeps counting across pages, so the
  first row on page 2 continues where page 1 left off.
- An **Export** dropdown at the top right saves the current page. CSV opens in
  Excel or Google Sheets; PDF opens the browser print dialog, where you can pick
  "Save as PDF" or print on paper. The export uses the values you see, including
  the current search and sort order.

All of this runs on the server, so the table stays fast no matter how many
records the lab accumulates: only the current page is fetched, and the search
and sort are applied by the database. When a screen has no records yet, the table
shows a short message naming what is missing instead of an empty grid.

Each listing is read-only. A **Create** button in the page header opens a
dedicated form page for a new record, and clicking a row's first column opens
that record's **detail page**, where you edit it, delete it, or run
record-specific actions (for example starting or finishing a test, or resetting
a user's password). Saving or deleting returns you to the list.

## Running a test

The test is the central record in Mişko. To run one:

1. Go to the **Tests** screen and click **Create** to open the new-test page.
2. Choose a **scenario** and a **subject**, then create. The operator is the
   signed-in user.
3. The test is created with status **pending**. Open its **detail page** (click
   the row) to run it.
4. The detail page lists the scenario's environments. Run them one by one: click
   **Start** on an environment, then **log events** as the run unfolds. Each event
   has a type (from the environment's paradigm), a timestamp in seconds, and an
   optional payload such as the zone entered.
5. Each environment panel shows a **metric counter bar** on top with the live
   metric values, and two tabs below it: a **Timeline** (the logged events placed
   in time order, plus the editable event list) and **Charts** (a parameter-based
   bar chart). Metrics recompute live as events are added or removed.
6. Click **Finish** to record that environment's result. Repeat for each
   environment.
7. When every environment is done the test becomes **done**. The result is stored
   as data per environment - there is no pass/fail verdict; interpretation and
   comparison happen later in the analysis views.

Logging events by hand today and receiving them from the camera system later use
the **same event types and the same validation** - the manual flow just mirrors
what the CV service will push.

Once a test is done, its summary metrics and any artifact links are kept by
Mişko. The raw video and frame-by-frame data stay in the separate camera
service.

## The dashboard

The **Dashboard** is the home screen. It shows summary counts (how many
subjects, scenarios, tests) and the most recent tests, so you can see lab activity
at a glance.

## Where the heavy data lives

Mişko deliberately does not store raw video or computer-vision telemetry. That
data belongs to an independent camera service. Mişko stores only the **summary
metrics and artifact URLs** for each test. The contract between the two systems
is described in [Integration](../integration/).
