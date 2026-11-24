---
name: handline-sim
description: Drive an iOS Simulator or an Android emulator with hl sim: boot a device, install and launch an app, read the screen, tap, swipe, type, press a button, rotate, take a screenshot. Use for any task that needs to see or operate a mobile app, instead of simctl, adb or a screen-control tool.
---

# hl sim

`hl sim` boots a device and drives it; the same commands work on iOS and Android.

1. `hl sim list` shows the devices (`ios` and `android`) and which are in use. `hl sim boot <name>` takes one and waits until it can be driven; the first iOS boot on a Mac builds a small driver with Xcode, which takes about a minute. A device you booted shuts down when your session ends, or `hl sim shutdown`.
2. `hl sim install <path to .app or .apk>`, then `hl sim launch <bundle id | package>`. The app you launch last is the one the next commands read and act in.
3. `hl sim tree` prints what is on the screen: type, "label", #id, =value and `@x,y WxH`. Read it to find what to tap; `--json` gives all of it.
4. `hl sim tap --label "General"` (or `--id`, or `--text` for part of a label or id) taps the first enabled match; `hl sim tap <x> <y>` taps a point. Run `tree` again afterwards: the screen changed.
5. `hl sim swipe up|down|left|right` (a finger's direction, so up scrolls down), `hl sim type "text"` types where the keyboard is, `hl sim button home` (Android also back, menu, volumeup, volumedown, power, enter, appswitch), `hl sim rotate portrait|left|right|upsidedown`.
6. `hl sim screenshot <file.png>` saves the screen; look at the file to check what the app drew.

With more than one device add `--device <name>` to each command.

Only one agent should drive a device: if the team has a shared run for simulators, take it first (`hl use <run> --hold`), and let it go when you are done.

When something is missing, `hl sim` says what. It never installs Xcode or the Android SDK; you do, from the command line:
- iOS needs Xcode (`xcode-select -p` and `xcrun simctl list` work). Xcode needs an Apple login to download: ask the founder for that, and only that.
- Android needs the SDK's `adb` and `emulator` and an AVD: `sdkmanager "platform-tools" "emulator" "system-images;android-34;google_apis;arm64-v8a"`, then `avdmanager create avd`.
