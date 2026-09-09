# Security policy

Mtile is in active development. Fixes target the current development branch;
older snapshots do not have a separate maintenance commitment.

## Report a vulnerability privately

Use **Security → Report a vulnerability** on the GitHub repository when
private vulnerability reporting is available. Include the affected revision,
platform, reproduction steps, impact, and a minimal project if possible.

If private reporting is unavailable, request a private contact channel in an
issue without posting the vulnerability details, exploit, or affected private
data. A maintainer should establish that channel before you share details.
Do not put sensitive reports into the ordinary bug-report form.

The project does not promise a response deadline or a bug bounty. Maintainers
will assess a report and coordinate fixes and disclosure with the reporter.

## Project files and generated ROMs

Mtile parses project assets, invokes native build tools, and runs ROMs through
an emulator helper. The editor is a development environment, not a security
sandbox for untrusted projects or cartridges. Parser crashes, unintended file
access, and code execution outside the intended build workflow are relevant
security reports; ordinary game/runtime bugs belong in the issue tracker.
