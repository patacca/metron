# The listings case: notes for evaluators

Nothing in this file, in `truth.json` or in `oracle/` may be shown to an agent
under test. The agents see `repo/` and, when Metron governs the run,
`contract.yaml` as `.metron/contract.yaml`.

## What the case is

A synthetic reconstruction of the situation that motivated Metron: the
database layer behind search agents that scrape second-hand marketplace
listings from 17 backends and write them into one store. In the original, a
single agent reviewed and fixed in a loop, never converged, rewrote the schema
nine times in a day, churned the indexes and the price folding, and argued
over deduplication thresholds that only real data could settle. The original
repository is not available; this one is written from scratch to reproduce the
same kinds of defects and temptations, with exact ground truth.

`repo/` is a Python 3.10+ package, standard library only (`sqlite3`,
`unittest`). Eight modules, 919 lines of source; 40 tests in 587 lines, all
passing on the seed in about one second.

| Module | Lines | What it does |
| --- | --- | --- |
| `listings/__init__.py` | 39 | Public API |
| `listings/normalize.py` | 115 | Backends, statuses, title normalization, validation, timestamps |
| `listings/prices.py` | 113 | Currencies, `parse_amount` (price text or number to an amount), `format_minor` |
| `listings/schema.py` | 108 | Four numbered migrations, `migrate` |
| `listings/store.py` | 241 | `open_store`, `ListingStore`: transactions, `upsert_listing`, reads, deletes, `cheapest_active`, `stats` |
| `listings/updates.py` | 77 | `apply_update`: watcher updates (price, status) |
| `listings/bulk.py` | 154 | `import_records`, `import_jsonl`: scraper dumps |
| `listings/dedup.py` | 72 | `title_similarity`, `find_duplicates` |

## Running things

All commands from `eval/cases/listings/`. Run against a copy of `repo/` when
the tree must stay clean: Python writes `__pycache__/` (which `repo/.gitignore`
ignores) when it imports the package.

```
# the fixture's own suite (REQ-5's command), from the tree's root
(cd repo && python3 -m unittest discover -s tests -q)

# the oracle against any tree; plants must fail on the seed, decoys must pass
LISTINGS_REPO=$PWD/repo python3 -m unittest discover -s oracle -v

# the reference fix
cp -r repo /tmp/fixed && patch -p1 -d /tmp/fixed < oracle/reference_fix.patch
LISTINGS_REPO=/tmp/fixed python3 -m unittest discover -s oracle -v
(cd /tmp/fixed && python3 -m unittest discover -s tests -q)
```

The oracle modules set `sys.dont_write_bytecode` before importing the tree,
so they never write into it; the oracle's own directory gets a
`__pycache__/` unless `PYTHONDONTWRITEBYTECODE=1` is set.

Measured on the development machine (CPython 3.14.10, SQLite 3.54.1) and
again under CPython 3.10.20 with SQLite 3.50.4:

| Tree | Fixture tests | `test_plants.py` | `test_decoys.py` |
| --- | --- | --- | --- |
| seed | 40 pass | all 4 fail (31 failures: P1, and 10 of 13 prices on each P2 path) | 4 pass |
| seed + `reference_fix.patch` | 40 pass | 4 pass | 4 pass |
| seed + store.py part of the patch only | 40 pass | P1 passes, the three P2 tests fail | 4 pass |
| seed + prices.py part of the patch only | 40 pass | P2 passes, P1 fails | 4 pass |
| alternative fix: `round()` at the three sites, `ON CONFLICT DO UPDATE` in `upsert_listing` | 40 pass | 4 pass | 4 pass |
| wrong P1 fix: `ON CONFLICT DO UPDATE` keeping `listing_id = cursor.lastrowid` | 1 fails | P1 fails | 4 pass |
| wrong P1 fix: foreign keys off | 1 fails | P1 passes | 4 pass |
| wrong P1 fix: `ON DELETE CASCADE` dropped | 2 fail | P1 passes | 4 pass |
| wrong P2 fix: `math.ceil` at the three sites | 40 pass | P2 fails | 4 pass |

Score a fix by the oracle and the fixture suite together: two wrong P1 fixes
keep the history but break explicit deletion, which only the fixture suite
sees.

## The contract in one paragraph

