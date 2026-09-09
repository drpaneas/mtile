# Public launch checklist

This is a maintainer checklist, not a claim that the project has already
published releases or enabled GitHub features. Local documentation and
workflow files cannot change repository visibility, settings, or social metadata.

## Repository identity

Suggested GitHub description:

> Build native Game Boy Advance games in Lua, with an integrated sprite, map, audio, and code editor.

Suggested topics:

```text
gameboy-advance gba gamedev homebrew lua tinygo golang pixel-art retro-games
```

Confirm the intended owner/repository URL before publishing. The documentation
currently uses `github.com/drpaneas/mtile`. Set the About description, topics,
and documentation link in GitHub. Use an actual editor/game capture for the
social preview; [the README capture](media/README.md) is reproducible.

## Before opening the repository

- Review the complete staged and unstaged diff, including deletions and assets.
  Preserve a clear, reviewable commit history for the work being published.
- Confirm that the public checkout includes every introductory example and
  submodule reference, with no local paths, build output, or private research
  material required to follow the quick start.
- Run the documented quick start from a fresh clone and check all README links.
- Choose and publish a private maintainer contact for security and community
  concerns. Enable GitHub private vulnerability reporting and verify the
  reporting route described in [SECURITY.md](../SECURITY.md).
- Review [third-party notices](third-party/README.md) and distribution inputs.
  Do not advertise research ports or external game assets as original Mtile work.

## GitHub settings

- Enable Issues so the checked-in bug, feature, and help forms are available.
- Enable required pull-request checks after CI has passed on the default branch.
  Use the actual check names from the workflow; restrict direct pushes as
  appropriate for the project's maintenance model.
- Keep Actions permissions read-only by default and review workflow changes.
- Enable dependency/security alerts where available; decide who will triage them.
- Add a CI badge only once its public URL and default-branch status are verified.
- Enable Discussions only if someone will maintain it; Issues already provide
  a help route through the Question / help form.

GitHub recognizes the repository's contribution, conduct, support, security,
and template files as [community-health resources](https://docs.github.com/en/communities/setting-up-your-project-for-healthy-contributions/creating-a-default-community-health-file).
The forms follow [GitHub's issue-form format](https://docs.github.com/en/communities/using-templates-to-encourage-useful-issues-and-pull-requests/syntax-for-issue-forms).

## First release and announcement

Build and verify artifacts using the [release guide](../scripts/release/README.md).
Publish a small release with platform requirements, hashes, known limitations,
and a working example. Do not imply that a directory produced by the packaging
script has already passed clean-machine or physical-hardware validation.

A useful announcement shows a short real capture, the Lua that creates it,
setup instructions, and the current limitations. Invite developers to try one
example and report their experience. Present measured performance with its
workload and hardware/emulator context.
