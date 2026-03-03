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

## Usage

~~~
Usage: hotkeys [COMMAND] [OPTIONS]

COMMANDS:

  install [--force]  creates/updates a Task Scheduler logon entry
  remove             removes the Task Scheduler logon entry
  status             shows Task Scheduler state (scheduled/running)

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

## Example

Run with file logging:
~~~
hotkeys --log=%TEMP%\hotkeys-console.log
~~~

This will run interactively, but requires to leave this console window open.

## Auto-start

After a first version using Windows services, I came to the conclusion that a better option
to start hotkeys with Windows is to setup a scheduled task named 'Hotkeys'.

As an administrator, install a logon task (do not move `hotkeys.exe` after this):
~~~
sudo hotkeys install --config=%USERPROFILE%\.config\hotkeys.toml --log=%TEMP%\hotkeys-task.log --force
~~~

Remove task:
~~~
sudo hotkeys remove
~~~

Show task status:
~~~
hotkeys status
~~~

I am using gsudo here (`winget install -e --Id gerardog.gsudo`).

## Start / Stop

Once the scheduled task is configured, you can control is easily.

Start:
~~~
schtasks /Run /TN "Hotkeys"
~~~

Stop:
~~~
schtasks /End /TN "Hotkeys"
~~~

Verify:
~~~
schtasks /Query /TN "Hotkeys" /V /FO LIST
~~~

## Configuration

By default, the hotkeys configuration file is loaded from: `%USERPROFILE%\.config\hotkeys.toml`.

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
      are killed when the daemon is stopped (could not reproduce it with `notepad.exe`).

* Some strange behaviour for console applications, eg. `action = [ "wait.exe", "20" ]`,
  nothing seems to happen, but the process is actually running:

~~~
❯ tasklist /FI "IMAGENAME eq wait.exe"

Image Name                     PID Session Name        Session#    Mem Usage
========================= ======== ================ =========== ============
wait.exe                     21452 Console                    1      6,620 K
~~~

Workaround:

~~~
action = [ "cmd", "/c", "wait.exe", "20" ]
~~~

* When hotkeys is already running in a console, it won't start again as a scheduled
  task. This is by design, since you cannot register keybindings multiple times.
  But keep this in mind when troubleshooting task startup. Here is an example:

~~~
❯ schtasks /Run /TN "Hotkeys"
SUCCESS: Attempted to run the scheduled task "Hotkeys".

❯ hotkeys status
Task 'Hotkeys' scheduled=yes status=Ready
~~~

Here the ATTEMPT was successful, but the RESULT is a failure! The status is 'Ready'
and not 'Running'. Let's see if another hotkeys process is live:

~~~
❯ tasklist /FI "IMAGENAME eq hotkeys.exe"

Image Name                     PID Session Name        Session#    Mem Usage
========================= ======== ================ =========== ============
hotkeys.exe                  11352 Console                    1      4,204 K

❯ taskkill /f /im hotkeys.exe
SUCCESS: The process "hotkeys.exe" with PID 11352 has been terminated.
~~~

Now we cleaned up the process and we can start the scheduled task.
