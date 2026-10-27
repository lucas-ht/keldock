---
name: Keldock Console
description: Compact application operations on customer-controlled infrastructure.
colors:
  background: "#f7f7f0"
  foreground: "#222a25"
  card: "#fdfdf8"
  card-foreground: "#222a25"
  popover: "#fdfdf8"
  popover-foreground: "#222a25"
  primary: "#a44228"
  primary-foreground: "#fff"
  secondary: "#e8edde"
  secondary-foreground: "#294233"
  muted: "#edf0e5"
  muted-foreground: "#5b665b"
  accent: "#e9efe2"
  accent-foreground: "#213d32"
  destructive: "#ad3d35"
  border: "#d6ddce"
  input: "#bdc9b7"
  ring: "#8b4e37"
  rail: "#213d32"
  rail-foreground: "#eff3e9"
  rail-muted: "#bccdbd"
  rail-active: "#e8edd9"
  success: "#237043"
  success-surface: "#e8f1e7"
  warning: "#805510"
  warning-surface: "#fbf0d7"
  failure: "#a33730"
  failure-surface: "#f8e9e4"
  header: "#fbfbf5"
  dark-background: "#161f1b"
  dark-foreground: "#e4ebdf"
  dark-card: "#1d2922"
  dark-card-foreground: "#e4ebdf"
  dark-popover: "#243129"
  dark-popover-foreground: "#e4ebdf"
  dark-primary: "#efac8f"
  dark-primary-foreground: "#382013"
  dark-secondary: "#304234"
  dark-secondary-foreground: "#e4ebdf"
  dark-muted: "#26362c"
  dark-muted-foreground: "#b0beac"
  dark-accent: "#304234"
  dark-accent-foreground: "#e4ebdf"
  dark-destructive: "#f29c91"
  dark-border: "#3a4b3e"
  dark-input: "#52664f"
  dark-ring: "#efac8f"
  dark-rail: "#101a15"
  dark-rail-foreground: "#e4ebdf"
  dark-rail-muted: "#b3c5b2"
  dark-rail-active: "#d4dfc7"
  dark-success: "#a4d0a6"
  dark-success-surface: "#233e2c"
  dark-warning: "#ebc27e"
  dark-warning-surface: "#44361e"
  dark-failure: "#f2aba2"
  dark-failure-surface: "#472b27"
  dark-header: "#1b261f"
typography:
  display:
    fontFamily: "Figtree Variable, sans-serif"
    fontSize: "clamp(32px, 4vw, 52px)"
    fontWeight: 550
    lineHeight: 1.08
    letterSpacing: "-0.025em"
  headline:
    fontFamily: "Figtree Variable, sans-serif"
    fontSize: "28px"
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: "-0.025em"
  title:
    fontFamily: "Figtree Variable, sans-serif"
    fontSize: "21px"
    fontWeight: 600
    lineHeight: 1.5
    letterSpacing: "-0.025em"
  body:
    fontFamily: "Figtree Variable, sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.5
  control:
    fontFamily: "Figtree Variable, sans-serif"
    fontSize: "14px"
    fontWeight: 500
    lineHeight: "20px"
  label:
    fontFamily: "Figtree Variable, sans-serif"
    fontSize: "12px"
    fontWeight: 500
    lineHeight: "16px"
  mono:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, Liberation Mono, Courier New, monospace"
    fontSize: "12px"
    fontWeight: 400
    lineHeight: "16px"
rounded:
  mark: "2px"
  navigation: "6px"
  base: "8px"
  control: "10px"
  panel: "12px"
  badge: "32px"
spacing:
  tight: "4px"
  small: "8px"
  detail: "12px"
  item: "16px"
  group: "20px"
  section: "24px"
  region: "28px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.primary-foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.control}"
    padding: "0 10px"
    height: "32px"
  button-outline:
    backgroundColor: "{colors.background}"
    textColor: "{colors.foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.control}"
    padding: "0 10px"
    height: "32px"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.foreground}"
    typography: "{typography.control}"
    rounded: "{rounded.control}"
    padding: "0 10px"
    height: "32px"
  button-destructive:
    backgroundColor: "color-mix(in oklab, #ad3d35 10%, transparent)"
    textColor: "{colors.destructive}"
    typography: "{typography.control}"
    rounded: "{rounded.control}"
    padding: "0 10px"
    height: "32px"
  input:
    backgroundColor: "transparent"
    textColor: "{colors.foreground}"
    rounded: "{rounded.control}"
    padding: "4px 10px"
    height: "32px"
  rail-navigation:
    backgroundColor: "{colors.rail}"
    textColor: "{colors.rail-muted}"
    rounded: "{rounded.navigation}"
    padding: "10px 13px"
    height: "42px"
  status:
    backgroundColor: "{colors.success-surface}"
    textColor: "{colors.success}"
    typography: "{typography.label}"
    rounded: "{rounded.badge}"
    padding: "2px 8px"
    height: "20px"
  panel:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.panel}"
  revision-component:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    padding: "18px 16px"
  brand-mark:
    width: "23px"
    height: "25px"
