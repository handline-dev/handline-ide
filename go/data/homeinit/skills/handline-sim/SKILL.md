---
name: handline-sim
description: Drive an iOS Simulator or an Android emulator with hl sim: boot a device, install and launch an app, read the screen, tap, swipe, type, press a button, rotate, take a screenshot. Use for any task that needs to see or operate a mobile app, instead of simctl, adb or a screen-control tool.
---

# hl sim

The same commands drive iOS and Android.

1. `hl sim list` shows the devices and which are in use. `hl sim boot <name>` takes one and waits until it can be driven (the first iOS boot builds a small driver with Xcode, about a minute). It shuts down when your session ends, or `hl sim shutdown`.
2. `hl sim install <.app or .apk>`, then `hl sim launch <bundle id | package>`; the app launched last is the one the next commands act in.
3. `hl sim tree` prints the screen: type, "label", #id, =value and `@x,y WxH` (`--json` for all of it).
4. `hl sim tap --label "General"` (or `--id`, or `--text` for part of a label or id) taps the first enabled match; `hl sim tap <x> <y>` a point. Run `tree` again after: the screen changed.
5. `hl sim swipe up|down|left|right` (a finger's direction: up scrolls down), `hl sim type "text"`, `hl sim button home` (Android also back, menu, volumeup, volumedown, power, enter, appswitch), `hl sim rotate portrait|left|right|upsidedown`.
6. `hl sim screenshot <file.png>`; look at the file to check what the app drew.

With more than one device add `--device <name>` to each command. Only one agent drives a device: if the team has a shared run for simulators, take it first (`hl use <run> --hold`) and release it after (`hl use --release`).

`hl sim` says what is missing and never installs it; you do, from the command line. iOS needs Xcode (`xcode-select -p` and `xcrun simctl list` work); ask the founder only for the Apple login it wants. Android needs the SDK's `adb`, `emulator` and an AVD (`sdkmanager "platform-tools" "emulator" "system-images;android-34;google_apis;arm64-v8a"`, then `avdmanager create avd`).
