#!/usr/bin/env python3
"""Check that everyone who authored a commit in a pull request signed the CLA.

Rules (CLA.md, "How To Sign"):
  1. Every commit author must be a GitHub account (a commit whose email is not
     linked to an account cannot be matched to a signature) and must have a
     line in .github/cla-signatures.txt, unless exempt (the repository owner
     and "[bot]" accounts).
  2. A pull request may only add signature lines for its own author: nobody
     signs for someone else.
  3. A pull request never removes or changes an existing signature line.
  4. The pull request author must have signed too (or be exempt): unlike a
     commit author, which is only an email in the commit, the pull request
     author is an authenticated GitHub account.

Commits by the AI coding agent account ("claude", from noreply@anthropic.com)
are accepted only in pull requests opened by the repository owner: they are
the owner's work done with an AI tool, not a separate contributor's.

Usage:
  check_cla.py --author LOGIN --commits FILE --patch FILE --signatures FILE
    FILE --commits: one commit author per line, a GitHub login or
      "UNLINKED:<short sha>"; FILE --patch: the pull request's diff of
      .github/cla-signatures.txt (empty when untouched).
  check_cla.py --self-test
"""
import argparse
import sys

OWNER = "kalijerry"
AI_AGENTS = {"claude"}


def exempt(login):
    return login.lower() == OWNER or login.endswith("[bot]")


def signed_logins(lines):
    out = set()
    for line in lines:
        line = line.strip()
        if line and not line.startswith("#") and "|" in line:
            out.add(line.split("|", 1)[0].strip().lower())
    return out


def check(author, commit_authors, patch_lines, signature_lines):
    """Return a list of error strings; empty means the CLA check passes."""
    errors = []
    for line in patch_lines:
        if line.startswith(("+++", "---", "@@")):
            continue
        if line.startswith("-") and line[1:].strip() and not line[1:].lstrip().startswith("#"):
            errors.append(f"removes or changes an existing signature: {line[1:].strip()}")
        elif line.startswith("+"):
            added = signed_logins([line[1:]])
            for login in added:
                if login != author.lower():
                    errors.append(f"adds a signature for {login}, but only the pull request author ({author}) may sign in this pull request")
    signed = signed_logins(signature_lines)
    owner_pr = author.lower() == OWNER
    for login in sorted(set(commit_authors) | {author}):
        if owner_pr and login.lower() in AI_AGENTS:
            continue
        if login.startswith("UNLINKED:"):
            errors.append(f"commit {login[9:]} has an author email not linked to a GitHub account")
        elif not exempt(login) and login.lower() not in signed:
            errors.append(f"{login} has not signed the CLA (add your line to .github/cla-signatures.txt, see CLA.md)")
    return errors


def self_test():
    sig = ["# header", "alice | Alice A | 2026-11-22 | CLA 1.0"]
    cases = [
        ("owner alone", "kalijerry", ["kalijerry"], [], sig, 0),
        ("bot alone", "dependabot[bot]", ["dependabot[bot]"], [], sig, 0),
        ("signed contributor", "alice", ["alice"], [], sig, 0),
        ("case-insensitive login", "Alice", ["Alice"], [], sig, 0),
        ("unsigned contributor", "bob", ["bob"], [], sig, 1),
        ("signs in this PR", "bob", ["bob"], ["+bob | Bob B | 2026-11-22 | CLA 1.0"], sig + ["bob | Bob B | 2026-11-22 | CLA 1.0"], 0),
        ("signs for someone else", "bob", ["bob"], ["+carol | C | 2026-11-22 | CLA 1.0", "+bob | B | 2026-11-22 | CLA 1.0"], sig + ["carol | C | 2026-11-22 | CLA 1.0", "bob | B | 2026-11-22 | CLA 1.0"], 1),
        ("removes a signature", "bob", ["kalijerry"], ["-alice | Alice A | 2026-11-22 | CLA 1.0"], ["# header"], 1),
        ("unsigned co-author commit", "alice", ["alice", "dave"], [], sig, 1),
        ("unlinked commit email", "alice", ["alice", "UNLINKED:abc1234"], [], sig, 1),
        ("comment-only patch", "alice", ["alice"], ["+# note"], sig, 0),
        ("AI agent commits in an owner PR", "kalijerry", ["claude"], [], sig, 0),
        ("AI agent commits in a contributor PR", "alice", ["alice", "claude"], [], sig, 1),
        ("unsigned PR author with signed commits", "bob", ["alice"], [], sig, 1),
    ]
    failed = 0
    for name, author, commits, patch, sigs, want in cases:
        got = len(check(author, commits, patch, sigs))
        if (got > 0) != (want > 0):
            failed += 1
            print(f"self-test FAIL: {name}: {got} errors", file=sys.stderr)
    print(f"check_cla self-test: {len(cases) - failed}/{len(cases)} passed")
    return 1 if failed else 0


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--self-test", action="store_true")
    ap.add_argument("--author")
    ap.add_argument("--commits")
    ap.add_argument("--patch")
    ap.add_argument("--signatures")
    args = ap.parse_args()
    if args.self_test:
        return self_test()

    def read(path):
        with open(path, encoding="utf-8") as f:
            return f.read().splitlines()

    errors = check(args.author, [c for c in read(args.commits) if c.strip()], read(args.patch), read(args.signatures))
    for e in errors:
        print(f"::error::{e}")
    if not errors:
        print("CLA: every commit author has signed or is exempt.")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
