# Changelog

## [v1.2.1] - 12 March 2026

* Add `--background` option for deamon startup:
    - scheduled task now re-starts as detached process and the task stops
    - console window is now hidden
    - fixes child processes killed when hotkeys exits

* Improve status output to indicate hotkeys process ID

## [v1.2.0] - 3 March 2026

* Removed Windows service installation for the following reasons:
    - does not make sense for interactive session / logged‑in user
    - security concerns
    - complexity

* Install and run as scheduled task (at user logon)

## [v1.1.0] - 2 February 2026

Install and run as service

## [v1.0.0] - 31 January 2026

First version