---

<!-- markdownlint-configure-file {"MD025": {"front_matter_title": "^title:"}} -->

# Design System: Keldock Console

## Overview

<!-- markdownlint-disable MD036 -->

**Creative North Star: "Field Guide"**

<!-- markdownlint-enable MD036 -->

Field Guide describes the implemented console: a forest navigation rail frames a pale, green-tinted workspace; compact records, clear headings and restrained controls keep deployment evidence readable. It is a provisional descriptive direction used after an unanswered preview, not a user-approved visual identity. The code and reviewed screens govern the rules below.

The durable product constraint is factual, revision-aware communication. Health, freshness, permissions and recoverable actions remain legible in both themes. Administration reuses the same typography, controls and surfaces; authentication gives the forest surface more space without introducing a second identity.

**Key Characteristics:**

- Forest navigation and green-tinted neutral surfaces.
- Compact tabular records with labeled semantic health.
- Locally sourced Figtree with monospace technical evidence.
- Flat persistent surfaces and restrained temporary elevation.
- Responsive evidence that wraps without hiding fixture provenance.

Source authority: `src/app/globals.css`, `src/app/styles/{shell,content,auth,responsive}.css`, and the implemented UI, layout, shared and deployment components. The frontmatter records their current light tokens and `dark-` counterparts; keep `globals.css` authoritative when changing themes. The sidecar ramps are synthesized preview metadata, not additional shipping palette values. The surface composition belongs in `.impeccable/surfaces/apps-console-src-app-console-page-tsx.md`.

## Colors

The palette combines forest and sage with pale paper tones, a clay action accent and functional health pairs. Descriptive names here remain provisional; exact source values are normative in the frontmatter.

### Primary

- **Fired Clay** (`primary`) colors primary actions; **Pale Apricot** (`dark-primary`) serves the same role in dark mode, with its paired foreground.
- **Clay Focus** (`ring`) supplies visible interaction focus and follows the theme counterpart in dark mode.

### Secondary

- **Forest Rail** (`rail`) anchors navigation; **Pale Leaf** (`rail-active`) identifies the active destination with dark forest text.
- **Sage Control** (`secondary`) and **Selection Sage** (`accent`) group quiet controls and selected records. Use their paired foreground tokens.

### Tertiary

- **Health Green**, **Attention Ochre** and **Failure Red** use `success`, `warning` and `failure` with their matching `-surface` tokens. Destructive controls use `destructive` separately.

### Neutral

- **Pale Field** (`background`), **Paper Panel** (`card`) and **Header Paper** (`header`) separate workspace, records and navigation context.
- `foreground`, `muted-foreground`, `border` and `input` establish reading hierarchy and control edges; `muted` provides footers and evidence notices.
- Dark mode uses the corresponding `dark-` values: a deep green workspace, lighter green records and warm pale action text. Popovers have their own dark surface.

**The Paired Status Rule.** Health uses a named state, a dot and its foreground/surface pair; preserve the accompanying evidence or freshness text.

## Typography

**Display and body font:** Figtree Variable, loaded locally through `@fontsource-variable/figtree`, with sans-serif fallback. **Technical font:** the platform monospace stack in `typography.mono`.

The same humanist family carries headings, data and forms. Tabular numerals are enabled on the body. Headings use slight negative tracking; labels retain sentence case.

- **Display:** authentication introduction only; fluid sizing, moderate weight and close leading follow `typography.display`.
- **Headline:** page and authentication form titles; mobile console titles reduce to (26px).
- **Title:** section headings; administration subheadings also use (18px, 600).
- **Body:** regular reading and table content follow `typography.body`; page introductions are bounded at (68ch).
- **Control / Label:** medium-weight actions and compact metadata follow their frontmatter roles. Inputs use (16px) below the framework's (768px) breakpoint and (14px) above it.
- **Mono:** short hashes in tables and complete identifiers in details. Full diagnostic evidence and endpoint strings wrap when needed.

**The Evidence Type Rule.** Use monospace for revisions, hashes, callback addresses and resource identifiers, and Figtree for names, state labels and prose.

## Layout

Desktop uses a sticky full-height rail (220px) and a fluid workspace. The header is (76px) high. Main content has (30px 34px 40px) padding, centers within a (1700px) maximum, and uses compact panels instead of a freestanding card grid. Forms are bounded at (600px); authentication forms at (370px).

