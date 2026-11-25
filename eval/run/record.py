"""JSON plumbing for the eval/run scripts, which are POSIX sh.

    record.py get <member>          print a member of the JSON document on stdin;
                                    a list prints one item per line
    record.py governance-kinds      the kinds of the open governance escalations
                                    in the metron status --json document on stdin
    record.py model                 the model that omp's JSON events on stdin report
    record.py diff <seed> <head>    diff.json from git diff --numstat -z on stdin
    record.py run <trial-dir> <case> <arm> <trial> <started> <ended> <seconds> <end-state> <harness>
                                    run.json from <trial-dir>/steps.tsv and steps/NN.jsonl
    record.py verdict <file> <out>  check an unrestricted agent's VERDICT.json, write its
                                    normalized form to <out>, print "ready" or "not-ready"
    record.py review <file> <out>   the same for a bounded reviewer's REVIEW.json; prints
                                    "approve" or "request_changes"
    record.py baseline <trial-dir> <arm> <final>
                                    baseline.json from <trial-dir>/rounds/

verdict and review exit 1 when the file is missing or does not have the
shape the prompt asked for.
"""

from __future__ import annotations

import glob
import json
import os
import sys


def emit(doc) -> None:
    print(json.dumps(doc, indent=2, ensure_ascii=False))


def get(member: str) -> None:
    value = json.load(sys.stdin).get(member)
    if isinstance(value, list):
        for item in value:
            print(item)
    elif value is not None:
        print(value)


def governance_kinds() -> None:
    status = json.load(sys.stdin)
    kinds = sorted({e["kind"] for e in status.get("open_escalations") or [] if e.get("owner") == "governance"})
    print(",".join(kinds))


def assistant_messages(lines) -> list[dict]:
    """Return the assistant messages of an omp JSON event stream, one event per line."""
    messages = []
    for line in lines:
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        message = event.get("message") if isinstance(event, dict) else None
        if event.get("type") == "message_end" and isinstance(message, dict) and message.get("role") == "assistant":
            messages.append(message)
    return messages


def models(messages: list[dict]) -> str:
    return ",".join(sorted({m["model"] for m in messages if m.get("model")}))


def model() -> None:
    print(models(assistant_messages(sys.stdin)))


def diff(seed: str, head: str) -> None:
    files = []
    for record in sys.stdin.buffer.read().decode("utf-8", "surrogateescape").split("\0"):
        if not record:
            continue
        added, deleted, path = record.split("\t", 2)
        files.append({
            "path": path,
            "added": int(added) if added != "-" else 0,
            "deleted": int(deleted) if deleted != "-" else 0,
        })
    files.sort(key=lambda f: f["path"])
    emit({
        "seed": seed,
        "head": head,
        "files": files,
        "added": sum(f["added"] for f in files),
        "deleted": sum(f["deleted"] for f in files),
    })


def run(trial_dir, case, arm, trial, started, ended, seconds, end_state, harness) -> None:
    steps = []
    with open(os.path.join(trial_dir, "steps.tsv"), encoding="utf-8") as handle:
        for line in handle:
            n, kind, role, step_seconds, code = line.rstrip("\n").split("\t")
            events = os.path.join(trial_dir, "steps", "%02d.jsonl" % int(n))
            messages = []
            if os.path.exists(events):
                with open(events, encoding="utf-8", errors="replace") as stream:
                    messages = assistant_messages(stream)
            usage = [m.get("usage") or {} for m in messages]
            steps.append({
                "n": int(n),
                "kind": kind,
                "role": role,
                "seconds": int(step_seconds),
                "exit": int(code),
                "model": models(messages),
                # Every prompt token, whether the provider served it from its cache or not.
                "input_tokens": sum(u.get("input", 0) + u.get("cacheRead", 0) + u.get("cacheWrite", 0) for u in usage),
                "output_tokens": sum(u.get("output", 0) for u in usage),
            })
    emit({
        "case": case,
        "arm": arm,
        "trial": trial,
        "started": started,
        "ended": ended,
        "seconds": int(seconds),
        "end_state": end_state,
        "steps": steps,
        "harness": harness,
    })


def string_list(value) -> bool:
    return isinstance(value, list) and all(isinstance(v, str) for v in value)


def findings(value) -> list[dict]:
    """Return the findings in the shape the prompts ask for, or raise ValueError."""
    if not isinstance(value, list):
        raise ValueError("findings is not a list")
    out = []
    for f in value:
        if not (isinstance(f, dict) and isinstance(f.get("title"), str) and f.get("severity") in ("blocker", "non-blocker")
                and string_list(f.get("files")) and isinstance(f.get("description"), str)):
            raise ValueError("a finding lacks title, severity, files or description, or has the wrong type")
        out.append({k: f[k] for k in ("title", "severity", "files", "description")})
    return out


def load(path: str) -> dict:
    with open(path, encoding="utf-8") as handle:
        doc = json.load(handle)
    if not isinstance(doc, dict):
        raise ValueError("not a JSON object")
    return doc


def verdict(path: str, out: str) -> None:
    doc = load(path)
    if not isinstance(doc.get("ready"), bool) or not isinstance(doc.get("summary"), str):
        raise ValueError("ready is not a boolean or summary is not a string")
    normalized = {"ready": doc["ready"], "summary": doc["summary"], "findings": findings(doc.get("findings"))}
    with open(out, "w", encoding="utf-8") as handle:
        json.dump(normalized, handle, indent=2, ensure_ascii=False)
    print("ready" if doc["ready"] else "not-ready")


def review(path: str, out: str) -> None:
    doc = load(path)
    if doc.get("verdict") not in ("approve", "request_changes"):
        raise ValueError("verdict is neither approve nor request_changes")
    normalized = {"verdict": doc["verdict"], "findings": findings(doc.get("findings"))}
    with open(out, "w", encoding="utf-8") as handle:
        json.dump(normalized, handle, indent=2, ensure_ascii=False)
    print(doc["verdict"])


def baseline(trial_dir: str, arm: str, final: str) -> None:
    rounds = []
    paths = glob.glob(os.path.join(trial_dir, "rounds", "*.review.json"))
    for path in sorted(paths, key=lambda p: int(os.path.basename(p).split(".")[0])):
        entry = {"review": load(path)}
        fixed = path[: -len("review.json")] + "fixed"
        if os.path.exists(fixed):
            with open(fixed, encoding="utf-8") as handle:
                entry["fixed"] = handle.read().strip() == "true"
        rounds.append(entry)
    emit({"arm": arm, "rounds": rounds, "final": final})


def main(argv: list[str]) -> None:
    commands = {
        "get": (get, 1), "governance-kinds": (governance_kinds, 0), "model": (model, 0),
        "diff": (diff, 2), "run": (run, 9), "verdict": (verdict, 2), "review": (review, 2),
        "baseline": (baseline, 3),
    }
    if len(argv) < 2 or argv[1] not in commands or len(argv) - 2 != commands[argv[1]][1]:
        sys.exit(__doc__)
    function, _ = commands[argv[1]]
    try:
        function(*argv[2:])
    except (OSError, ValueError) as err:
        sys.exit("record.py %s: %s" % (argv[1], err))


if __name__ == "__main__":
    main(sys.argv)
