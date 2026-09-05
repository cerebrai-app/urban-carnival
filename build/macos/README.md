# macOS packaging

Turns a prebuilt `cerebrai-desktop` binary into `CerebrAI.app` and a
`.dmg` installer.

| File            | Purpose                                                        |
| --------------- | ------------------------------------------------------------- |
| `package-app.sh` | Assembles the `.app` bundle; `--dmg` also builds the installer. |
| `Info.plist.in`  | Bundle metadata template (`@TOKENS@` filled by the script).     |
| `icon.png`       | 1024×1024 source icon. **Placeholder — replace with the real art.** |

The script builds nothing itself: the binary comes from `make build-desktop`
or from goreleaser in the release workflow, so version ldflags have one
source of truth.

## Local use

```sh
make package-macos          # -> dist/macos/CerebrAI.app
make package-macos DMG=1    # also -> dist/macos/CerebrAI-<version>-<arch>.dmg
make install-macos          # build + overwrite the copy in /Applications
```

`install-macos` quits a running `CerebrAI`, then swaps a fresh build in for
the installed bundle (copy first, replace only once the copy succeeds).
Override the destination with `INSTALL_DIR=$HOME/Applications`.

## Signing

The bundle is only ad-hoc signed. That is enough to launch on Apple Silicon
but not to pass Gatekeeper on a download — first launch needs right-click ▸
Open, or `xattr -dr com.apple.quarantine CerebrAI.app`. A Developer ID
signature plus notarization is out of scope here (it needs Apple credentials
in CI).
