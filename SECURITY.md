# Security Policy

## Supported versions

Only the latest release receives security fixes.

## Reporting a vulnerability

Report vulnerabilities privately using GitHub's private vulnerability reporting: open the
repository's **Security** tab and choose **Report a vulnerability**. Do not open a public
issue for a security problem.

Include the affected version, steps to reproduce, and the impact you expect.

## Scope

Acton runs workflows with the same filesystem, Docker, and network access as the desktop
app. The optional local MCP server (`--mcp`) binds to `127.0.0.1` by default, accepts
numeric loopback addresses only, and rejects requests from other browser origins. An
attached local agent can run workflow commands with the app's privileges, which is by
design. Reports about bypassing the loopback or origin restrictions are in scope.
