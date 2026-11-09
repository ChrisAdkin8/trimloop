# ADR 0049: A Claude Code plugin with a trimloop skill

- Status: Accepted — built: `.claude-plugin/marketplace.json` and `plugins/trimloop/`; passes `claude plugin validate --strict`; not yet tried by users
- Date: 2026-11-09
- Amends: [ADR 0008](0008-packaging-and-distribution.md) (a new artefact), [ADR 0047](0047-first-release-pure-go.md) (releases bump the plugin's version)

## Context

Most people who would use trimloop already work in Claude Code. Asking
Claude "where did my tokens go?" is easier than learning a command line,
and Claude can explain the report in the user's own terms. Claude Code
installs skills through plugins, listed in a marketplace that can live in
the same GitHub repository as the code.

What a plugin can and cannot do (Claude Code docs, checked 2026-11-09):
a plugin can ship skills, commands, agents and hooks; it cannot set the
status line; and its hooks run in addition to those in the user's
settings, so a plugin copy of trimloop's hooks would nudge twice for users
who already ran `trimloop setup --write`. A plugin does not install a
program: trimloop comes from Homebrew or a release download.

## Decision

1. **This repository is also a Claude Code marketplace.**
   `.claude-plugin/marketplace.json` lists one plugin, `trimloop`, at
   `plugins/trimloop/`. Users install it with
   `/plugin marketplace add ChrisAdkin8/trimloop` and
   `/plugin install trimloop@trimloop`.
2. **The plugin ships one skill and no hooks.** The skill
   (`skills/trimloop/SKILL.md`) checks that trimloop is installed (and says
   how to install it, without installing it itself), runs the analysis to
   a private temporary file, explains the top actions in plain words,
   leads with usage limits for Pro and Max users, and offers the web page,
   `--since` and `setup --check`. Live nudges stay with `trimloop setup`,
   which the skill runs with `--write` only on the user's yes.
3. **No pre-approved commands.** The skill sets no `allowed-tools`: every
   command it runs goes through the user's normal permission prompts, since
   one of them can edit Claude Code's settings.
4. **Versions move together.** The plugin's `version` in `plugin.json`
   matches the trimloop release; a release bumps it, or users stay on the
   older skill.
5. **The plugin is under the repository's licence** (FSL-1.1-ALv2,
   [ADR 0046](0046-licensing.md)): it is part of trimloop, not a connector
   for other tools.

## Consequences

- The skill depends on the report's JSON field names; `trimloop-report/1`
  keeps them stable (removing or renaming one bumps the format), and the
  skill names only fields that format guarantees.
- While the repository is private, only people with access can add the
  marketplace (Claude Code uses their own git credentials).
- `claude plugin validate . --strict` checks the marketplace and plugin;
  run it when either changes.
