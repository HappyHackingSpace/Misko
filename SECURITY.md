# Security policy

Mişko stores laboratory data and signs access to private video storage, so
security reports are taken seriously.

## Reporting a vulnerability

Please **do not open a public issue** for a vulnerability.

Report it privately through GitHub:
**Security tab of the repository > Report a vulnerability**
(<https://github.com/HappyHackingSpace/Misko/security/advisories/new>).

Include:

- what is affected (API route, worker route, panel page, deployment file),
- steps or a request that reproduces it,
- what an attacker could do,
- the commit or version you tested.

We aim to acknowledge a report within 5 working days and to tell you the plan
within 14 days. Mişko is maintained by volunteers, so these are goals, not
guarantees. Please give us reasonable time to fix the problem before you
disclose it.

## Supported versions

Mişko has no tagged stable release yet (see the roadmap). Only the latest
commit on `main` receives fixes.

## Scope

Examples of what we want to hear about:

- authentication or role bypass (the five roles and their permissions are
  described in [backend/README.md](backend/README.md)),
- one user reading or changing data they should not,
- worker token or signed URL exposure, including in logs,
- a worker delivering a result for a run it did not claim,
- injection, unsafe file handling or request forgery,
- secrets committed to the repository or built into an image.

Out of scope: findings that need a compromised administrator account or physical
access to the server, denial of service by sending very large volumes of traffic,
and issues in a third-party dependency with no way to reach it from Mişko
(report those upstream).

## For deployers

- Set a long random `JWT_SECRET` (`openssl rand -base64 48`).
- The administrator password is printed once by `bootstrap setup`; change it
  after the first sign-in.
- Keep the Cloud Storage bucket private and use workload identity or a service
  account with the narrowest roles. Do not commit service account files.
- Run the panel over HTTPS (the production compose file uses Caddy).

Dependencies are watched by Dependabot, and the backend runs `govulncheck` in
`make check`.
