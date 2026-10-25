---
name: mTile — Your next cartridge
description: An indigo handheld title screen that opens into a warm, readable GBA instruction booklet.
colors:
  paper-ground: "#eee8dc"
  guide-ground: "#e8e3ee"
  reading-paper: "#fffdf6"
  shell-light: "#9a8ac6"
  shell: "#7864a8"
  shell-shadow: "#6a5898"
  bezel: "#352d47"
  lcd: "#292e49"
  lcd-ink: "#f4edcd"
  guide-ink: "#42384f"
  muted-ink: "#6d6378"
  indigo: "#71549a"
  chapter-ink: "#584277"
  divider: "#d9d3e0"
  power-light: "#bee1a2"
  selection-coral: "#b56d5b"
typography:
  display:
    fontFamily: "Rubik, Arial, sans-serif"
    fontWeight: 750
    letterSpacing: "-.035em"
  body:
    fontFamily: "Trebuchet MS, Segoe UI, sans-serif"
    fontSize: "16px"
    lineHeight: 1.65
  mono:
    fontFamily: "JetBrains, Courier New, monospace"
rounded:
  guide-controls: "8–12px"
  reading-paper: "14–16px"
  console: "Large elliptical corners, reduced on narrow screens"
---

# Design System: mTile

## Direction

**“You used to carry whole worlds in your pocket. Now you can make one.”**

The site recalls the original indigo Game Boy Advance through the shape of a
handheld, the dark screen, the green power light, and a small cartridge-style
menu. Its destination feels like the instruction booklet that came in the box:
warm paper, indigo chapter navigation, clear diagrams, and comfortable technical text.
The nostalgia introduces mTile's purpose: making a game of your own.

This system applies to `website/`, not the editor UI or public Lua API. The
website uses the existing application logo rather than creating another
wordmark. Build-time copying from `cmd/editor/assets/mtile-header.png` keeps the
application, title menu, and guide on the same identity.

## Routes and source ownership

- `index.html` is the title/menu screen. Its four links lead to the first-game
  course, the guide, the examples, and installation.
- `guide.html` is the normal guide landing page. Guides, lessons, categories,
  API pages, and examples retain their existing routes.
- The menu's “Open the guide” link provides direct access to the guide. Guide
  navigation also offers a return to the title screen.
- `templates/menu.html`, `assets/menu.css`, and `assets/menu.js` own the title
  screen. `assets/site.css` and `assets/site.js` own the guide. `build.py` owns
  route generation, shared navigation, asset copying, and credits.

Edit these sources, not generated files in `_site/`. Keep instructional text,
source-backed guides, API classifications, local search, and relative URLs
intact when changing presentation.

## Menu composition

The title screen places the headline above a CSS handheld illustration. The
shared mTile logo sits inside its screen, followed by four ordinary HTML links.
They should read as choices in a game menu while remaining useful navigation.
A cream selection row and small coral arrow show the selected choice; focus
must remain visible independently of that treatment.

The D-pad, A/B buttons, speaker, and power light describe the console's physical
shape. They are decorative, hidden from assistive technology, and must not look
like working controls that the visitor needs to use. Do not add fake progress,
unavailable game modes, or a loading sequence before the guide opens.

The screen menu supports normal Tab and Enter navigation. Arrow Up/Down and
Home/End can move focus within the menu. Hover and focus share the visual
selection treatment. Navigation remains available without JavaScript.

## Guide composition

The guide is quieter than the title screen. It uses a cream reading surface,
an indigo header, a pale lavender topic sidebar, rounded search and action
controls, and restrained separators. The chapter navigation connects the two
views without turning documentation into a simulated game interface.

The landing page leads with a first-game action and an actual mTile game
capture inside a small console illustration. The learning steps and topic
index follow in the reading flow. Article pages keep their section table of
contents when space permits; API identifiers and examples retain their
monospace treatment.

There is no separate Nintendo character rail, character backdrop, or mobile
character frieze. The console and mTile's own game/editor captures carry the
visual story. Screenshots may include third-party game content and retain its
ownership notes.

## Color, type, and depth

Use indigo for the console and guide navigation, cream for paper and the menu
selection, dark violet for prose, and muted lavender for secondary surfaces.
Green belongs to the small power light; coral is a minor selection detail.
Code and the title-screen menu use contrasting dark surfaces with pale text.
Keep syntax colors legible on the code surface.

Rubik is self-hosted for guide display headings and title-screen body text.
Guide prose and controls use Trebuchet MS / Segoe UI / system sans-serif.
JetBrains Mono is self-hosted for code, keyboard hints, API names, and the
title-screen menu.
The logo is the application image; do not retype it using a display font.
Large headings can be expressive, while prose stays at a comfortable size and
line-height. Long identifiers must wrap or scroll within their own container.

Use soft shadows and inset highlights to suggest plastic on the console.
Reading surfaces use much lighter shadows. Controls have modest rounded
corners rather than oversized pills. Avoid the previous gold double frame,
ceremonial serif headings, or glossy 2000s web-button treatment.

## Responsive and accessible behavior

On smaller screens, simplify the handheld casing and remove decorative side
controls before reducing the usable screen. The menu links keep readable text
and sufficient space to select them. The guide collapses its topic sidebar
into the existing mobile navigation and stacks the landing-page content.
Dense code may scroll inside a code block without making the page scroll
sideways.

Preserve semantic navigation, image alternatives, the skip link, visible focus,
search announcements, copy-button feedback, and mobile menu states. Respect
reduced motion and keep the site usable without animation. Print styles should
produce readable content without the decorative console shell or navigation.
Color values here describe the intended palette; check actual contrast and
keyboard behavior when changing them.

## Assets and rights

Serve fonts and branding locally. Rubik lives in `assets/Rubik.ttf` with
`assets/RUBIK-OFL.txt`; copy JetBrains Mono and its OFL notice from the repository
with the generated site. Original CSS illustrations, SVG icons, site
code, and original project artwork follow the repository license; third-party
material keeps its own rights and notices.

Retain the credits link and a concise Nintendo trademark/non-affiliation notice
on the title screen and guide pages. Game Boy Advance is a Nintendo trademark;
mTile is an independent project and is not affiliated with or endorsed by
Nintendo. The credits page also identifies third-party content present in game
captures. Do not claim that attribution or a disclaimer grants a license or
permission. Decorative Nintendo character artwork from the earlier previews
is not part of this production design.

## Verification

After changes, rebuild and run the website checks. Inspect fresh menu, guide,
and article captures at desktop and mobile sizes. Exercise menu keyboard
navigation, direct guide entry, title-screen return, search, copy controls, and
the mobile topic menu. Check that nested routes and subdirectory hosting retain
working relative links. Screenshots in `build/website-style-previews/` are
review evidence, not source assets or a public deployment.
