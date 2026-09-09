#!/usr/bin/env python3
"""Check local links in Mtile's maintained public guides, without network IO.

This intentionally does not crawl dated research reports or validate remote URLs.
Pass repository-relative Markdown paths to check a different set of guides.
"""

from collections import Counter
from pathlib import Path
import re
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
GUIDES = (
    "README.md", "CONTRIBUTING.md", "SUPPORT.md", "SECURITY.md", "CODE_OF_CONDUCT.md",
    "docs/README.md", "docs/getting-started.md", "docs/reference.md",
    "website/README.md", "docs/first-game.md", "internal/build/templates/coin-quest/README.md",
    "docs/launch-checklist.md", "docs/third-party/README.md", "docs/media/README.md",
    "examples/README.md", "scripts/release/README.md",
    "docs/ide.md", "docs/render-modes.md", "scripts/release/BUNDLE-README.md",
    "docs/toolchain-042-performance.md",
    "docs/audio/captures/README.md", "docs/audio/captures/v2/README.md",
    *(str(path.relative_to(ROOT)) for path in sorted((ROOT / "docs/audio").glob("*.md"))),
)


def without_fences(text):
    lines = []
    fence = None
    for line in text.splitlines(keepends=True):
        marker = re.match(r"^\s*(`{3,}|~{3,})", line)
        if marker:
            value = marker.group(1)
            if fence is None:
                fence = value
            elif value[0] == fence[0] and len(value) >= len(fence):
                fence = None
            lines.append("\n")
        else:
            lines.append("\n" if fence else line)
    return "".join(lines)


def headings(text):
    counts = Counter()
    anchors = set()
    for heading in re.findall(r"^#{1,6}\s+(.+?)\s*#*\s*$", without_fences(text), re.M):
        heading = re.sub(r"\[([^]]+)\]\([^)]+\)", r"\1", heading)
        slug = re.sub(r"[^\w\- ]", "", heading.lower()).replace(" ", "-")
        anchors.add(slug if not counts[slug] else f"{slug}-{counts[slug]}")
        counts[slug] += 1
    anchors.update(re.findall(r'<a\s+(?:id|name)=[\"\']([^\"\']+)', text))
    return anchors


def check_file(path, root=ROOT):
    errors = []
    if not path.is_file():
        return [f"missing guide: {path}"]
    text = without_fences(path.read_text(encoding="utf-8"))
    for match in re.finditer(r"\[[^\]\n]*\]\((<[^>]+>|[^\s)]+)(?:\s+\"[^\"]*\")?\)", text):
        target = match.group(1).strip("<>")
        url = urlsplit(target)
        if url.scheme or url.netloc:
            continue
        destination = (path.parent / unquote(url.path)).resolve() if url.path else path
        line = text.count("\n", 0, match.start()) + 1
        prefix = f"{path.relative_to(root)}:{line}: {target}"
        if not destination.is_relative_to(root):
            errors.append(prefix + " leaves the repository")
        elif not destination.exists():
            errors.append(prefix + " does not exist")
        elif url.fragment:
            if destination.is_dir():
                destination /= "README.md"
            if destination.suffix.lower() == ".md":
                if not destination.is_file() or unquote(url.fragment) not in headings(destination.read_text(encoding="utf-8")):
                    errors.append(prefix + " has no matching heading")
    return errors


def main():
    guides = sys.argv[1:] or GUIDES
    errors = []
    for name in guides:
        path = (ROOT / name).resolve()
        if not path.is_relative_to(ROOT):
            errors.append(f"guide is outside the repository: {name}")
        else:
            errors.extend(check_file(path))
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print(f"Checked local links and headings in {len(guides)} guides.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
