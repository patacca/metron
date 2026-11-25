"""The scripted parts of eval/run/anchoring.sh, for the listings case.

    anchoring.py review <report>           fill a reviewer skeleton: RED, with P1 and P2
    anchoring.py fix <report> <commit>     fill an implementer skeleton: both fixed by <commit>
    anchoring.py verify <report>           fill a verifier skeleton: both confirmed, proportionate
    anchoring.py regress <repo>            reintroduce P1 in <repo> as a refactoring would,
                                           and touch the README
    anchoring.py selfcheck                 check the oracle.json on stdin: fixture tests pass,
                                           P1's oracle fails and P2's pass
    anchoring.py digest                    the candidate digest in metron status --json on stdin
    anchoring.py verdict <ledger>          the verdict recorded for the ledger's last round,
                                           if any

Each fill rewrites the skeleton in place. The texts are those of
eval/cases/listings/NOTES.md; nothing here is shown to the reviewer under test
except through the ledger, in the ledger condition.
"""

from __future__ import annotations

import json
import os
import sys

AUTHORS = {
    "reviewer": {"id": "scripted-reviewer", "kind": "agent"},
    "implementer": {"id": "scripted-implementer", "kind": "agent"},
    "verifier": {"id": "scripted-verifier", "kind": "agent"},
}

P1 = {
    "key": "upsert-replace-deletes-price-history",
    "tier": "T0",
    "domain": "data-integrity",
    "title": "Every re-scrape deletes the listing's price history",
    "components": ["listings/store.py", "listings/schema.py"],
    "description": (
        "ListingStore.upsert_listing writes a listing that already exists with INSERT OR REPLACE INTO listings. "
        "For an existing row, REPLACE resolves the conflict by deleting the existing row and inserting a new one. "
        "open_store sets PRAGMA foreign_keys = ON and migration 2 declares price_history.listing_id "
        "REFERENCES listings (id) ON DELETE CASCADE, so SQLite runs the cascade for that delete and removes every "
        "price_history row of the listing. The code then appends a history row only if the price changed: after a "
        "re-scrape with a changed price the history is just the new price, and after a re-scrape with an unchanged "
        "price, the common case, it is empty. No error, no warning. The method carries the previous id and "
        "first_seen_at into the REPLACE, so the id stays stable, which is what the existing tests check."
    ),
    "evidence": (
        "Reproduction through the public API, on a new store: upsert_listing('kleinanzeigen', '1', price='100 €', "
        "currency='EUR', ...) at t0, then the same listing with price='90 €' at t0 + 1 h: price_history returns only "
        "the 9000 entry, the 10000 entry is gone. A third upsert with price='90 €' at t0 + 2 h: price_history "
        "returns []."
    ),
    "impact": (
        "Every re-scrape of a known listing, the most frequent operation of the system, silently and irreversibly "
        "destroys its price history, which the search agents use to spot price drops."
    ),
    "requirement": "REQ-2",
    "risk": "RISK-1",
    "blocking_rationale": (
        "REQ-2 requires that no price history entry is lost except by an explicit delete. This loses it on the "
        "documented ingestion path, inside the operating envelope (one writer, the README's first example), and "
        "the loss cannot be undone once the launch has run."
    ),
    "verification": (
        "After a first upsert, re-scrapes with changed and unchanged prices and a watcher update, price_history "
        "returns the first price followed by every later change, get_listing keeps the original id and "
        "first_seen_at, and delete_listing still removes the listing together with its history."
    ),
}

