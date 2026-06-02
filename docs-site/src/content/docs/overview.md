---
title: Overview
description: What Mişko does, who it is for, and the core concepts.
---

Mişko manages behavioral tests run on laboratory mice from a single place.
Studies, subjects, apparatuses, devices and tests are all defined and tracked
through the application.

## Who it is for

Biology and neuroscience laboratories that run behavioral paradigms on rodents
and need a reliable record of **what was tested, on which animal, with which
setup, and what the result was** — without managing raw video by hand.

## Core concepts

- **Paradigm** — a scientific test type (Morris Water Maze, Open Field, Elevated
  Plus Maze, Rotarod). Defined in code; the catalog is fixed and stable.
- **Apparatus** — a concrete physical rig instantiating a paradigm (geometry in
  cm, surface color/material for vision contrast, zones).
- **Subject** — the mouse, with a research-grade biological profile (strain,
  line/genotype, zygosity, sex, coat color) and a weight-log time series.
- **Study → Group** — longitudinal experiments with comparison arms (control,
  model, treated) so repeated tests of the same animal are comparable.
- **Test** — the central transaction: paradigm + apparatus + subject + operator
  + device, moving through `PENDING → RUNNING → DONE / FAILED`.

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