At (1000px) and below, workspace insets reduce to (24px) and optional table columns disappear. At (760px) and below, a left sheet replaces the rail, the header becomes (64px), content insets become (24px 18px 36px), and headings/actions stack. Table columns preserve application, environment and status; component evidence becomes a single column. Framework visibility utilities also use (640px) and (768px) breakpoints.

Panel toolbars wrap their search and filter controls. Toolbars use (14px 16px), footers (9px 16px), data rows (18px 20px), and component cells (18px 16px). The spacing scale records recurring rhythm rather than every local inset. Authentication uses a forest introduction column (minimum 300px, 42%) and a centered form; mobile keeps the brand and form while collapsing the introduction copy.

## Elevation & Depth

Persistent tables, panels and forms are flat. Fine borders and changes of surface tone establish their boundaries. Select menus use the medium shadow; the temporary navigation sheet uses the large shadow. Dialogs use a fine foreground ring and a lightly blurred backdrop. The selection transition is a brief accent edge, not a permanent panel shadow.

### Shadow Vocabulary

- **Menu:** `0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1)`.
- **Navigation sheet:** `0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1)`.

**The Flat Record Rule.** Persistent records use tone and fine borders; temporary selection menus and navigation sheets use their implemented elevation.

## Shapes

Controls use softly rounded corners; navigation is slightly tighter and panels broader. Use the frontmatter radius roles. Badges have pill corners, avatars are circular, and status dots are small circles (5px). One-pixel dividers organize rows and adjacent components.

The Keldock mark is CSS geometry: two bottom-aligned bars, one tall pale-green bar and one shorter clay bar, with small corner rounding. Lucide supplies interface icons as SVG; the shipping interface contains no raster imagery.

## Components

### Buttons

Compact and quiet, with a standard height (32px), horizontal padding (10px), medium Figtree text and a small icon gap (6px). Primary uses the action pair; outline uses the workspace and border; ghost reveals a muted hover surface; destructive uses a translucent destructive surface. The source also defines secondary and link variants for library use.

Hover changes surface tone. Focus uses a (3px) half-opacity ring and matching border. Pressing ordinary buttons shifts them down (1px). Disabled buttons use half opacity and stop interaction. Icons are normally (16px), and icon-only controls retain accessible names.

### Chips

Health badges pair compact (12px) text with a status dot and semantic foreground/surface colors. Their normal height is (20px). Neutral states use the secondary pair. These are records of state, not unlabeled decorative dots.

### Cards / Containers

Panels use the card surface, fine border and panel radius, with clipped internal corners. Toolbars, muted notices and footers share the panel boundary. Administration lists divide rows without separate shadows. Long technical values wrap within their region.

### Inputs / Fields

Inputs and select triggers are compact (32px) with the control radius, input border and horizontal inset (10px). Their background is transparent in light mode and a translucent input tone in dark mode. Placeholders use muted text. Focus uses the button ring treatment; invalid fields use the destructive border/ring. Disabled fields show reduced opacity and a non-interactive cursor. Visible labels and short descriptions sit outside the field.

### Navigation

The rail combines medium-weight labels with SVG icons (18px). Links have a minimum height (42px), an inset (10px 13px), muted default text, a lightened forest hover, and a pale active surface. Active state is exposed with `aria-current`. The mobile sheet reuses the same navigation and account context. Plain links use the global focus outline (2px with 3px offset).

### Revision evidence

The component strip keeps a freshness/generation notice above equal-width component cells (minimum 150px). Each cell presents name and labeled health, reason, image digest and dependencies. It stacks on mobile and preserves complete diagnostic identifiers in details. Selection triggers a short transition (180ms, ease-out); menus/dialogs use short state transitions (100–200ms). Reduced-motion preference removes animation and transitions.

## Do's and Don'ts

### Do

- Do use the semantic CSS variables so light and dark themes retain paired contrast.
- Do retain visible focus, meaningful labels and text alongside health color.
- Do allow full diagnostic hashes, endpoints and resource identifiers to wrap on narrow screens.
- Do preserve visible synthetic-fixture provenance at mobile and desktop widths.
- Do reuse the local Figtree asset, CSS brand geometry and Lucide SVG icons.

### Don't

- Don't infer readiness from a successful submission or stale observation.
- Don't add shadows to persistent tables, evidence panels or administration lists.
- Don't replace labeled health with color alone or clip full diagnostic evidence.
- Don't promote this provisional direction or the code-led sketch to user-approved identity.

Verification scope: source extraction was compared with final local desktop, mobile, dark, deployment-details and account screenshots. The finish review reported ship after mobile diagnostic wrapping and fixture-notice corrections. Separate Field Guide catalog imagery was unavailable; the sketch is a code-led composition reference. No additional design defect was identified or canonized in this documentation pass.
