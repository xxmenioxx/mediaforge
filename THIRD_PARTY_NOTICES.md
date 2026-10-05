# Third-Party Notices

MVForge uses third-party software under its respective licenses. This is an
engineering inventory, not legal advice. The MVForge project license remains
Apache License 2.0; see [LICENSE](LICENSE).

## Runtime/frontend dependencies

The 23 direct dependencies in `frontend/package.json` below were checked
against installed package metadata and `frontend/package-lock.json` after
`npm ci`. Source links point to the packages' upstream projects. MIT and
Apache-2.0 are treated here as permissive licenses with notice-preservation
requirements when redistributed.

| Package | Manifest range → locked version | Declared license | Upstream source | Use |
| --- | --- | --- | --- | --- |
| `@emotion/react` | `11.14.0` → `11.14.0` | MIT | [Emotion](https://github.com/emotion-js/emotion) | Runtime |
| `@emotion/styled` | `11.14.1` → `11.14.1` | MIT | [Emotion](https://github.com/emotion-js/emotion) | Runtime |
| `@mui/icons-material` | `7.3.6` → `7.3.6` | MIT | [MUI](https://github.com/mui/material-ui) | Runtime |
| `@mui/material` | `7.3.6` → `7.3.6` | MIT | [MUI](https://github.com/mui/material-ui) | Runtime |
| `@tanstack/react-query` | `5.90.12` → `5.90.12` | MIT | [TanStack Query](https://github.com/TanStack/query) | Runtime |
| `react` | `18.3.1` → `18.3.1` | MIT | [React](https://github.com/facebook/react) | Runtime |
| `react-dom` | `18.3.1` → `18.3.1` | MIT | [React](https://github.com/facebook/react) | Runtime |
| `react-router-dom` | `6.30.4` → `6.30.4` | MIT | [React Router](https://github.com/remix-run/react-router) | Runtime |
| `@eslint/js` | `9.39.2` → `9.39.2` | MIT | [ESLint](https://github.com/eslint/eslint) | Development tooling |
| `@testing-library/react` | `^16.3.2` → `16.3.2` | MIT | [Testing Library](https://github.com/testing-library/react-testing-library) | Development tooling |
| `@testing-library/user-event` | `^14.6.5` → `14.6.5` | MIT | [Testing Library](https://github.com/testing-library/user-event) | Development tooling |
| `@types/node` | `22.19.1` → `22.19.1` | MIT | [DefinitelyTyped](https://github.com/DefinitelyTyped/DefinitelyTyped) | Development tooling |
| `@types/react` | `18.3.27` → `18.3.27` | MIT | [DefinitelyTyped](https://github.com/DefinitelyTyped/DefinitelyTyped) | Development tooling |
| `@types/react-dom` | `18.3.7` → `18.3.7` | MIT | [DefinitelyTyped](https://github.com/DefinitelyTyped/DefinitelyTyped) | Development tooling |
| `@vitejs/plugin-react` | `5.1.2` → `5.1.2` | MIT | [Vite React plugin](https://github.com/vitejs/vite-plugin-react) | Development tooling |
| `eslint` | `9.39.2` → `9.39.2` | MIT | [ESLint](https://github.com/eslint/eslint) | Development tooling |
| `eslint-plugin-react-hooks` | `7.0.1` → `7.0.1` | MIT | [React](https://github.com/facebook/react) | Development tooling |
| `eslint-plugin-react-refresh` | `0.4.26` → `0.4.26` | MIT | [eslint-plugin-react-refresh](https://github.com/ArnaudBarre/eslint-plugin-react-refresh) | Development tooling |
| `jsdom` | `^30.0.1` → `30.0.1` | MIT | [jsdom](https://github.com/jsdom/jsdom) | Development tooling |
| `typescript` | `5.9.3` → `5.9.3` | Apache-2.0 | [TypeScript](https://github.com/microsoft/TypeScript) | Development tooling |
| `typescript-eslint` | `8.50.0` → `8.50.0` | MIT | [typescript-eslint](https://github.com/typescript-eslint/typescript-eslint) | Development tooling |
| `vite` | `7.3.6` → `7.3.6` | MIT | [Vite](https://github.com/vitejs/vite) | Development tooling |
| `vitest` | `^4.1.11` → `4.1.11` | MIT | [Vitest](https://github.com/vitest-dev/vitest) | Development tooling |

Classification: **A, permissive/open-source:** 23 direct packages (22 MIT, one
Apache-2.0). **B, attribution/notice:** all 23 require applicable notices to be
preserved when redistributed. **C, commercial/restricted:** zero found.
**D, unclear/manual review:** zero found. The generated
[`frontend/licenses/npm/THIRD_PARTY_NOTICES.txt`](frontend/licenses/npm/THIRD_PARTY_NOTICES.txt)
and
[`frontend/licenses/npm/inventory.json`](frontend/licenses/npm/inventory.json)
cover the 90 resolved production/shared npm packages, including runtime direct
dependencies. They are regenerated and checked by
`frontend/scripts/generate-license-inventory.mjs`; development-only tools in the
table are intentionally outside that runtime bundle.

## MUI and MUI X

The lockfile contains eight `@mui/` packages: direct `@mui/material` and
`@mui/icons-material`, plus `@mui/core-downloads-tracker`, `@mui/private-theming`,
`@mui/styled-engine`, `@mui/system`, `@mui/types`, and `@mui/utils`. All eight
declare MIT. No MUI X package is present in the lockfile or frontend imports.

MUI X Community packages may be considered after their exact package license is
verified. MUI X Pro/Premium and other commercial packages require an explicit
license and approval before inclusion. This includes current or renamed Pro and
Premium package variants such as `@mui/x-data-grid-pro`,
`@mui/x-data-grid-premium`, `@mui/x-date-pickers-pro`, `@mui/x-charts-pro`, and
`@mui/x-tree-view-pro`; check the actual package name and license at the time of
adoption.

## Incorporated/adapted source

No third-party UI template code, standalone icon pack, font, image, or other
source substantially copied or adapted into the frontend was identified in
this audit. Add an entry here before distributing any later copied or
substantially adapted material. Record its project/component, exact upstream
repository and file, copyright holder, license, local files, and modification
note; retain any required upstream notices.

## Design/reference-only projects

- Mantis Free React Admin Template — reference-only; no code incorporated.
- Official MUI templates and examples — reference-only; no code incorporated.

Assets V2 remains MVForge's visual source of truth, as set out in [issue
#212](https://github.com/xxmenioxx/mediaforge/issues/212) and [issue
#213](https://github.com/xxmenioxx/mediaforge/issues/213). Prefer studying a
pattern and implementing it with MVForge tokens and components over copying a
template and recoloring it. Visual similarity alone is not evidence that source
code was copied.

Before copying or substantially adapting third-party source, verify its exact
upstream location and license, preserve required notices, and record the
provenance above. Software licenses do not automatically cover logos,
trademarks, branding, screenshots, illustrations, stock/demo artwork, or fonts
distributed under separate terms.

## Frontend asset notes

- **Fonts:** `frontend/src/theme/theme.ts` names Inter first and then uses
  system/local fallbacks. Inter is not bundled or downloaded by the frontend;
  no font files, `@font-face`, or remote font import were found.
- **Icons:** the direct icon dependency is `@mui/icons-material` 7.3.6 (MIT).
  Its installed package license is included in the generated npm notices. No
  separate icon pack or copied SVG/logo asset was found.
- **Images and charts:** no static frontend image/illustration files or direct
  charting dependency was found. The frontend imports MUI icons from the package
  above. This was a product UI asset review, not a forensic review of media or
  test fixtures.

## Review policy and license-text files

New GPL, AGPL, SSPL, source-available, custom-commercial, or unclear-license
dependencies or copied source require explicit review before inclusion. This is
a conservative project policy, not a general legal conclusion that any license
is incompatible. Pro/Premium or other paid template code also requires explicit
approval and an applicable license.

No new root `licenses/` directory is needed for the current audit: runtime npm
license texts are already collected under `frontend/licenses/npm/`, and other
existing notices are kept with their respective component inventories. No
additional standalone third-party license text for incorporated frontend
source was identified.
