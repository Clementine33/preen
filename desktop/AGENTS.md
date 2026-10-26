# Desktop Shell

- This is an independent Go module. Keep Wails bindings, assets and native application lifecycle here; keep service layout, credentials, logging and process lifecycle in `supervisor`, which uses only the standard library. Communicate with the backend executable through its handshake, HTTP and stdin; never import the backend Go module.
- Acquire Wails' single-instance lock before preparing or starting the service. Closing the main window exits the application and waits for supervised shutdown; startup and unexpected-exit errors retain their diagnostic tail in a native dialog.
- Pin both `github.com/wailsapp/wails/v3` and the frontend's `@wailsio/runtime` to the same exact `3.0.x` release. Linux uses the default GTK4/WebKitGTK 6.0 build. Keep Wails runtime imports inside the frontend's desktop-only dynamic import.
- Use root `desktop:dev` and `desktop:build` tasks to populate embedded assets and build both binaries with one version. Release builds require `RAW_TAG`; development data stays under `.dev_data/desktop` on Linux.
- Run `go test ./supervisor` without GTK or WebView. Its lifecycle tests build a real backend. To compile the shell, first populate `assets` with the shared frontend build (CI may use a placeholder index); Linux compilation requires GTK4 and WebKitGTK 6.0 development libraries. Native UI and media playback need platform acceptance checks.
