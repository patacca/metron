"""Judge one tree of a case: its own test suite and the case's hidden oracle.

    python3 oracle.py <case-dir> <tree>

<case-dir> is the case that holds the seed repository, the base of a variant.
Prints the trial's oracle.json:

    {"fixture_tests": "pass" | "fail",
     "plants": {"<test method>": "pass" | "fail" | "error", ...},
     "decoys": {...}}

The fixture's suite is the case's test_command (truth.json), run in the tree.
The oracle modules are <case-dir>/oracle/test_plants.py and test_decoys.py,
each run in its own process with LISTINGS_REPO set to the tree. A module
that cannot be imported, crashes or runs out of time marks every one of its
test methods "error": nothing about the tree is assumed to pass.
"""

from __future__ import annotations

import ast
import importlib.util
import json
import os
import subprocess
import sys
import unittest

TIMEOUT_SECONDS = 900
MODULES = (("plants", "test_plants.py"), ("decoys", "test_decoys.py"))
RANK = {"pass": 0, "fail": 1, "error": 2}


def test_methods(path: str) -> list[str]:
    """Return the test methods the module defines, read from its source."""
    with open(path, encoding="utf-8") as handle:
        tree = ast.parse(handle.read(), path)
    return [
        item.name
        for node in tree.body
        if isinstance(node, ast.ClassDef)
        for item in node.body
        if isinstance(item, ast.FunctionDef) and item.name.startswith("test")
    ]


class Recorder(unittest.TestResult):
    """Keeps the worst outcome of each test method, its subtests included."""

    def __init__(self) -> None:
        super().__init__()
        self.outcomes: dict[str, str] = {}

    def note(self, test: unittest.TestCase, outcome: str) -> None:
        name = getattr(test, "_testMethodName", None) or test.id()
        if RANK[outcome] > RANK.get(self.outcomes.get(name, ""), -1):
            self.outcomes[name] = outcome

    def addSuccess(self, test):
        self.note(test, "pass")

    def addFailure(self, test, err):
        self.note(test, "fail")

    def addError(self, test, err):
        self.note(test, "error")

    def addSkip(self, test, reason):
        self.note(test, "error")

    def addExpectedFailure(self, test, err):
        self.note(test, "fail")

    def addUnexpectedSuccess(self, test):
        self.note(test, "fail")

    def addSubTest(self, test, subtest, err):
        if err is not None:
            self.note(test, "fail" if issubclass(err[0], test.failureException) else "error")


def child(path: str) -> None:
    """Run one oracle module in this process and print its outcomes."""
    spec = importlib.util.spec_from_file_location(os.path.splitext(os.path.basename(path))[0], path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    result = Recorder()
    unittest.defaultTestLoader.loadTestsFromModule(module).run(result)
    print(json.dumps(result.outcomes))


def run_module(path: str, tree: str) -> dict[str, str]:
    outcomes = {name: "error" for name in test_methods(path)}
    env = dict(os.environ, LISTINGS_REPO=tree)
    try:
        done = subprocess.run(
            [sys.executable, "-I", "-B", os.path.abspath(__file__), "--child", path],
            env=env, cwd=tree, capture_output=True, text=True, timeout=TIMEOUT_SECONDS,
        )
        reported = json.loads(done.stdout) if done.returncode == 0 else {}
    except (subprocess.TimeoutExpired, json.JSONDecodeError):
        reported = {}
    for name, outcome in reported.items():
        outcomes[name] = outcome
    return outcomes


def fixture_tests(case_dir: str, tree: str) -> str:
    with open(os.path.join(case_dir, "truth.json"), encoding="utf-8") as handle:
        command = json.load(handle)["test_command"]
    env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1")
    try:
        done = subprocess.run(["sh", "-c", command], env=env, cwd=tree, capture_output=True, timeout=TIMEOUT_SECONDS)
    except subprocess.TimeoutExpired:
        return "fail"
    return "pass" if done.returncode == 0 else "fail"


def main(argv: list[str]) -> None:
    if len(argv) == 3 and argv[1] == "--child":
        child(argv[2])
        return
    if len(argv) != 3:
        sys.exit("usage: oracle.py <case-dir> <tree>")
    case_dir, tree = (os.path.abspath(a) for a in argv[1:])
    doc = {"fixture_tests": fixture_tests(case_dir, tree)}
    for key, name in MODULES:
        doc[key] = run_module(os.path.join(case_dir, "oracle", name), tree)
    print(json.dumps(doc, indent=2, sort_keys=True))


if __name__ == "__main__":
    main(sys.argv)