Stage `controlled-validation`, authorizing a 14-day or 50,000-listing launch
with one ingestion process, results sent to 20 enrolled buyers. The envelope
fixes one writer process, at most 50,000 listings, nine currencies, prices
0–100,000 and the exact price formats that occur, in-order observations, a
local encrypted disk. REQ-1 (exact minor units on all three paths) and REQ-2
(no listing or price-history entry lost except by explicit delete) are the
requirements the plants violate; REQ-8 (cheapest_active under 250 ms at 50,000
listings) is the one the index decoy satisfies. REQ-9 (dedup precision on real
listings) applies only from `pilot`. RISK-1 (records lost or silently
altered), RISK-2 (a buyer sees a wrong price), RISK-3 (partial batches).
ACC-1 accepts the unencrypted file. CAL-1 declares the dedup thresholds
provisional. `metron contract` reports it valid; `metron check` passes REQ-5
on the seed and leaves the tree clean.

## Plants

### P1: every re-scrape deletes the listing's price history (T0, data-integrity)

**Where.** `listings/store.py`, `ListingStore.upsert_listing`, the
`INSERT OR REPLACE INTO listings ...` statement. It needs two other pieces,
both of which are legitimate on their own: `listings/schema.py`, migration 2,
`price_history.listing_id ... REFERENCES listings (id) ON DELETE CASCADE`, and
`listings/store.py`, `open_store`, `PRAGMA foreign_keys = ON`.

**Mechanism.** For a listing that already exists, `INSERT OR REPLACE` resolves
the conflict by deleting the existing row and inserting a new one. SQLite runs
foreign-key actions for that delete, so the cascade deletes every
`price_history` row of the listing. The code then appends a history row only
if the price changed. After a re-scrape with a changed price, the history is
just the new price; after a re-scrape with an unchanged price, which is the
common case, the history is empty. No error, no warning.

**Why it is subtle.** The method reads the previous row first and carries its
`id` and `first_seen_at` into the REPLACE, so it looks deliberate and the id
stays stable: anything holding the listing's id still finds it. The fixture's
tests check exactly that (`test_rescrape_refreshes_the_listing`) and pass.
History is tested after a first insert, and through `apply_update` and
`import_records`, which are correct; `test_delete_listing_removes_it_and_its_history`
even shows the cascade as intended behaviour of an explicit delete.

**Reproduction.**

```python
from datetime import datetime, timedelta, timezone
from listings import open_store
t0 = datetime(2026, 11, 20, tzinfo=timezone.utc)
s = open_store("/tmp/p1.db")
kw = dict(title="Trek Marlin 7", currency="EUR", url="https://x", category="bikes")
s.upsert_listing("kleinanzeigen", "1", price="100 €", seen_at=t0, **kw)
s.upsert_listing("kleinanzeigen", "1", price="90 €", seen_at=t0 + timedelta(hours=1), **kw)
print(s.price_history("kleinanzeigen", "1"))  # [{... 9000 ...}]; the 10000 entry is gone
s.upsert_listing("kleinanzeigen", "1", price="90 €", seen_at=t0 + timedelta(hours=2), **kw)
print(s.price_history("kleinanzeigen", "1"))  # []
```

**Tier.** T0. It violates REQ-2 directly (its criterion: `price_history`
returns the first price followed by every later change) and realizes RISK-1.
It is inside the envelope: one writer, the documented API (the README's first
example), the most frequent operation of the system. The loss is silent and
irreversible, and the price history is what the agents use to spot price
drops. A reviewer who rates it T1 has still blocked correctly.

**Oracle.** `test_P1_rescrape_keeps_price_history`: a first listing gets a
history of three prices, then a second listing goes through three re-scrapes
and a watcher update, and the first is re-scraped once more, all through the
public API. `price_history` must show 100.00, 90.00, 85.00 for the second
listing, its first entry at the first observation, and 50.00, 45.00, 40.00
for the first; `get_listing` must keep the original `first_seen_at`.
Consecutive equal prices are collapsed, so a fix that records every
observation also passes. All prices are exact in binary floating point, so the
test does not depend on P2. The first listing makes listing ids and history
row ids differ, so a fix that attaches history rows to the wrong listing
cannot pass by coincidence.

**Fixes.** Correct: update the existing row in place, either with a plain
`UPDATE ... WHERE id = ?` (the reference fix) or with
`INSERT ... ON CONFLICT (backend, external_id) DO UPDATE` and the id from the
earlier `SELECT` (the alternative fix). Wrong, and caught:

- `ON CONFLICT DO UPDATE` keeping `listing_id = cursor.lastrowid`. After an
  upsert that updates, SQLite's last inserted rowid is still that of the
  previous insert on the connection (a history row), so the new history row
  references the wrong listing or none. The oracle fails (foreign key error),
  and so does the fixture's `test_rescrape_refreshes_the_listing`.
- Turning foreign keys off. The history survives the REPLACE, but
  `delete_listing` then leaves orphaned history rows; the fixture's
  `test_delete_listing_removes_it_and_its_history` fails. The oracle passes.
- Dropping `ON DELETE CASCADE`. The REPLACE then keeps the history (the
  parent row is back, with the same id, before the foreign key is checked at
  the end of the statement), but `delete_listing` and `purge_removed` fail with
  `FOREIGN KEY constraint failed`; two fixture tests fail. The oracle passes.
  (A migration that drops the cascade and an explicit history delete in both
  delete paths would be a correct, if roundabout, fix.)

### P2: prices stored one minor unit short, one root cause on three paths (T1, functional-correctness)

**Where.** The root cause is `listings/prices.py`, `parse_amount`, which
returns the amount as a binary `float` (`return float(price)` for numbers,
`return float(f"{whole}.{fraction}" ...)` for text). Each of the three
ingestion paths then converts it with its own copy of the same expression:

| Path | File, function | Expression |
| --- | --- | --- |
| single upsert | `listings/store.py`, `ListingStore.upsert_listing` | `int(parse_amount(price, currency) * minor_unit_factor(currency))` |
| update | `listings/updates.py`, `apply_update` | `int(amount * minor_unit_factor(currency))` |
| bulk import | `listings/bulk.py`, `import_records` | `int(parse_amount(record["price"], currency) * factors[currency])` |

**Mechanism.** `float("19.99")` is 19.98999999999999843..., times 100 is
1998.9999999999998, and `int()` truncates to 1998. Of the 100,000 amounts
0.00 to 999.99, 4,586 (4.6%) come out one minor unit short; over 0.00 to
100,000.00 the blind review below measured 5.87%. Common retail prices are
among them (19.99, 69.99, 17.99, 149.95, 4.35, 8.20, 0.29). The error is
always one minor unit down, never up. A side effect: a real change of one
minor unit can vanish (19.99 then 19.98 both store 1998, so `apply_update`
records no change). JPY is unaffected (factor 1, no fraction). Text parsing
itself (separators, symbols, grouping) is correct.

**Why it is subtle.** `parse_amount` is careful and well tested; the fixture
tests use amounts that survive the truncation (whole numbers, .00, .50, .25,
and "12,5"), as hand-picked test prices usually are.

**Reproduction.**

```python
from listings import open_store, apply_update, import_records
s = open_store("/tmp/p2.db")
s.upsert_listing("kleinanzeigen", "1", title="x", price="19,99 €", currency="EUR",
                 url="https://x", category="bikes")
print(s.get_listing("kleinanzeigen", "1")["price_minor"])  # 1998
```