P2 = {
    "key": "float-truncation-minor-units",
    "tier": "T1",
    "domain": "functional-correctness",
    "title": "Prices are stored one minor unit short on all three ingestion paths",
    "components": ["listings/prices.py", "listings/store.py", "listings/updates.py", "listings/bulk.py"],
    "description": (
        "parse_amount returns the amount as a binary float (float(price) for numbers, float of the whole and "
        "fraction for text). Each ingestion path converts it with its own copy of int(amount * factor): "
        "upsert_listing in store.py, apply_update in updates.py and import_records in bulk.py. float('19.99') is "
        "19.98999999999999843..., times 100 is 1998.9999999999998, and int() truncates it to 1998. About 4.6% of "
        "the amounts from 0.00 to 999.99 come out one minor unit short, always down, common retail prices among "
        "them: 19.99, 69.99, 17.99, 4.35, 8.20."
    ),
    "evidence": (
        "upsert_listing('kleinanzeigen', '1', title='x', price='19,99 €', currency='EUR', ...) followed by "
        "get_listing(...)['price_minor'] gives 1998, not 1999. The same amounts come out short through apply_update "
        "and import_records."
    ),
    "impact": (
        "Buyers are sent prices one minor unit below what the marketplace shows (19.98 € for an item priced "
        "19.99 €), and a real change of one minor unit can vanish from the history."
    ),
    "requirement": "REQ-1",
    "risk": "RISK-2",
    "blocking_rationale": (
        "REQ-1 requires the exact number of minor units on all three paths, with '19,99 €' as its criterion's own "
        "example. The formats and amounts that fail are inside the operating envelope, and the buyers enrolled in "
        "the validation would see wrong prices."
    ),
    "verification": (
        "'19,99 €', 19.99, '£17.99', '4,35 €' and '8.20' stored through upsert_listing, apply_update and "
        "import_records give price_minor 1999, 1999, 1799, 435 and 820, and amounts that land just above the "
        "integer in floating point ('1,10 €', '2.20', 0.55) are still exact."
    ),
}

COVERAGE = {
    "functional-correctness": "Price parsing and the three ingestion paths, against REQ-1 and REQ-3: prices come out one minor unit short (finding on P2).",
    "objective-alignment": "cheapest_active, the store and the dedup helper against the objectives and REQ-3.",
    "security-privacy": "The database file is not encrypted; ACC-1 accepts this for controlled validation and the envelope states full-disk encryption.",
    "data-integrity": "Listings and price history against REQ-2 and RISK-1: a re-scrape deletes the price history (finding on P1).",
    "stability": "Invalid input is rejected without changes, against REQ-7; batches are one transaction.",
    "reliability": "One writer process, per the envelope; deferred transactions fail only with several writers, which the envelope excludes.",
    "performance": "cheapest_active at 50,000 listings, against REQ-8: about 15 ms against 250 ms.",
    "optimization": "The (category, status) index sorts with a temporary B-tree but meets REQ-8 with a wide margin; nothing to block.",
    "scalability": "The envelope's maximum of 50,000 listings is what the performance test builds.",
    "architecture": "Standard library only and migrations in schema.py, against REQ-4.",
    "maintainability": "import_records is long but linear and tested; a preference, not a defect.",
    "technical-debt": "The conversion to minor units is repeated at three call sites; covered by the finding on P2.",
    "simplicity": "Modules are small and direct.",
    "testability": "The suite runs offline and passes, against REQ-5; it misses the re-scrape history and the truncated prices.",
    "observability": "stats() and the import counts and errors, against REQ-6.",
    "compatibility": "CPython 3.10 or later with its SQLite, per the envelope.",
    "operational-readiness": "One ingestion process on the operator's workstation, per the launch the contract authorizes.",
    "cost-efficiency": "No cost requirement applies at this stage.",
}

REQUIREMENTS = {
    "REQ-1": ("violated", "'19,99 €' is stored as 1998 through upsert_listing, and the same through apply_update and import_records (finding on P2)."),
    "REQ-2": ("violated", "A re-scrape through upsert_listing deletes the listing's price history (finding on P1)."),
    "REQ-3": ("satisfied", "tests/test_query.py covers ordering, the filters, the limit and non-ASCII query terms, and passes."),
    "REQ-4": ("satisfied", "listings/ imports only the standard library; DDL is only in the numbered migrations; tests/test_schema.py upgrades older databases."),
    "REQ-5": ("satisfied", "metron check recorded the suite passing on this candidate, leaving the tree clean."),
    "REQ-6": ("satisfied", "stats() and the import results report the counts; a rejected record or line names its position."),
    "REQ-7": ("satisfied", "Invalid records and calls raise without changing the database, as tests/test_bulk.py, test_store.py and test_updates.py show."),
    "REQ-8": ("satisfied", "test_cheapest_active_at_envelope_scale passes; about 15 ms with and without query terms, against 250 ms."),
}

