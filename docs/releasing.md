# Releasing

Dive is one Go module plus nine tagged sub-modules. Everything below assumes
you are on an up-to-date `main` and have `gh` authenticated.

## 1. Cut the changelog

Rename `## [Unreleased]` to the version being released and leave a fresh empty
`## [Unreleased]` above it:

```markdown
## [Unreleased]

## [1.20.0] - 2026-08-09
```

Before moving on, confirm every merged PR since the last release has an entry.
`git log v<previous>..HEAD --oneline` is the list to check against.

Keep each changelog entry to one to three source lines. State the user-visible
change rather than listing every command, flag, or implementation detail; put
that detail in feature documentation and use Highlights for upgrade context.

## 2. Sync the module requirements

```sh
make release-prep VERSION=v1.20.0
```

Every sub-module builds locally through a `replace` directive, so a stale
`github.com/deepnoodle-ai/dive vX.Y.Z` requirement is invisible to CI but breaks
anyone who resolves the tag. This rewrites them all, including the untagged
`demos/` modules.

## 3. Draft the release notes

```sh
make release-notes VERSION=v1.20.0
```

This writes `docs/releases/v1.20.0.md` with three sections:

| Section         | Source                                                    |
| --------------- | --------------------------------------------------------- |
| `Highlights`    | You. A placeholder comment until you replace it.          |
| `Pull requests` | Commit subjects between the previous root tag and `HEAD`. |
| `Changelog`     | The version's `CHANGELOG.md` section, verbatim.           |

**Write the Highlights.** This is the part the changelog cannot give you: what
changed, who it affects, and what they need to do about it. Two to four short
paragraphs, no restating the bullets below it. An agent drafting this should
read the merged PR bodies (`gh pr view <n>`), not just the commit subjects — the
reasoning behind a change rarely survives into a one-line subject.

Re-running the command is safe. Highlights you have already written are carried
over and only the mechanical sections are refreshed, so you can regenerate after
a late PR merges.

## 4. Open the release PR

Commit the changelog, the `go.mod` updates, and `docs/releases/v1.20.0.md`
together as `Prepare v1.20.0 release`. The notes get reviewed alongside the code
they describe, and nothing is published until the tag goes up.

## 5. Tag

After the PR merges, from an updated `main`:

```sh
git tag v1.20.0
make tag-modules VERSION=v1.20.0
git push origin --tags
```

`tag-modules` re-runs the requirement check first and refuses to tag if anything
is stale, so a skipped step 2 stops here rather than shipping.

## 6. Publish the GitHub release

```sh
make release-publish VERSION=v1.20.0
```

This creates the release from `docs/releases/v1.20.0.md`. It refuses to publish
while the Highlights are still a placeholder, and `--verify-tag` means a mistyped
version fails instead of creating a release against a tag `gh` invents from the
default branch.

## Checking a draft

`make release-notes-test` covers the tooling. To see the assembled body without
publishing, read `docs/releases/<version>.md` — it is exactly what gets posted.
