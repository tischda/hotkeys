[![Build Status](https://github.com/tischda/hotkeys/actions/workflows/build.yml/badge.svg)](https://github.com/tischda/hotkeys/actions/workflows/build.yml)
[![Test Status](https://github.com/tischda/hotkeys/actions/workflows/test.yml/badge.svg)](https://github.com/tischda/hotkeys/actions/workflows/test.yml)
[![Coverage Status](https://coveralls.io/repos/tischda/hotkeys/badge.svg)](https://coveralls.io/r/tischda/hotkeys)
[![Linter Status](https://github.com/tischda/hotkeys/actions/workflows/linter.yml/badge.svg)](https://github.com/tischda/hotkeys/actions/workflows/linter.yml)
[![License](https://img.shields.io/github/license/tischda/hotkeys)](/LICENSE)
[![Release](https://img.shields.io/github/release/tischda/hotkeys.svg)](https://github.com/tischda/hotkeys/releases/latest)

# hotkeys

Starts a hotkey daemon that binds hotkeys such as `CTRL+A` to an action. The bindings
are defined a TOML config file (hot-reload supported).

The deamon can run as a console application or be installed as a Windows
service using the 'install' command.

The processes executed by the daemon will inherit the current environment and update
USER and SYSTEM environment variables from the Windows registry.

## Install

~~~
go install github.com/tischda/hotkeys@latest
~~~

Install and run as service:
~~~
hotkeys install --log=%TEMP%\hotkeys-service.log
sc start hotkeys
sc query hotkeys
~~~

## Usage

~~~
Usage: hotkeys [COMMANDS] [OPTIONS]

COMMANDS:

  install    installs the application as a Windows service
  remove     removes the Windows service

OPTIONS:

  -c, --config path
        specify config file path (default '%USERPROFILE%\.config\hotkeys.toml')
  -l, --log path
        specify log output path (default stdout)
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

* When starting alacritty without `cmd /c`, all child terminals launched by the
  daemon are killed when the console version of the daemon is stopped (not when
  run as a service). I could not reproduce this with notepad.exe.

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