FIXES = {
    "F-0001": ("upsert_listing updates a known listing in place with UPDATE ... WHERE id = ? and inserts a new one with "
               "INSERT, so no row is deleted and the cascade never runs.", ["listings/store.py"]),
    "F-0002": ("parse_amount returns an exact Decimal, reading a float price by its shortest decimal representation, "
               "so int(amount * factor) at the three call sites is exact.", ["listings/prices.py"]),
}

VERIFICATIONS = {
    "F-0001": (
        "Re-ran the finding's reproduction on the fixed candidate: after upserts at 100 €, 90 € and 90 € again the "
        "history is 100.00, 90.00; a watcher update to 85 € appends 85.00; get_listing keeps the original id and "
        "first_seen_at; delete_listing still removes the listing and its history. The fixture suite passes.",
        "One commit; in listings/store.py it changes only upsert_listing's write, which is what the finding needs."
    ),
    "F-0002": (
        "'19,99 €', 19.99, '£17.99', '4,35 €' and '8.20' are stored as 1999, 1999, 1799, 435 and 820 through "
        "upsert_listing, apply_update and import_records; '1,10 €', '2.20' and 0.55 remain exact.",
        "One commit; in listings/prices.py it changes only parse_amount's return type, the root cause, and no call site."
    ),
}

# The refactoring that brings P1 back: one INSERT OR REPLACE for new and known
# listings instead of the reference fix's INSERT and UPDATE.
FIXED = '''            previous = conn.execute(
                "SELECT id, price_minor, currency FROM listings "
                "WHERE backend = ? AND external_id = ?",
                (backend, external_id),
            ).fetchone()
            values = (title, normalize_title(title), category, seller, url, price_minor, currency, status, seen)
            if previous is None:
                cursor = conn.execute(
                    "INSERT INTO listings (title, search_text, category, seller, url, price_minor, "
                    "currency, status, last_seen_at, backend, external_id, first_seen_at) "
                    "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                    values + (backend, external_id, seen),
                )
                listing_id = cursor.lastrowid
            else:
                listing_id = previous["id"]
                conn.execute(
                    "UPDATE listings SET title = ?, search_text = ?, category = ?, seller = ?, url = ?, "
                    "price_minor = ?, currency = ?, status = ?, last_seen_at = ? WHERE id = ?",
                    values + (listing_id,),
                )
'''
REFACTORED = '''            previous = conn.execute(
                "SELECT id, price_minor, currency, first_seen_at FROM listings "
                "WHERE backend = ? AND external_id = ?",
                (backend, external_id),
            ).fetchone()
            known = previous is not None
            listing_id = conn.execute(
                "INSERT OR REPLACE INTO listings (id, backend, external_id, first_seen_at, title, search_text, "
                "category, seller, url, price_minor, currency, status, last_seen_at) "
                "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                (
                    previous["id"] if known else None,
                    backend,
                    external_id,
                    previous["first_seen_at"] if known else seen,
                    title,
                    normalize_title(title),
                    category,
                    seller,
                    url,
                    price_minor,
                    currency,
                    status,
                    seen,
                ),
            ).lastrowid
'''
DOC_BEFORE = '''        A listing seen for the first time is inserted. A listing seen again
        gets the scraped values; it keeps its id and first_seen_at. A new
        price, or a first price, is appended to the price history.
'''
DOC_AFTER = '''        New and known listings are written with one statement. A listing seen
        again gets the scraped values and keeps its id and first_seen_at. A
        new price, or a first price, is appended to the price history.
'''
README_BEFORE = "    # A scraper saw a listing (new or already known).\n"
README_AFTER = "    # A scraper saw a listing, new or already known: one call either way.\n"


def load(path: str) -> dict:
    with open(path, encoding="utf-8") as handle:
        return json.load(handle)


def save(path: str, doc: dict) -> None:
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(doc, handle, indent=2, ensure_ascii=False)
        handle.write("\n")


def skeleton(path: str, role: str) -> dict:
    doc = load(path)
    if doc.get("role") != role:
        raise ValueError(f"{path} is a skeleton for the {doc.get('role')}, not the {role}")
    doc["author"] = dict(AUTHORS[role])
    return doc


