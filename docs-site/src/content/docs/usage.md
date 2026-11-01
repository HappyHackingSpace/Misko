---
title: Usage
description: How to use the Mişko panel day to day, from substances, disease models and study design to dosing, test sessions, result review and reports.
---

This page is the practical guide to using Mişko once it is installed. It is
written for everyone in the lab, not only developers. If you have not installed
Mişko yet, start with [Installation](../installation/).

## The big picture

Mişko is the logbook of a preclinical behaviour study. A study asks one
question: does a substance at a given dose reverse the deficit caused by a
disease model? Everything in the panel follows that question:

1. **Substances** and **Disease models** list what the lab doses and the
   deficits it induces; each has its own page in the menu.
2. **Subjects** are the animals, with their body weights.
3. **Environments** are the arenas, each calibrated once for its fixed camera.
4. An **experiment** is designed as groups (each with a role, a disease model and
   what it receives), timepoints, protocols and endpoints, then started.
5. Animals are **allocated** to groups, **dosed** every day and filmed in
   **test sessions**.
6. Every animal is measured automatically on the session video: an analysis
   worker tracks it in every paradigm. A person **reviews** each result, and only accepted results enter the
   **report**.
7. **Paradigm trials** try a paradigm on a single video outside any study,
   for example to check a camera or the analysis thresholds.

The panel is in Turkish by default. Use the language switch in the top bar to
change it to English; your choice is remembered.

## First login

