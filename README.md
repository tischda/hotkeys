[![Build Status](https://github.com/tischda/hotkeys/actions/workflows/build.yml/badge.svg)](https://github.com/tischda/hotkeys/actions/workflows/build.yml)
[![Test Status](https://github.com/tischda/hotkeys/actions/workflows/test.yml/badge.svg)](https://github.com/tischda/hotkeys/actions/workflows/test.yml)
[![Coverage Status](https://coveralls.io/repos/tischda/hotkeys/badge.svg)](https://coveralls.io/r/tischda/hotkeys)
[![Linter Status](https://github.com/tischda/hotkeys/actions/workflows/linter.yml/badge.svg)](https://github.com/tischda/hotkeys/actions/workflows/linter.yml)
[![License](https://img.shields.io/github/license/tischda/hotkeys)](/LICENSE)
[![Release](https://img.shields.io/github/release/tischda/hotkeys.svg)](https://github.com/tischda/hotkeys/releases/latest)

# hotkeys

Starts a hotkey daemon that binds hotkeys such as `CTRL+A` to an action. The bindings
are defined in a TOML config file (hot-reload supported).

The daemon should be run in user mode as a background process.

The processes executed by the daemon will inherit the current environment and update
USER and SYSTEM environment variables from the Windows registry.

## Install

~~~
go install github.com/tischda/hotkeys@latest
~~~

Run with file logging:
~~~
hotkeys --log=%TEMP%\hotkeys.log
~~~

Install logon task (`At logon` + `Run only when user is logged on`):
~~~
hotkeys install --config=%USERPROFILE%\.config\hotkeys.toml --log=%TEMP%\hotkeys.log --force
~~~

Remove logon task:
~~~
hotkeys remove
~~~

Show task status:
~~~
hotkeys status
~~~

## Usage

~~~
Usage: hotkeys [COMMAND] [OPTIONS]

COMMANDS:

      install   creates/updates a Task Scheduler logon entry
      remove    removes the Task Scheduler logon entry
      status    shows Task Scheduler state (scheduled/running)

OPTIONS:

  -c, --config path
        specify config file path (default '%USERPROFILE%\.config\hotkeys.toml')
  -l, --log path
        specify log output file path (default stdout)
  -?, --help
        display this help message
  -v, --version
        print version and exit
~~~

## Configuration

By default, the configuration file is loaded from: `%USERPROFILE%\.config\hotkeys.toml`.

You can override the path for `hotkeys.toml` by setting the `HOTKEYS_CONFIG_HOME`
environment variable, or by specifying the full path with `--config`.

The configuration is hot-reloaded on every change.

## Keybindings file

The configuration file is in TOML format, for example:

~~~
[keybindings]
bindings = [
    { modifiers = "alt", key = "enter", action = [
        'C:\Program Files\Alacritty\alacritty.exe',
    ] },
    { modifiers = "alt", key = "c", action = [
        "cmd",
        "/c",
        'C:\Program Files\Alacritty\alacritty.exe',
    ] },
]
~~~

In `action`, use single quotes to avoid issues with backslashes in file paths.

## Known issues

* When starting alacritty without `cmd /c`, all child terminals launched by hotkeys
      are killed when the daemon is stopped (could not reproduce with notepad.exe).

* Some strange behaviour for console applications, eg. `action = [ "wait.exe", "20" ]`,
  nothing seems to happen, but the process is actually running:

~~~
tasklist /FI "IMAGENAME eq wait.exe"

Image Name                     PID Session Name        Session#    Mem Usage
========================= ======== ================ =========== ============
wait.exe                     21452 Console                    1      6,620 K
~~~

Workaround:

~~~
action = [ "cmd", "/c", "wait.exe", "20" ]
~~~
