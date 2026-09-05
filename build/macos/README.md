# macOS packaging

Turns a prebuilt `cerebrai-desktop` binary into `CerebrAI.app` and a
`.dmg` installer.

| File            | Purpose                                                        |
| --------------- | ------------------------------------------------------------- |
| `package-app.sh` | Assembles the `.app` bundle; `--dmg` also builds the installer, `--env KEY=VALUE` adds `LSEnvironment` entries. |
| `Info.plist.in`  | Bundle metadata template (`@TOKENS@` filled by the script).     |
| `icon.png`       | Square source icon, 1024×1024 ideal (see [Credits](#credits)).   |

The script builds nothing itself: the binary comes from `make build-desktop`
or from goreleaser in the release workflow, so version ldflags have one
source of truth.

## Local use

```sh
make package-macos          # -> dist/macos/CerebrAI.app
make package-macos DMG=1    # also -> dist/macos/CerebrAI-<version>-<arch>.dmg
make install-macos          # build + overwrite the copy in ~/Applications, as a dev build
```

`install-macos` quits a running `CerebrAI`, then swaps a fresh build in for
the installed bundle (copy first, replace only once the copy succeeds). It
installs to the per-user `~/Applications` so no admin prompt is needed;
override with `INSTALL_DIR=/Applications`.

Unlike `package-macos`, `install-macos` bakes the dev-mode environment into
the bundle: it passes each `CEREBRAI_*` / `OTEL_*` name from the repo's
`.env`, plus `CEREBRAI_DB_PATH` pinned to the checkout's `cerebrai.db`, as
`--env KEY=VALUE`. `package-app.sh` writes those into `Info.plist` as an
`LSEnvironment` dict, which LaunchServices applies on a Finder/Dock launch —
so the installed app shows the Developer preferences section, logs at debug
level, and reads/writes the same database as `make run-desktop`. The pin is
needed because a Finder launch runs with working directory `/`, where the
plain `CEREBRAI_DEV_MODE` path (`./cerebrai.db`) would be unwritable and
abort startup.

## Signing

The bundle is only ad-hoc signed. That is enough to launch on Apple Silicon
but not to pass Gatekeeper on a download — first launch needs right-click ▸
Open, or `xattr -dr com.apple.quarantine CerebrAI.app`. A Developer ID
signature plus notarization is out of scope here (it needs Apple credentials
in CI).

## Credits

`icon.png`: <a href="https://www.flaticon.com/free-icons/brain" title="brain icons">Brain icons created by Reddie - Flaticon</a>

