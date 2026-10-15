---
title: Overview
description: What Mişko does, who it is for, and the core concepts.
---

Mişko manages preclinical behavioral studies on laboratory rodents from a single
place: the study design, the animals and what they received, the filmed test
sessions, the video analysis and the reviewed results.

## Who it is for

Biology and neuroscience laboratories that test whether a substance reverses a
deficit in a disease model with behavioral paradigms, and need a reliable record
of **which animal was in which group, what it received, how it was tested, and
what the result was**, without managing raw video by hand.

## Core concepts

- **Catalog**: substances (compounds, reference drugs, vehicles, inducing
  agents) and disease models, shared by every study of the laboratory.
- **Paradigm**: a scientific test type (Morris Water Maze, Open Field, Elevated
  Plus Maze, Rotarod and others). Defined in code in published versions that
  never change.
- **Environment**: a physical apparatus of one paradigm, with measurement
  revisions. In a test, an animal is placed in an arena, which is one
  environment revision.
- **Subject**: the animal (code, species, sex) and its body weights.
- **Experiment**: the study. It is designed as a draft (groups, regimens,
  timepoints, protocols, endpoints), started on day 0 and completed.
- **Group**: an arm of the study with a role (healthy control, sham, disease
  control, positive control or treatment), a disease model and a regimen. An
  animal is allocated to exactly one group, and can be excluded with a reason.
- **Test**: one filmed session at one timepoint on one protocol step, with one
  video and one or more animals, each in its own arena.
- **Result**: the analysis of one animal in one test. Every result is accepted
  or rejected by a person before it counts. Results are data, not a pass or fail
  verdict.
- **Report**: per endpoint, the group statistics and the comparisons that follow
  from the group roles (model validity, assay sensitivity, efficacy with
  rescue %, dose response).

## Internal SaaS model

Mişko is deployed **on-prem, one laboratory per installation**. There is no
public sign-up: a **superadmin is created by the setup command** (the system
generates a strong password and prints it once), and all other users are created
from inside by an administrator. Access is governed by **role-based
permissions** tied to each person's role in the lab.

## The system boundary

The API does not run computer vision itself. Separate analysis workers claim
one animal of a test at a time, track it in the video and upload a trajectory
and an annotated video to object storage. The API verifies the uploads and
computes every metric from the trajectory, so each measurement has one
definition. Video files stay in object storage; the database holds records,
metrics, events and references to objects. See [Architecture](../architecture/).
