# Security Policy

The security of Switaculous and of the people who use it is taken seriously. Thank you for
taking the time to report a problem responsibly.

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues, discussions or
pull requests.** Public reports put users at risk before a fix is available.

Instead, email the details privately to **kevin@kevincoder.co.za** with the subject line
"Switaculous security report". Where possible, include:

- a description of the issue and its potential impact
- the steps to reproduce it, or a proof of concept
- the Switaculous version and the operating system you used
- any suggestions you have for a fix

## What to expect

- Your report will be acknowledged as soon as possible, and you will be kept informed while
  the issue is investigated and fixed.
- Please allow reasonable time for a fix to be released before disclosing the issue publicly.
  The disclosure date can be agreed with you.
- With your permission, you will be credited in the release notes for the fix.

## Supported versions

Security fixes are made for the latest release. Please check that the issue still occurs with
the most recent version before reporting it.

## Scope

Switaculous runs a small web server on your own computer. Reports of particular interest
include:

- a web page or another device being able to read, change or delete files on the computer
  running Switaculous
- a device on the same network getting past the QR code access check
- the MCP endpoint (`/mcp`) being reachable from anything other than the same computer
- Swift code opened in Switaculous being able to affect the computer outside the project folder
