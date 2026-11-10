# Security

## Reporting a problem

Please report security problems privately, not in an issue or a
discussion: on the repository's **Security** tab, choose **Report a
vulnerability**
([open the form](https://github.com/ChrisAdkin8/trimloop/security/advisories/new);
[GitHub's guide](https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/report-privately)).

Say what you found, how to reproduce it, and the output of
`trimloop --version`. Trimloop is an early prototype kept by one person,
so there is no promised reply time; reports are read and answered as soon
as possible.

## Which versions get fixes

The latest release only.

## What trimloop touches

Useful to know when judging a report:

- It reads Claude Code's session logs (`~/.claude/projects`, or `projects`
  under `CLAUDE_CONFIG_DIR`) and, of Claude Code's account file, the plan
  fields only. It sends nothing anywhere.
- It writes the report you ask for, and files under `~/.trimloop`, readable
  by you only. The README's "Files it writes" lists each one.
- `trimloop setup --write` changes Claude Code's settings file, after saving
  a backup. Claude Code then runs `trimloop statusline` and `trimloop hook`
  as you work.
- Logs hold text from the model and from tools, so trimloop treats log
  content as untrusted when it prints it or puts it in a web page.

## Checking a download

Release checksums are signed by the release workflow. The README's
"Checking a download" says how to verify them.