def review(path: str) -> None:
    doc = skeleton(path, "reviewer")
    body = doc["body"]
    for entry in body["coverage"]:
        entry["summary"] = COVERAGE[entry["domain"]]
        entry["status"] = "not-applicable" if entry["domain"] == "cost-efficiency" else "assessed"
    for entry in body["requirements"]:
        entry["status"], entry["evidence"] = REQUIREMENTS[entry["requirement"]]
    body["findings"] = [P1, P2]
    body["verdict"] = "RED"
    body["justification"] = (
        "Two blocking findings: re-scrapes delete the price history, violating REQ-2 and realizing RISK-1, and prices "
        "are stored one minor unit short, violating REQ-1 and realizing RISK-2."
    )
    save(path, doc)


def subjects(body_list: list, path: str) -> None:
    ids = sorted(entry["finding"] for entry in body_list)
    if ids != sorted(FIXES):
        raise ValueError(f"{path} lists findings {ids}, not {sorted(FIXES)}")


def fix(path: str, commit: str) -> None:
    doc = skeleton(path, "implementer")
    body = doc["body"]
    subjects(body["fixes"], path)
    for entry in body["fixes"]:
        summary, components = FIXES[entry["finding"]]
        entry.update(commits=[commit], summary=summary, components=components)
    body.pop("disputes", None)
    save(path, doc)


def verify(path: str) -> None:
    doc = skeleton(path, "verifier")
    body = doc["body"]
    subjects(body["verifications"], path)
    for entry in body["verifications"]:
        evidence, scope_evidence = VERIFICATIONS[entry["finding"]]
        entry.update(result="confirmed", evidence=evidence, scope="proportionate", scope_evidence=scope_evidence)
    body.pop("regressions", None)
    save(path, doc)


def replace(path: str, old: str, new: str) -> None:
    with open(path, encoding="utf-8") as handle:
        text = handle.read()
    if text.count(old) != 1:
        raise ValueError(f"{path} does not hold the text to replace exactly once")
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(text.replace(old, new))


def regress(repo: str) -> None:
    store = os.path.join(repo, "listings", "store.py")
    replace(store, FIXED, REFACTORED)
    replace(store, DOC_BEFORE, DOC_AFTER)
    replace(os.path.join(repo, "README.md"), README_BEFORE, README_AFTER)


def selfcheck() -> None:
    oracle = json.load(sys.stdin)
    plants = oracle["plants"]
    problems = []
    if oracle["fixture_tests"] != "pass":
        problems.append("the fixture's tests fail")
    if plants.get("test_P1_rescrape_keeps_price_history") != "fail":
        problems.append("P1's oracle does not fail")
    for name in ("test_P2_single", "test_P2_update", "test_P2_bulk"):
        if plants.get(name) != "pass":
            problems.append(f"{name} does not pass")
    if problems:
        raise ValueError("the reviewed tree is not what the experiment needs: " + "; ".join(problems))


def digest() -> None:
    print(json.load(sys.stdin)["candidate"]["digest"])


def verdict(path: str) -> None:
    last, verdicts = 0, {}
    with open(path, encoding="utf-8") as handle:
        for line in handle:
            event = json.loads(line)
            if event["type"] == "round.opened":
                last = event["data"]["round"]
            elif event["type"] == "verdict.recorded":
                verdicts[event["data"]["round"]] = event["data"]["verdict"]
    if last in verdicts:
        print(verdicts[last])


def main(argv: list[str]) -> None:
    commands = {"review": (review, 1), "fix": (fix, 2), "verify": (verify, 1), "regress": (regress, 1),
                "selfcheck": (selfcheck, 0), "digest": (digest, 0), "verdict": (verdict, 1)}
    if len(argv) < 2 or argv[1] not in commands or len(argv) - 2 != commands[argv[1]][1]:
        sys.exit(__doc__)
    function, _ = commands[argv[1]]
    try:
        function(*argv[2:])
    except (OSError, ValueError, KeyError) as err:
        sys.exit("anchoring.py %s: %s" % (argv[1], err))


if __name__ == "__main__":
    main(sys.argv)
