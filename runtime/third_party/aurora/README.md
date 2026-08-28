# Aurora 1.1 palette

Richard “DawnBringer” Fhager created the 256-color Aurora palette for his
GrafX2 Toolbox. This directory includes his original palette script and the
GNU General Public License, version 2. GrafX2's maintainer recorded the
author's permission to license the toolbox scripts under the GPL when adding
this file. [Upstream commit](https://gitlab.com/GrafX2/grafX2/-/commit/03fa08855b278b1240467f61d6c37467a94c6035)

[pfunc_pal_Aurora11.lua](pfunc_pal_Aurora11.lua) contains the original 256
indexed RGB triples. mTile stores the triples as six-digit RGB values in
`internal/assetsync/aurora.hex`. Its GBA palette uses the upper five bits
of each channel, packed as `r | (g << 5) | (b << 10)`.

When distributing the palette, include this source, notice and
[license](COPYING) with the project or ROM that embeds the table.
