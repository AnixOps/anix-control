#!/usr/bin/env python3
"""Split a Go package list into balanced CI test shards.

Reads package import paths (one per line) on stdin and prints the packages of
one shard, in input order. Every input package lands in exactly one shard, so
a package added later is never left out; it only gets the default weight until
its measured time is added below.

The split is longest-processing-time first over the measured seconds per
package (WEIGHTS, from the `ok ... Ns` lines of recent CI runs) plus a fixed
per-package overhead for compiling and linking the test binary. It is
deterministic: the same input gives the same shards on every runner.

  go list ./internal/... | plan_test_shards.py --suite backend --shards 3 --shard 1
  plan_test_shards.py --suite postgres --shards 4 --summary < packages.txt

--merge-coverage OUTPUT INPUT... joins the coverage profiles of disjoint
shards into one: one mode line, then every block. It fails when the modes
differ or a block appears twice (the shards overlapped), so the merged
profile is what one `go test -coverprofile` run over all the packages writes.
"""

from __future__ import annotations

import argparse
import sys

MODULE = "github.com/AnixOps/anix-control/v4/"

# Seconds per package, the mean of five go_dev and pull request runs
# (2026-10-02) for backend ("Backend Tests" with coverage); postgres:
# "Package Storage PostgreSQL", see its comment. Keys are relative to
# MODULE.
WEIGHTS: dict[str, dict[str, float]] = {
    "backend": {
        "internal/moduleruntime": 31.3,
        "internal/service": 31.0,
        "internal/pluginhost": 29.0,
        "internal/tests/identitycompat": 21.1,
        "internal/handler": 17.0,
        "internal/tests/subscriptioncompat": 15.2,
        "internal/tests/ordercompat": 13.5,
        "internal/tests/paymentcompat": 13.4,
        "internal/kernelnodeops": 11.0,
        "internal/tests/affiliatecompat": 9.4,
        "internal/tests/notificationcompat": 7.9,
        "internal/grpc": 7.5,
        "internal/tests/gostmeshcompat": 6.1,
        "internal/tests/nodeopsagent": 6.0,
        "internal/nodesecrets": 3.1,
        "internal/router": 3.0,
        "internal/tests/plancompat": 2.9,
        "internal/tests/machinetelemetrycompat": 2.8,
        "internal/tests/proxynodecompat": 4.5,
        # Not measured in CI yet: M3-1's protocol and agent routes (local).
        "internal/tests/protocolruntimecompat": 3.0,
        "internal/tests/nodesecretsplit": 2.1,
        "internal/kernelidentity": 1.6,
        "internal/tests/platformcompat": 1.2,
        "internal/kernelsubscriber": 1.1,
    },
    "postgres": {
        # The mean of 12 runs of 2026-10-04 (11 pull request and go_dev runs,
        # then the first run with the job's PostgreSQL on fsync off).
        "internal/tests/proxynodecompat": 44.5,
        "internal/tests/identitycompat": 42.3,
        "internal/kernelnodeops": 39.7,
        "internal/tests/subscriptioncompat": 37.9,
        "internal/tests/nodeopsagent": 30.0,
        "internal/tests/integration": 28.1,
        "internal/tests/ordercompat": 27.1,
        "internal/tests/protocolruntimecompat": 26.4,
        "internal/tests/paymentcompat": 24.6,
        "internal/tests/affiliatecompat": 19.8,
        "internal/tests/notificationcompat": 16.4,
        "internal/tests/gostmeshcompat": 11.7,
        "internal/nodesecrets": 10.4,
        "internal/tests/nodesecretsplit": 10.3,
        "internal/tests/machinetelemetrycompat": 6.1,
        "internal/tests/plancompat": 5.3,
        "internal/packagestore": 4.4,
        "internal/kernelforward": 4.3,
        "internal/agentpki": 2.3,
        # Not measured in CI yet: the revocation race test (about half a second).
        "internal/modulepki": 1.5,
        "internal/tests/platformcompat": 2.3,
        "internal/tests/bridgecontract": 1.7,
        "internal/tests/ticketcompat": 1.2,
        "internal/subscriber": 1.1,
        "internal/agentupgrade": 0.7,
        "internal/tests/knowledgecompat": 0.6,
        "internal/tests/packagecompat": 0.6,
        "internal/tests/forwardv4": 0.5,
        "internal/shadowsamples": 0.3,
        "internal/agentreports": 0.1,
        "internal/tests/wireguardcompat": 0.1,
    },
}

# Compiling and linking one test binary, and the default for a package with
# no measurement.
OVERHEAD = 1.5
DEFAULT = 1.0


def normalize(package: str) -> str:
    package = package.strip()
    if package.startswith(MODULE):
        return package[len(MODULE) :]
    return package.removeprefix("./")


def plan(packages: list[str], suite: str, shards: int) -> list[list[str]]:
    """Return the packages of each shard, each in input order."""
    if shards < 1:
        raise ValueError("shards must be at least 1")
    if len(set(packages)) != len(packages):
        raise ValueError("duplicate packages in the input")
    weights = WEIGHTS[suite]
    cost = {package: weights.get(normalize(package), DEFAULT) + OVERHEAD for package in packages}
    loads = [0.0] * shards
    owner: dict[str, int] = {}
    for package in sorted(packages, key=lambda name: (-cost[name], name)):
        shard = min(range(shards), key=lambda index: (loads[index], index))
        owner[package] = shard
        loads[shard] += cost[package]
    return [[package for package in packages if owner[package] == index] for index in range(shards)]


