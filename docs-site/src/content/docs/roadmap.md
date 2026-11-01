---
title: Roadmap
description: What Mişko does today and what comes next.
---

Mişko is a **video-only** behavioral test platform: every measurement comes from
a recording, its calibration and a paradigm version defined in code. There are
no sensors in the plan. Open work is tracked in
[GitHub issues](https://github.com/HappyHackingSpace/Misko/issues).

## Done

- **Laboratory and access**: one laboratory per installation, the superadmin
  created by the setup command, five roles with a permission matrix, user
  management.
- **Catalog**: substances, disease models, and four paradigms (Morris water
  maze, novel object, pole test, treadmill) in published versions that never
  change, with parameters, zones, metrics, events and quality rules in English
  and Turkish.
- **Environments**: apparatus of one paradigm with measurement revisions and
  calibration per arena.
- **Study design**: experiments with groups and roles, regimens, timepoints,
  protocols and endpoints, start rules checked before day 0.
- **Execution**: allocation and exclusion, daily dosing and induction
  outcomes, test sessions with one video and several animals in their arenas,
  trials and comments.
- **Analysis**: jobs claimed by workers, trajectories verified and checked by
  quality rules, every metric and event computed by the Go engine; paradigm
  trials outside any study.
- **Results**: human review of every result, the study report with the
  comparisons that follow from group roles, CSV and Excel export, the
  dashboard and a global search.

## Next

- **Validate the tracking on real recordings.** The bundled worker has only
  been tested on synthetic video. Its numbers must be compared with blind
  manual measurements of real sessions, and the error a still animal adds to
  distance and immobility (tracking jitter) must be measured, before results
  are used for research.
- **Change measurements only through new versions.** If the validation calls
  for smoothing or other changes, they ship as a new paradigm version, so
  results already published keep the definition they were computed with.