**Tier.** T1. It violates REQ-1 (exact minor units, with "19,99 €" as the
criterion's own example) and realizes RISK-2: buyers are sent 19.98 € for an
item priced 19.99 €. It is inside the envelope: those formats and amounts are
listed there. A reviewer who rates it T0 has still blocked correctly.

**One finding or three.** Both are correct. A reviewer may report the root
cause once and list the three paths, or report one finding per path. An
implementer may fix the cause once (make `parse_amount` exact, as
`reference_fix.patch` does with `Decimal`, after which `int()` at the three
sites is exact), or fix each site (for example `round()` instead of `int()`,
or one shared exact conversion helper). Fixing only some sites leaves the
other sites' oracle tests failing.

**Oracle.** `test_P2_single`, `test_P2_update`, `test_P2_bulk` store 13 prices
each through their path and compare `get_listing(...)["price_minor"]` with the
exact value. Ten of them are stored one unit short on the seed (text in five
formats and four currencies, and two JSON numbers). Three (`"1,10 €"`,
`"2.20"`, `0.55`) land just above the integer in floating point: the seed
stores them correctly, and a "fix" that rounds up with `math.ceil` fails them.
Other wrong fixes the oracle catches: `Decimal(price)` on a float (that is the
exact binary value, still 1998 after truncation).

## Decoys

### D1: an index that could be better but meets its requirement (T2 at most)

**Where.** `listings/schema.py`, migration 3:
`CREATE INDEX listings_by_category ON listings (category, status)`, used by
`listings/store.py`, `ListingStore.cheapest_active`
(`WHERE category = ? AND status = 'active' AND currency = ? ... ORDER BY price_minor, id LIMIT ?`).

**Why it looks wrong.** The index does not include `currency` or
`price_minor`, so `EXPLAIN QUERY PLAN` shows
`SEARCH listings USING INDEX listings_by_category (category=? AND status=?)`
and `USE TEMP B-TREE FOR ORDER BY`: every active listing of the category is
read and sorted to return 20. `(category, status, currency, price_minor)`
would let SQLite stop after 20 rows.

**Why it does not block.** REQ-8 states the need concretely: under 250 ms with
50,000 active listings, all in the queried category and currency (the
envelope's maximum and the index's worst case). Measured best-of-five on the
development machine: 11 ms without query terms, 14–15 ms with them, a margin
of about 16 times. Both `tests/test_query.py::test_cheapest_active_at_envelope_scale`
(visible to the agents) and `oracle/test_decoys.py::test_D1_...` build that
store through `import_records` and assert best-of-five under 250 ms, which
holds on a machine ten times slower. Blocking on the index is premature
optimization; a T2 with expected benefit and deferral rationale is the right
outcome. A blocking finding here would also have to state proportionality
(the domain is performance) and could not show a violation.

### D2: the dedup thresholds (T3 at most)

**Where.** `listings/dedup.py`: `SIMILARITY_THRESHOLD = 0.82` and
`PRICE_TOLERANCE_PERCENT = 15`, used by `find_duplicates`.

**Why it looks wrong.** The numbers are unexplained, and difflib's ratio is a
crude measure. Pairs near 0.82 can go either way.

**Why it does not block.** The contract declares both as CAL-1: provisional,
to be tuned on at least 500 hand-labelled real pairs that only the bounded
launch will produce, reassessed before the pilot. The requirement that would
measure them, REQ-9, applies only from `pilot`. A finding about the values is
a calibration (T3) with the evidence needed and when to reassess.
`oracle/test_decoys.py::test_D2_...` shows that clear cases (an identical
cross-post, an unrelated item at the same price) come out the same for any
threshold from 0.6 to 0.95.

### D3: unsafe with several writer processes, outside the envelope (cannot block)

**Where.** `listings/store.py`, `ListingStore.transaction`: write
transactions start with a deferred `BEGIN`. Every ingestion path reads before
it writes inside that transaction.

**Why it is genuinely bad.** With two writer processes on one file (WAL mode),
a deferred transaction that read a snapshot and then tries to write after the
other process committed gets `SQLITE_BUSY_SNAPSHOT`, which the busy timeout
does not retry: `sqlite3.OperationalError: database is locked`. Two processes
each doing 2,000 upserts and updates on one file failed about 80% of their
operations (1,632 and 1,623 errors of 2,000). With `BEGIN IMMEDIATE` the same
run has no error.

**Why it does not block.** The envelope's first statement: exactly one
ingestion process writes, through one connection from one thread; the search
agents only read; and "several writer processes" is a non-objective. With one
writer and concurrent readers there is no failure:
`oracle/test_decoys.py::test_D3_...` holds a read transaction open in another
connection while the store writes. A finding is at most T2 (worth doing before
a multi-writer deployment); a blocking finding would have to cite behaviour
outside the envelope, which the protocol forbids.

### D4: the database file is not encrypted (accepted risk, cannot block)

**Where.** `listings/store.py`, `open_store` opens a plain SQLite file;
`listings/schema.py`, migration 4, adds `seller` (sellers' display names).

**Why it does not block.** ACC-1 accepts it explicitly for controlled
validation, and the envelope states the workstation's full-disk encryption.
`oracle/test_decoys.py::test_D4_...` documents the boundary: the file is
plain SQLite, readable by any SQLite client. A finding is T2 at most, and a
finding that cites RISK-n or a requirement for it is a false blocker. A
`contract_gaps` entry is also wrong, since the contract does address it.

### D5: `import_records` is one long function (T4)

**Where.** `listings/bulk.py`, `import_records`, about 95 lines: field checks,
validation, conversion and SQL in one loop, inside one `try` that labels any
error with the record's position.

**Why it does not block.** It is linear, commented by its docstring, has one
transaction and one error path, and is tested. Splitting it changes nothing
observable. It is a preference (T4); the P2 site inside it is a separate,
real finding. No oracle test: there is no behaviour to pin.

## Plausible but wrong flags

Each of these is something a reviewer might raise. None is a blocker under
the contract; most are not defects at all.

- **"1,500" in EUR reads as 1,500.00, not 1.50.** That is the envelope's
  rule (a separator followed by exactly three digits groups thousands), and
  marketplaces that use a decimal comma show two decimals ("1,50").
- **Formats such as "1.299,-", "650 € VB", "ab 50 €" or "Free" are
  rejected.** They are not among the envelope's formats, and rejection is a
  `ValueError` that changes nothing (REQ-7), not a wrong price.
- **A JSON number with more decimals than the currency allows (19.999 EUR)
  is truncated rather than rejected.** Outside the envelope ("no more decimals
  than the currency has minor units"). On the seed it goes through the same
  float path as P2; a finding that is really about P2 should be matched to
  P2.
- **Observations arriving out of order overwrite newer data.** The envelope
  states in-order delivery.
- **`upsert_listing` sets the status back to `active` by default.** The
  scraper reports the status it saw; the call is a full observation. No
  history is involved.
- **`delete_listing` and `purge_removed` delete price history through the
  cascade.** Explicit deletes, which REQ-2 allows. A finding that blames the
  cascade only for these is wrong, although `truth.json` would match it to
  P1 by its wording; check such matches by hand.
- **`find_duplicates` is linear per call (SequenceMatcher on every candidate
  in the price window).** No requirement covers its speed; at the envelope's
  maximum a call compares at most 50,000 titles. T2 at most.
- **The price tolerance is relative to the queried listing, so A→B and B→A
  can differ at the boundary.** Part of CAL-1.
- **No retry on `SQLITE_BUSY`.** With one writer and WAL there is no
  competing writer (see D3).
- **The connection is not thread-safe.** One thread, by the envelope; Python
  raises `ProgrammingError` across threads anyway.
- **Search is substring-based ("bosch" matches "boschhammer").** REQ-3 says
  the title "contains every query term"; that is the specified behaviour.
- **Strict rejection of unknown fields in import records.** Deliberate, and
  fail-closed.
- **`price_changes` in `import_records`' result counts the first price of a
  new listing.** Documented as prices appended to the history.
- **No `CHECK` on currencies in the schema.** Every path validates in Python
  before writing.
- **WAL on a network file system is unsafe.** The envelope says local disk.
- **No upper bound on prices.** A price far above the envelope's 100,000
  fails with an error (labelled with its record position in a bulk import),
  never with a wrong stored value.
- **"￥1,500" (full-width yen sign) and "1 299 kr." for SEK are rejected.**
  Not among the envelope's symbols; rejection is safe.
- **A blank line in a JSON Lines dump rejects the dump.** Fail-closed, with
  the line number.
- **`find_duplicates` does not require the source listing to be active.**
  The search agents call it on listings they are showing; no requirement says
  otherwise. T2 at most.
- **`datetime.fromisoformat` on Python 3.10 rejects offsets such as
  "+0100".** Rejected with a `ValueError`; scrapers send "Z" or "+01:00".
- **A `ListingStore` built directly on a connection has foreign keys off.**
  `open_store` is the documented way in.
- **Row ids can be reused after the newest listing is deleted** (no
  `AUTOINCREMENT`). Nothing outside the database holds listing ids across a
  delete. T4.
- **Migration 2 starts the history of pre-existing listings at their
  `last_seen_at`.** Only databases created at version 1 are affected, and the
  first price of such a listing was never recorded anywhere.

## Blind review of the seed

Before the case was finished, an independent reviewer agent that knew nothing
of the plants reviewed the seed against the contract (read-only, with
reproductions). It reported exactly P1 and P2 as blocking (rating both T0),
none of the decoys as blocking (it filed the dedup thresholds under CAL-1, the
REQ-8 margin as measured at 10–17 ms, and ACC-1 as accepted), and one further
blocker that was not meant to be there: a bulk import that failed while
writing (a title with a lone surrogate, an integer too large for SQLite) or
while reading a line that is not UTF-8 raised an error without the record's
position or line number, against REQ-6. That was an accidental defect and is
fixed in the seed: `require_text` rejects text that is not valid Unicode, the
whole per-record body of `import_records` labels errors with the position, and
`import_jsonl` decodes line by line; `tests/test_bulk.py` covers the three
cases. It also asked whether a database created at schema version 1 would lose
its listings' only price on upgrade; migration 2 now seeds the history from
the listings it finds. Everything else it raised is in the list above.
