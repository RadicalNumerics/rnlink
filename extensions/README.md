# RN Links browser extensions

One Manifest V3 extension for Chrome and Safari. It turns exact Google searches
for `rn/` and `rn/<shortname>` into navigation to a configured golink server.
The server and its UI live at the repository root; browser source lives here.

## Build

Requires Node.js 22 or later. No npm dependencies are needed.

```sh
node extensions/scripts/build.mjs https://rn.YOUR-TAILNET.ts.net/
```

The generated `extensions/dist/` directory works in both browsers. Supply your
server's actual HTTPS origin. Generated files and archives are ignored so the
public repository does not contain internal DNS names or deployment configuration.
The hostname must remain reachable from the user's laptop (typically via Tailscale).

To create an archive with the manifest at its root:

```sh
(cd extensions/dist && zip -r ../rn-links.zip .)
```

## Local installation

### Chrome

1. Open `chrome://extensions` and turn on Developer mode.
2. Choose **Load unpacked** and select `extensions/dist/`.
3. After rebuilding, click the extension's reload button.

### Safari

1. In Settings > Advanced, enable **Show features for web developers**.
2. In Settings > Developer, choose **Add Temporary Extension** and select
   `extensions/dist/` or the generated ZIP. Approve the unsigned development build.
3. Enable RN Links, and grant website access to Google and the configured server.
   Safari requires permission for both sides of a redirect.
4. Reload the extension after rebuilding. If installed from a ZIP, replace it
   with the new archive rather than reloading an old extracted copy.

Temporary Safari extensions expire on quit or after 24 hours. If the temporary
extension option is unavailable, package a Safari app using Apple's tools.
This repository does not yet provide signed Safari apps or Chrome Web Store releases.

## Verification

Build with the fixture hostname before running automated routing tests:

```sh
node extensions/scripts/build.mjs https://rn.example.ts.net/
node extensions/test/routing.mjs
node --check extensions/dist/diagnostics.js
# macOS with Swift command-line tools:
swift extensions/test/webkit.swift extensions/dist/rules.json
```

Rebuild with the real destination before installing. Create a disposable `test`
link on your server, then check in normal Chrome and Safari windows:

- `rn/test` redirects to the chosen destination.
- `rn/` opens the index and link creation form.
- A second, previously unvisited shortname redirects correctly (to rule out history).
- Ordinary searches, including `what is rn/test`, remain Google searches.

The toolbar popup reports enabled rulesets, host permissions, and regex support.
The original local pilot was manually confirmed in both Chrome and Safari.
The consolidated build must be smoke-tested again before release.

WebKit's content-rule compiler rejects regex alternation (`|`), although Chrome
accepts it. Separate rules handle literal `/` and encoded `%2F`. CI compiles the
filters with native WebKit in addition to testing 58 routing cases. Compilation
tests do not validate browser permission prompts or address-bar behavior.

## Current scope and privacy

- Google.com search only; other search engines and regional Google domains are
  not intercepted.
- Shortnames start with an ASCII letter, digit, or underscore and may contain
  letters, digits, underscores, periods, and hyphens. Nested path arguments and
  encoded Unicode are not supported yet.
- Static browser-managed rules apply only to top-level navigation. There are no
  content scripts, background processes, analytics, or stored browsing history.
- Diagnostics execute only in the popup and are not uploaded.
- Search suggestions can still send typed text to the search provider before
  navigation. These rules do not control suggestions.

## Distribution and upstream maintenance

Chrome Web Store publication and a signed Safari app are the intended permanent
installation paths. Signing does not bypass Safari's website-permission consent
on unmanaged Macs. Signing credentials and internal configuration must stay out
of Git. No publication or installation occurs automatically in CI.

Keep the upstream server's module path and license intact. Fetch upstream fixes
with `git remote add upstream https://github.com/tailscale/golink.git` (once), then
`git fetch upstream`. Integrate updates through reviewed branches and PRs.

References: [Chrome redirect API](https://developer.chrome.com/docs/extensions/reference/api/declarativeNetRequest),
[Safari installation](https://developer.apple.com/documentation/safariservices/running-your-safari-web-extension),
[Safari rules and permissions](https://developer.apple.com/documentation/safariservices/blocking-content-with-your-safari-web-extension).
