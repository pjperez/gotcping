# Security Policy

## Reporting a Vulnerability

gotcping is a small, dependency-light CLI tool. If you find a security issue,
please **do not** open a public GitHub issue.

Instead, report it privately:

- open a GitHub **Security Advisory** on this repository
  (`Security` → `Report a vulnerability`), or
- email the maintainer at **pjperez@users.noreply.github.com**.

Please include:

- a description of the issue and its impact,
- the `gotcping -version` you reproduced against,
- a minimal reproduction (command line + expected vs. actual behavior).

You should receive an acknowledgement within **5 business days**. Verified
issues will be fixed and a new release cut as soon as a patch is ready.

## Scope

gotcping opens outbound TCP connections to user-supplied host:port targets and
prints RTT statistics. Inputs (host, port, timeout, count, deadline, interval)
are validated before use, the resolved IP is dialed (no per-probe re-resolution),
and hostnames are sanitized before being written to output to prevent terminal
/log injection.

Issues that require controlling the command line a victim already runs are out
of scope, as are denial-of-service effects limited to the target the operator
deliberately probes (the tool's intended purpose).
