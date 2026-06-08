---
title: Overview
description: What Mişko does, who it is for, and the core concepts.
---

Mişko manages behavioral tests run on laboratory mice from a single place.
Subjects, environments, scenarios and tests are all defined and tracked
through the application.

## Who it is for

Biology and neuroscience laboratories that run behavioral paradigms on rodents
and need a reliable record of **what was tested, on which animal, with which
setup, and what the result was** — without managing raw video by hand.

## Core concepts

- **Paradigm** — a scientific test type (Morris Water Maze, Open Field, Elevated
  Plus Maze, Rotarod). Defined in code; the catalog is fixed and stable.
- **Environment** — a named, concrete instance of a paradigm: the physical setup
  (geometry in cm, surface color/material for vision contrast, zones).
- **Subject** — the mouse, kept simple (code, sex, group, birth date, notes).
- **Scenario** — the central object. It bundles one or more environments (whose
  paradigms fix the metrics), so an experiment is defined once.
- **Test** — a subject measured against a scenario, run environment by environment
  (`PENDING → RUNNING → DONE / FAILED`). Results are data, not a pass/fail verdict.

## Internal SaaS model

Mişko is deployed **on-prem, one laboratory per installation**. There is no
public sign-up: a **superadmin is bootstrapped at startup** (the system
generates a strong password and prints it once), and all other users are created
from inside by an administrator. Access is governed by **role-based
permissions** tied to each person's role in the lab.

## The system boundary

Mişko deliberately does **not** do computer vision. It is the system of record
and keeps only **summary metrics + artifact URLs**. A separate CV service
(Python / FastAPI / YOLOv8 / ByteTrack, with its own PostgreSQL) owns the heavy
data and pushes a result summary back when a test ends. See
[Integration](../integration/).
