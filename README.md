[![Build Status](https://github.com/tischda/hotkeys/actions/workflows/build.yml/badge.svg)](https://github.com/tischda/hotkeys/actions/workflows/build.yml)
[![Test Status](https://github.com/tischda/hotkeys/actions/workflows/test.yml/badge.svg)](https://github.com/tischda/hotkeys/actions/workflows/test.yml)
[![Coverage Status](https://coveralls.io/repos/tischda/hotkeys/badge.svg)](https://coveralls.io/r/tischda/hotkeys)
[![Linter Status](https://github.com/tischda/hotkeys/actions/workflows/linter.yml/badge.svg)](https://github.com/tischda/hotkeys/actions/workflows/linter.yml)
[![License](https://img.shields.io/github/license/tischda/hotkeys.svg)](/LICENSE)
[![Release](https://img.shields.io/github/release/tischda/hotkeys.svg)](https://github.com/tischda/hotkeys/releases/latest)

# hotkeys

Hotkey daemon that binds hotkeys such as `ALT+ENTER` to an action. The bindings
are defined in a TOML config file (hot-reload supported).

The action processes executed by the daemon will obtain a refreshed environment
(updated USER and SYSTEM variables).

When running with `--background`, the process is re-executed in a detached state without
a console window. In that case, the parent process exits immediately, and the detached
child process will continue to run the server.


## Install

~~~
go install github.com/tischda/hotkeys@latest
~~~


## Usage

~~~
Usage: hotkeys [COMMAND] [OPTIONS]

Starts a hotkey daemon that binds hotkeys such as ALT+ENTER to an action.
The bindings are defined in a TOML config file (hot-reload supported).

COMMANDS:

  install [--force]  creates/updates a Task Scheduler entry
  remove             removes the Task Scheduler entry
  start              starts the scheduled task
  stop               stops the running hotkeys process
  status             shows the scheduled task and process states

OPTIONS:

  -c, --config path
        specify config file path (default '%USERPROFILE%\.config\hotkeys.toml')
  -l, --log path
        specify log output file path (default stdout)
  -b, --background
        start in background without a console window
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

This will run interactively, as long as the console window remains open.


## Auto-start

Install a scheduled task that runs at logon:

~~~
hotkeys install --config=%USERPROFILE%\.config\hotkeys.toml --log=%TEMP%\hotkeys-task.log --force
~~~

Remove task:
~~~
hotkeys remove
~~~


## Start / Stop

Once the scheduled task is configured, you can control is easily.

Start:
~~~
hotkeys start
~~~
You can achieve the same with `schtasks /Run /TN "Hotkeys"` (but hard to remember).

Stop:
~~~
hotkeys stop
~~~
You can achieve the same with `taskkill /f /im hotkeys.exe`, but `hotkeys stop` terminates
the process gracefully so it can unregister bindings and close the log file.


## Status

Show scheduled task and process status:
~~~
hotkeys status
Task 'Hotkeys' scheduled=yes status=Ready, process=Running
~~~

Remember, the scheduled task only starts the process once at logon. When done, it
returns to `Ready` state and does not monitor the detached hotkeys process anymore.


## Configuration

By default, the hotkeys configuration file is loaded from: `%USERPROFILE%\.config\hotkeys.toml`.

You can override the configuration directory by setting the `HOTKEYS_CONFIG_HOME`
environment variable, or by specifying the full path with the `--config` flag.

The configuration is hot reloaded on every change to the keybindings file.


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


## Additional notes

* When hotkeys is already running, it won't start another instance. This is by design,
  since you cannot register keybindings multiple times.
  
  Also keep this in mind when running the scheduled task: it will not restart an
  already running hotkeys process. Use the hotkeys start/stop commands to manage the
  process.

* For some console applications, eg. `action = [ "wait.exe", "20" ]`, nothing seems
  to happen, but the process is running:

~~~
❯ tasklist /FI "IMAGENAME eq wait.exe"

Image Name                     PID Session Name        Session#    Mem Usage
========================= ======== ================ =========== ============
wait.exe                     21452 Console                    1      6,620 K
~~~

This is normal since the hotkeys process itself is hidden. If you need to see console
output, use this:

~~~
action = [ "cmd", "/c", "wait.exe", "20" ]
~~~

* When starting alacritty without `cmd /c`, all child terminals launched by hotkeys
  in console mode are killed when hotkeys is stopped. When hotkeys is executed with
  `--background` (general use case), this issue does not occur.

* When you execute `hotkeys install`, the absolute path to the `hotkeys.exe` binary
  is hard coded in the scheduled task. If you move the binary, the task won't start.
  Check the registered path with:

~~~
schtasks /Query /TN "Hotkeys" /V /FO LIST
~~~
