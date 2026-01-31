[![Build Status](https://github.com/tischda/hotkeys/actions/workflows/build.yml/badge.svg)](https://github.com/tischda/hotkeys/actions/workflows/build.yml)
[![Test Status](https://github.com/tischda/hotkeys/actions/workflows/test.yml/badge.svg)](https://github.com/tischda/hotkeys/actions/workflows/test.yml)
[![Coverage Status](https://coveralls.io/repos/tischda/hotkeys/badge.svg)](https://coveralls.io/r/tischda/hotkeys)
[![Linter Status](https://github.com/tischda/hotkeys/actions/workflows/linter.yml/badge.svg)](https://github.com/tischda/hotkeys/actions/workflows/linter.yml)
[![License](https://img.shields.io/github/license/tischda/hotkeys)](/LICENSE)
[![Release](https://img.shields.io/github/release/tischda/hotkeys.svg)](https://github.com/tischda/hotkeys/releases/latest)

# hotkeys

Starts a hotkey daemon that binds hotkeys such as `CTRL+A` to an action. The bindings
are defined in a TOML config file (hot-reload supported).

The processes executed by the daemon will inherit the current environment and update
USER and SYSTEM environment variables from the Windows registry.

## Install

~~~
go install github.com/tischda/hotkeys@latest
~~~

## Usage

~~~
Usage: hotkeys [OPTIONS]

OPTIONS:

  -f, --file path
        specify config file path (default '%USERPROFILE%\.config\hotkeys.toml')
  -?, --help
        display this help message
  -v, --version
        print version and exit
~~~

## Configuration file

The configuration file is in TOML format, for example:

~~~
[keybindings]
bindings = [
  { modifiers = "alt", key = "d", action = [ "detach.exe", 'C:\\Program Files\\Alacritty\\alacritty.exe' ] },
  { modifiers = "alt", key = "a", action = [ 'C:\\Program Files\\Alacritty\\alacritty.exe' ] },
  { modifiers = "ctrl+alt", key = "n", action = [ "notepad.exe" ] },
]
~~~

In `action`, use single quotes to avoid issues with backslashes in file paths.

## Setup

By default, this file is expected to be here: `%USERPROFILE%\\.config\\hotkeys.toml`.

You can override this by setting the `HOTKEYS_CONFIG_HOME` environment variable,
or by specifying the `--file` option via the commande line.

The configuration is hot-reloaded on every change.

## Known issues

* When starting alacritty without `detach`, all child terminals launched by the daemon are
  killed when the daemon is stopped. I could not reproduce this with notepad.exe for example.

* Some strange behaviour for console applications, eg. `action = [ "wait.exe", "20" ]`,
  nothing seems to happen, but you can check that the process:

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