def loads(packages: list[str], suite: str, shards: int) -> list[float]:
    weights = WEIGHTS[suite]
    return [
        round(sum(weights.get(normalize(package), DEFAULT) + OVERHEAD for package in shard), 1)
        for shard in plan(packages, suite, shards)
    ]


def merge_coverage(inputs: list[str]) -> str:
    """Merge Go coverage profiles of disjoint package sets."""
    mode = None
    seen: set[str] = set()
    blocks: list[str] = []
    for path in inputs:
        with open(path, encoding="utf-8") as handle:
            lines = handle.read().splitlines()
        if not lines or not lines[0].startswith("mode: "):
            raise ValueError(f"{path}: not a coverage profile")
        if mode is None:
            mode = lines[0]
        elif lines[0] != mode:
            raise ValueError(f"{path}: {lines[0]!r} differs from {mode!r}")
        for line in lines[1:]:
            if not line.strip():
                continue
            # file.go:start.col,end.col statements count
            key = line.rsplit(" ", 2)[0]
            if key in seen:
                raise ValueError(f"{path}: block {key} is in more than one profile")
            seen.add(key)
            blocks.append(line)
    if mode is None:
        raise ValueError("no coverage profiles")
    return "\n".join([mode, *blocks]) + "\n"


def self_test() -> None:
    backend = [MODULE + name for name in WEIGHTS["backend"]] + [MODULE + "internal/new-package"]
    for suite, packages in (("backend", backend), ("postgres", ["./" + name for name in WEIGHTS["postgres"]])):
        for shards in (1, 2, 3, 4, 7):
            split = plan(packages, suite, shards)
            flat = [package for shard in split for package in shard]
            assert sorted(flat) == sorted(packages), f"{suite}/{shards}: packages lost or duplicated"
            assert len(split) == shards
            assert plan(list(packages), suite, shards) == split, "the split must be deterministic"
            for shard in split:
                assert shard == [package for package in packages if package in shard], "input order must be kept"
    # Balanced: no shard above the mean by more than the heaviest package.
    for suite, shards in (("backend", 3), ("postgres", 4)):
        packages = list(WEIGHTS[suite])
        load = loads(packages, suite, shards)
        heaviest = max(WEIGHTS[suite].values()) + OVERHEAD
        assert max(load) - sum(load) / shards <= heaviest, (suite, load)
    # More shards than packages leaves the extra shards empty.
    assert plan(["a", "b"], "backend", 3)[2] == []
    try:
        plan(["a", "a"], "backend", 2)
    except ValueError:
        pass
    else:
        raise AssertionError("duplicates must be rejected")
    import os
    import tempfile

    with tempfile.TemporaryDirectory() as tmp:
        def profile(name: str, text: str) -> str:
            path = os.path.join(tmp, name)
            with open(path, "w", encoding="utf-8") as handle:
                handle.write(text)
            return path

        a = profile("a.out", "mode: atomic\nm/a/a.go:1.1,2.2 1 3\nm/a/a.go:3.1,4.2 2 0\n")
        b = profile("b.out", "mode: atomic\nm/b/b.go:1.1,2.2 4 1\n")
        merged = merge_coverage([a, b])
        assert merged == "mode: atomic\nm/a/a.go:1.1,2.2 1 3\nm/a/a.go:3.1,4.2 2 0\nm/b/b.go:1.1,2.2 4 1\n", merged
        for bad in (
            [a, profile("set.out", "mode: set\nm/c/c.go:1.1,2.2 1 1\n")],
            [a, profile("dup.out", "mode: atomic\nm/a/a.go:1.1,2.2 1 5\n")],
            [profile("empty.out", "")],
            [],
        ):
            try:
                merge_coverage(bad)
            except ValueError:
                continue
            raise AssertionError(f"merge must fail: {bad}")
    print("plan_test_shards self-test passed")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--suite", choices=sorted(WEIGHTS))
    parser.add_argument("--shards", type=int)
    parser.add_argument("--shard", type=int, help="1-based shard to print")
    parser.add_argument("--summary", action="store_true", help="print every shard and its estimated seconds")
    parser.add_argument("--merge-coverage", nargs="+", metavar="PATH", help="OUTPUT INPUT...: merge shard coverage profiles")
    parser.add_argument("--self-test", action="store_true")
    args = parser.parse_args()
    if args.self_test:
        self_test()
        return 0
    if args.merge_coverage:
        if len(args.merge_coverage) < 2:
            parser.error("--merge-coverage needs OUTPUT and at least one INPUT")
        output, *inputs = args.merge_coverage
        try:
            merged = merge_coverage(inputs)
        except ValueError as error:
            print(f"error: {error}", file=sys.stderr)
            return 1
        with open(output, "w", encoding="utf-8") as handle:
            handle.write(merged)
        print(f"merged {len(inputs)} profiles, {merged.count(chr(10)) - 1} blocks, into {output}", file=sys.stderr)
        return 0
    if not args.suite or not args.shards or (args.shard is None and not args.summary):
        parser.error("--suite, --shards and --shard (or --summary) are required")
    packages = [line.strip() for line in sys.stdin if line.strip()]
    split = plan(packages, args.suite, args.shards)
    if args.summary:
        for index, (shard, load) in enumerate(zip(split, loads(packages, args.suite, args.shards)), start=1):
            print(f"shard {index}/{args.shards}: {len(shard)} packages, about {load} s")
            for package in shard:
                print(f"  {package}")
        return 0
    if not 1 <= args.shard <= args.shards:
        parser.error("--shard must be between 1 and --shards")
    for package in split[args.shard - 1]:
        print(package)
    return 0


if __name__ == "__main__":
    sys.exit(main())