There is no public sign-up. After installation a single **superadmin** account
exists, and its password was printed once to the backend log (see
[Installation](../installation/#get-the-first-login-password)).

1. Open the panel (default [http://localhost:8080](http://localhost:8080)).
2. Log in with `ADMIN_EMAIL` and the generated password.
3. Create the rest of the team from the **Users** screen.

## Roles

There are five roles. The API decides what each role may do, and the panel
hides buttons the API would refuse.

| Role | What it can do |
|---|---|
| `SUPERADMIN`, `LAB_MANAGER` | Everything, including users and lab settings. |
| `RESEARCHER` | Everything except users and lab settings: substances, disease models, subjects, environments, study design, planning sessions, dosing, running sessions, review. |
| `TECHNICIAN` | The lab day: record doses and weights, start and complete sessions, record trials, upload videos, calibrate an animal on a video, run paradigm trials, review results. Cannot plan or cancel sessions or change a study. |
| `VIEWER` | Reads everything, changes nothing. |

When you create a user on the **Users** screen you may leave the password blank
and let the system generate a strong one. It is shown only once, so copy it
before closing.

## Common controls

Lists work the same everywhere: a search box, filters, sortable columns and
paging at the bottom. New records and edits open in a side drawer or a dialog,
a green message confirms a save, and an error is shown in words next to the
form. Deleting always asks for confirmation. Many screens keep their filters in
the address bar, so you can bookmark or share a filtered list.

**Search** in the header (or Ctrl+K, Cmd+K on a Mac) finds studies, their
reports, test sessions and subjects from any page. Type a study code or title,
a timepoint or a subject code; arrow keys and Enter open a result, Esc closes.

## Substances

Open **Substances** in the menu (address `/substances`). Substances are
compounds, reference drugs, vehicles (saline, CMC, DMSO) and disease inducers
(MPTP, LPS). The list can be searched by name and filtered by kind. To add one:

1. Click **New substance**.
2. Type the name, choose the kind and optionally a description.
3. Click **Save**.

Use **Edit** on a row to change it and **Delete** to remove it; deleting asks
for confirmation. The kind only sorts the pickers; the role of a substance in a
study comes from where it is used. Names are unique: a second "Saline" (in any
letter case) is refused. A substance used by a regimen or a recorded dose cannot
be deleted; the panel says so.

## Disease models

Open **Disease models** in the menu (address `/disease-models`). A disease
model describes how a deficit is produced (chemical, surgical, diet, stress or
other) and how it is confirmed. The list can be searched by name and filtered by
induction type.

1. Click **New disease model**.
2. Type the name, choose the induction type, and optionally a confirmation
   criterion (for example "pole descent time above 150% of baseline") and a
   description.
3. Click **Save**.

Names are unique, as for substances. A model used by an experiment group cannot
be deleted; the panel says so.

When a group or regimen form in an experiment finds no disease model or no
substance yet, it links to the matching page.

## Subjects and weights

Open **Subjects**. Each animal has a code (letters, digits, `.`, `_`, `/`,
`-`), a species (mouse or rat) and a sex.

1. Click **New subject**, type the code, choose species and sex, and save.
2. Use the search box and the species and sex filters to find animals. Tick
   **Only subjects not in a group** to see animals that belong to no group yet.
3. Click a code to open the animal's page. It shows its study and group (or that
   it is unallocated) and its body weights, newest first, with the change from
   the weighing before.
4. To weigh an animal, type the grams and the time on its page and click
   **Record weight**. Weights are never edited afterwards.

Codes are unique. An animal that is in a group or has weights cannot be deleted.

## Environments and calibration

The **Paradigms** page lists the tests Mişko knows, each with a short
description of what it measures.

An environment is a physical arena built from a paradigm (Morris water maze,
novel object box, pole, treadmill). Open **Environments** and click **New
environment**:

1. Type a name (for example "Box A").
2. Choose the paradigm. The form then shows the measurements that paradigm
   needs, filled with typical values.
3. Enter the real measurements (for example the arena width and height in cm).
   Analysis settings are folded away and rarely need changing.
4. Click **Save**. The environment's page opens.

**Default calibration.** A camera that does not move is calibrated once per
arena, and every video filmed in that arena uses it. Calibration tells Mişko how
camera pixels map to centimetres on the apparatus, and, for a paradigm that
declares drawn zones, where those zones are. Every paradigm is calibrated,
and the calibrator shows a short guide for the paradigm: where the camera goes
and what to draw.

| Paradigm | Camera | Apparatus frame | Zones |
|---|---|---|---|
| Morris water maze | Above the tank | Centre of the tank, x east, y north | none drawn |
| Novel object | Above the arena | Origin corner of the floor, x along the width, y along the depth | **Novel object** and **familiar object**: a circle around each object, about 2 cm wider than it |
| Pole | At the side of the pole | x the distance from the pole, y the height above the base (base 0, top the pole length) | **Pole** (where the animal climbs) and **base** (the floor at its foot), polygons |
| Treadmill | At the side of the lane | x along the lane from the shock grid end (0) to the front, y up from the belt | **Shock grid** at the back of the lane, a polygon |

1. On the environment's page click **Calibrate this revision**. The visual
   calibrator opens.
2. Under **Video for the frame** click **Choose a video file** and pick a video
   filmed with this camera. The file is only read in your browser to show a
   frame; it is never uploaded. The frame size is read from the video. Use the
   scrubber and the frame step buttons to pick a frame where the apparatus is
   clearly visible.
3. Type the **Camera** name and check the **Measurement plane** (arena floor,
   or water surface for the water maze).
4. With the **Fit points** tool, click each fit point on the frame, then drag it
   to adjust. The highlighted row is the point the next click places; click a
   row to choose another one. **Use the apparatus points** fills in the real
   positions from the environment's measurements, so you only click them in
   turn:
   - water maze: the north, east, south and west points of the tank wall,
     measured from the tank centre (x grows east, y north).
   - novel object box: the four inside corners of the floor, from the origin
     corner (0, 0) along the width and the depth.
   - pole: the base (height 0) and the top (the pole length) of the pole. Both
     lie on one vertical line, so two more marks of the rig away from the pole
     are needed (for example on the back panel): measure their distance from
     the pole and their height above the base and type them. The calibrator
     says so when the fit points lie on one line.
   - treadmill: the corners of the lane wall, at the grid end (0) and the front
     (the lane length), on the belt and at the top of the wall. Type the
     **Lane wall height** to fill in the upper points.

   Without a preset, type the real position in cm of every point you click, in
   any frame you like (for example centred on the apparatus).
5. With the **Check points** tool, place at least three check points at
   places other than the fit points (the presets use the middles of the sides,
   the diagonal wall points of the tank, the middle of the pole with two more
   rig marks, or the middle of the lane).
6. With the **Crop** tool, drag a rectangle around this arena. Points and zones
   must lie inside it. **Whole frame** resets it.
7. If the paradigm has drawn zones (novel object, pole, treadmill), open the
   **Zones** tool. Every zone is listed as **Required** or **Optional**. Click
   **Draw circle** or **Draw polygon** next to a zone (the object zones offer
   both, circle first; the pole, base and shock grid zones are polygons) and
   draw it on the frame:
   - a polygon: click each corner, then click the first corner again (or
     **Close polygon**) to close it; **Undo last corner** removes the last one;
   - a circle: press at the centre and drag to the edge.

   A zone may need several shapes; draw them one after another under the same
   zone. While drawing, a click on a corner or centre of a shape
   already drawn snaps to it, so neighbouring zones share their edges exactly.
   Drag the handles of a finished shape to adjust it, or use **Redraw** and
   **Delete**. **Stop drawing** ends the current zone. For the novel object,
   **Swap Novel object and Familiar object** exchanges the two drawn zones, for
   example in one animal's calibration when its novel object stands on the
   other side (counterbalancing).
8. Every value can also be typed under **Enter numbers by hand**. The list
   under the frame says what is still missing.
9. Click **Save the calibration**. The panel shows the fit and check errors,
   the tolerance and each zone in pixels and in cm. A calibration whose check
   error is above the tolerance is rejected; save it again with better points.

Analyses in this arena wait until a valid calibration with every required zone
exists. A saved calibration never changes: **Correct calibration** saves a new
one that replaces it, and earlier calibrations are kept.

**Revisions.** Measurements are never edited in place, so every test keeps the
values it was run with. Click **Change the measurements** to create a new
revision. A new revision needs its own calibration.

## Designing a study

Open **Experiments** and click **New experiment**. Fill in the code, the title,
the hypothesis, how animals are randomised and whether the study is blinded,
then save. The experiment opens on its **Groups and regimens** tab. A new study
is a **Draft**; its design can be changed until it starts.

**Groups and regimens.**

1. Click **Add group**. Give a code (G1, G2), a name, a role and a target number
   of animals. Every role except healthy control needs a disease model.
2. On the group's card click **Add item** to say what the group receives: the
   purpose (induction, vehicle or treatment), the substance, the dose and unit
   (for example 10 mg/kg), the route, how often (once, daily, twice daily,
   weekly or every N days) and the first and last study day. The purpose list
   only offers what the group's role may receive; when only one purpose fits it
   is already chosen. A short line under **Regimen** on every card says what
   the role must and must not receive. A healthy control receives no
   substance, so its **Add item** asks only for the **First study day** and
   the **Last study day** of the period in which it receives nothing; see
   below.

The roles are: healthy control, sham, disease control, positive control and
treatment. **Every study needs at least one healthy control group**: model
validity and rescue % are measured against it, so a study without one cannot
start. What each role may receive:

| Role | Must receive | May also receive | Never receives |
| --- | --- | --- | --- |
| Healthy control | no substance (only its period is recorded) | | vehicle, induction, treatment |
| Sham | a vehicle | | induction, treatment |
| Disease control | the induction and a vehicle | | treatment |
| Positive control | the induction and one treatment (the reference drug) | a vehicle | a second treatment |
| Treatment | the induction and one treatment (its dose level) | a vehicle | a second treatment |

The panel and the API refuse an item the role never receives ("This group's
role cannot receive an item with this purpose"), and a second treatment item of
a group.

**Healthy control: no substance, only the period.** A healthy control is a
*naive* control and receives no substance at all: no vehicle, no induction and
no treatment. Its card says "No substance is given to this group (naive
control)". To record when the group goes without any substance, click **Add
item** on its card and enter only the **First study day** and the **Last study
day**; there is no purpose, substance, dose, unit, route or frequency. The item
reads "No substance: day 0 to 14", and the Dosing tab lists the group with
"No substance" and that period. The period is optional; an empty healthy
control is still valid. A vehicle a healthy control received in an older
draft blocks the start ("A healthy control group has a vehicle item it must
not have"): edit it and save the two days to turn it into a period, or delete
it. Studies that already started keep their design unchanged.

**Timepoints and protocols.** On the **Timepoints and protocols** tab:

1. Click **Add timepoint** for each moment animals are tested, with a name
   (Baseline, Day 7) and a study day (negative before day 0).
2. Click **New protocol**, name it and add one step per test: the paradigm, the
   trial type and the number of trials. Paradigms with session settings (for
   example treadmill belt speeds) show them on the step.
3. To change a protocol later, click **New version**. Earlier versions never
   change, so sessions keep the version they were planned with.

**Endpoints.** On the **Endpoints** tab, click **Add endpoint** to add what the study is judged on: a
metric of a paradigm in the protocols, at a timepoint, optionally compared with
a baseline timepoint, and which direction is better. Exactly one endpoint is
**primary**; a second primary is refused.

**Starting.** The **Overview** tab lists every start rule that is not met yet,
each with a link to the tab that fixes it. When the healthy control is
missing, **Add a healthy control group** opens the group dialog with that role
already chosen. When the list says the study is
ready, click **Start study**, choose day 0 (the date study days count from,
usually the induction) and confirm. The study becomes **Running**: groups,
regimens, timepoints and endpoints are now shown read-only. New protocol
versions are still possible.

## Allocation and exclusion

On the **Subjects** tab:

1. Click **Allocate subjects**. The drawer lists animals that belong to no group.
2. Search and tick the animals.
3. Either choose **One group** and the group, or **Randomise** to let the
   system spread them over the groups up to their targets (optionally only over
   some groups). A cage can be added.
4. Click **Allocate** or **Randomise**.

The group cards at the top show how full each group is. An animal belongs to
exactly one group and, once the study runs, never changes group. If an animal
dies or must leave the study, click **Exclude**, choose the reason (died, humane
endpoint, induction failed, technical problem, other) and add a note. Excluded
animals stay on the list with their reason and are left out of statistics.

## Daily dosing

The **Dosing** tab shows the doses due on a day, grouped by group and animal,
with each animal's latest weight. Use the arrows or the date picker to move to
another day and **Today** to come back. A healthy control is never due a dose:
it is listed below with **No substance** and its period (for example "No
substance: day 0 to 14"), and any other group without a regimen item with **No
dosing**. A dose is **Due** today, **Given** once
recorded, **Missing** on a past day and **Planned** on a future day.

1. Click **Record** next to a due dose. The dialog shows the planned dose;
   change the time, amount or route only if what was given differs.
2. A dose per kilogram needs a weight from the last seven days. If there is
   none, the dialog asks for the weight first, records it, and then records the
   dose with it. The list shows the dose in mg next to the planned mg/kg.
3. For a dose that is not in the plan, click **Unplanned dose** and choose the
   animal, the purpose, the substance, the amount, unit and route.

**Induction outcomes.** Animals that received an induction are listed under
**Induction outcomes**. Click **Record outcome**, choose confirmed or not
confirmed and save. An outcome is recorded once.

The **Dose history** at the bottom lists every recorded dose and can be
filtered by animal and purpose.

## Planning and running test sessions

A test session is one timepoint, one protocol step and **one video**, with one
or more animals, each in its own arena (for example three novel object boxes
filmed by one camera).

**Planning** (researchers and above), on the experiment's **Tests** tab:

1. Click **Plan a session**.
2. Choose the timepoint; the protocol, version and step are preselected.
3. The time defaults to day 0 plus the timepoint's study day.
4. Choose the first animal and its arena, then **Add subject** for each further
   animal. Only animals that are not excluded and not already planned at the
   same timepoint and step are offered, and only arenas of the step's paradigm.
5. Click **Plan**.

While a session is planned, **Edit subjects** on its page adds or removes
animals. **Cancel session** asks for a reason, which is kept on the session.

**The worklist.** The **Tests** menu lists sessions across studies in four
views: **Today**, **Overdue** (planned or still running from an earlier day),
**Upcoming** and **All**, with filters for experiment, status and paradigm.
The view and filters are kept in the address.

**Running** (technicians and above), on the session's page:

1. Click **Start**. A session with an excluded animal cannot start until that
   animal is removed.
2. For each animal, click **Record trial** in its card, check the repetition and
   times and save. The card counts the trials done against the planned number.
3. Upload the session video with **Upload video**. It is checked after upload
   and then shown as the video in use. **Replace video** uploads a new one after
   a confirmation; earlier videos stay listed under **Earlier videos**.
4. Each animal's card shows which calibration its part of the frame uses:
   normally the **Arena default**. If the camera or an arena moved, click
   **Calibrate on this video** on that animal. The same calibrator opens on a
   frame of the session's own video, starting from the arena's points and
   zones: move the points and zones to where they are in this video and save.
   The frame you used is kept with the calibration. It applies to this animal
   on this video only; the other animals keep their arena default. If it is
   rejected, the animal waits for a new one instead of falling back to the
   arena default.
5. Click **Complete** when every animal has at least one trial; until then the
   button stays disabled and the page says why.

Comments at the bottom of the page let the lab note anything unusual. Anyone
can comment; you can edit your own, and lab managers can delete any.

## Analysis and result review

Every paradigm is analysed automatically: an analysis worker follows the
animal on the video, and Mişko derives the events (exploration of each object,
the pole turn, arrival at the base and falls, treadmill shocks and exhaustion)
from its track and the calibrated zones. A person only reviews the result.

When a running session's video is verified and the
animal's calibration is valid, the analysis worker measures each animal in its
arena. The **Analysis** block of the session page shows the state of each
animal, and **Analyse again** queues a new run (for example after a new
calibration).

Every result must be approved by a person before it counts. Open **Result
review**:

1. The list shows results waiting for review, filtered by experiment and status
   (pending, accepted, rejected, all). The filters are kept in the address.
2. Click a row to open the analysed video, the quality checks and the metrics.
3. Click **Accept**, or choose a reason (tracking lost, wrong animal, video
   defect, protocol deviation, other), add a note and click **Reject**. The
   drawer then moves to the next pending result.
4. If an animal was analysed again, the checks say **A newer run is available**.
   Pick a run in the run list and click **Review this run instead**; the result
   goes back to pending with that run.
5. **Select clean rows** ticks every pending result with all metrics measured,
   no newer run and no exclusion; **Accept selected** accepts them together.

In a **blinded** study the review screen shows only the animal, the arena and
the session, never the group, dose or substance.

## Paradigm trials

**Paradigm trials** in the main menu try a paradigm on one video in one
arena, outside any experiment: there are no groups, no review and no report,
and a trial's result never moves into an experiment. Use it to check a new
camera position, an arena or the analysis thresholds before a study. Everyone
can read trials; starting and running one needs the `trial:run` permission
(every role except `VIEWER`).

1. Click **New trial**, give it a name, choose the paradigm and the
   environment revision to try it in, add notes if useful and click **Create
   trial**. The trial page opens.
2. Click **Upload video** and choose the recording. The file goes straight to
   storage and is verified; the player then shows it. **Replace video** uploads
   another one; earlier analyses keep the video they used.
3. The **Calibration** card says which calibration applies: the
   environment's, or the trial's own made on this video. If the environment is
   not calibrated or the camera moved, click **Calibrate on this video** and
   place the points and zones as on the environment page. This calibration
   belongs to the trial and this video only; the environment never changes.
4. Once the video is verified and a calibration applies, the analysis starts by
   itself. The **Analyses** card lists every analysis, newest first, with its
   state (queued, running, succeeded, failed with the reason), the tracking
   quality, the metrics in the panel's language and, under **Overlay video
   and events**, the video the worker drew and the events; clicking an event
   plays it. The page refreshes itself while an analysis is queued or running.
5. **Analyse again** opens the paradigm's analysis thresholds, filled with the
   environment's values. Change what you want to try and click **Analyse**: a
   new analysis of the same video is queued with only the changed values, and
   the earlier ones stay. **Compare** shows two succeeded analyses side by side
   with the difference of every metric.
6. **Delete** removes the trial from the list after a confirmation; its
   analyses stay stored.

## Results and reports

The experiment's **Results** tab, and the same report on the **Reports**
screen, show each endpoint at its timepoint:

- per group: n, mean, SD, SEM and the change from baseline, plus how many
  results are pending, waiting for analysis, rejected, missing or excluded;
- a chart of the group means;
- the comparisons in words: model validity, assay sensitivity, efficacy with
  rescue %, dose response and the time course.

Only accepted results of animals that are not excluded count. A banner on top
says whether the numbers are **provisional** (some results are still pending,
with a link to review them), **interim** (all reviewed, study still running) or
**final** (study completed). **Export** saves the report as CSV or Excel, one
row per animal or one row per group. A blinded study's report carries a
reminder before it is shared.

To finish a study, click **Complete study** on the **Overview** tab once every
planned session is completed or cancelled. A completed study is read-only.

## Dashboard

The **Dashboard** is the home screen. Its tiles count results (pending,
accepted, rejected), analysis (queued, running, failed, waiting for
calibration), sessions (planned, in progress, completed, cancelled) and
experiments (draft, running, completed). Click a tile to open the list it
counts, already filtered: for example **Pending** opens the review queue and
**In progress** opens the worklist of running sessions. Below the tiles are the
latest results and the upcoming sessions.

## Where the heavy data lives

Videos are uploaded straight to object storage; Mişko keeps their records,
calibrations, analysis runs, metrics and review decisions. The contract with the
analysis worker is described in [Integration](../integration/).
