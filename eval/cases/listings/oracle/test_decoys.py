"""Hidden oracle for the decoys of the listings case.

These tests pass on the seed tree and on any correct fix. They document where
each decoy's boundary lies: what the contract asks of that part of the code,
and that the seed already does it. A review that blocks on a decoy blocks on
something these tests show to be within the contract.

    LISTINGS_REPO=/path/to/tree python3 -m unittest discover -s oracle -p 'test_decoys.py' -v

D5 (a preference about the shape of import_records) has no behaviour to
test; NOTES.md explains it.
"""

from __future__ import annotations

import os
import sqlite3
import sys
import tempfile
import time
import unittest
from datetime import datetime, timedelta, timezone

sys.dont_write_bytecode = True
_REPO = os.environ.get("LISTINGS_REPO")
if not _REPO:
    raise RuntimeError("set LISTINGS_REPO to the root of the listings tree under test")
sys.path.insert(0, os.path.abspath(_REPO))

from listings import find_duplicates, import_records, open_store  # noqa: E402

T0 = datetime(2026, 11, 20, 9, 0, tzinfo=timezone.utc)

# Operating envelope: at most 50,000 stored listings, any category may hold
# all of them.
ENVELOPE_LISTINGS = 50_000

# REQ-8: cheapest_active returns within 250 ms at envelope scale.
REQ_8_BUDGET_SECONDS = 0.250


def record(n: object, **overrides: object) -> dict:
    fields = {
        "backend": "willhaben",
        "external_id": n,
        "title": f"Cube Aim Race {n}",
        "price": 450,
        "currency": "EUR",
        "url": f"https://www.willhaben.at/iad/kaufen-und-verkaufen/d/{n}",
        "category": "bikes",
    }
    fields.update(overrides)
    return fields


class DecoyCase(unittest.TestCase):
    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.path = os.path.join(directory.name, "oracle.db")
        self.store = open_store(self.path)
        self.addCleanup(self.store.close)


class D1Test(DecoyCase):
    """D1: the search index forces a sort, but REQ-8 is met with a wide margin."""

    def test_D1_cheapest_active_meets_REQ_8_at_envelope_scale(self):
        # Worst case: every listing active, in one category and one currency,
        # so the index narrows nothing and the sort covers all 50,000 rows.
        import_records(
            self.store,
            (
                record(n, title=f"Mountainbike Modell {n % 977} Größe {'SML'[n % 3]}", price=100 + (n * 7919) % 50_000)
                for n in range(ENVELOPE_LISTINGS)
            ),
            seen_at=T0,
        )
        for query in (None, "modell 12", "größe s"):
            with self.subTest(query=query):
                fastest = float("inf")
                for _ in range(5):
                    started = time.perf_counter()
                    rows = self.store.cheapest_active("bikes", currency="EUR", query=query, limit=20)
                    fastest = min(fastest, time.perf_counter() - started)
                prices = [row["price_minor"] for row in rows]
                self.assertEqual(len(prices), 20)
                self.assertEqual(prices, sorted(prices))
                self.assertLess(fastest, REQ_8_BUDGET_SECONDS)


class D2Test(DecoyCase):
    """D2: the dedup thresholds are provisional (CAL-1); clear cases do not depend on them."""

    def test_D2_clear_cases_hold_for_any_plausible_threshold(self):
        import_records(
            self.store,
            [
                record("k1", backend="kleinanzeigen", title="Trek Marlin 7 2022 Rahmen M", price=650,
                       url="https://www.kleinanzeigen.de/s-anzeige/k1"),
                record("t1", backend="tori", title="Trek Marlin 7 2022 Rahmen M", price=650,
                       url="https://www.tori.fi/t1"),
                record("w1", title="Kühlschrank Bosch KGN39", price=650),
            ],
            seen_at=T0,
        )
        for threshold in (0.6, 0.7, 0.82, 0.9, 0.95):
            with self.subTest(threshold=threshold):
                matches = find_duplicates(self.store, "kleinanzeigen", "k1", threshold=threshold)
                self.assertEqual([(m["backend"], m["external_id"]) for m in matches], [("tori", "t1")])


class D3Test(DecoyCase):
    """D3: one writer with concurrent readers, the envelope's case, works."""

    def test_D3_single_writer_with_a_concurrent_reader(self):
        import_records(self.store, [record(1)], seen_at=T0)
        reader = sqlite3.connect(self.path, timeout=1.0, isolation_level=None)
        self.addCleanup(reader.close)
        reader.execute("BEGIN")
        self.assertEqual(reader.execute("SELECT COUNT(*) FROM listings").fetchone()[0], 1)

        # The writer is not blocked by the open read transaction.
        import_records(self.store, [record(n) for n in range(2, 102)], seen_at=T0 + timedelta(hours=1))
        self.store.upsert_listing(
            "tori", "t1", title="Jopo", price="120 €", currency="EUR", url="https://www.tori.fi/t1",
            category="bikes", seen_at=T0 + timedelta(hours=2),
        )

        self.assertEqual(reader.execute("SELECT COUNT(*) FROM listings").fetchone()[0], 1)
        reader.execute("COMMIT")
        self.assertEqual(reader.execute("SELECT COUNT(*) FROM listings").fetchone()[0], 102)


class D4Test(DecoyCase):
    """D4: the database file is plain SQLite, readable by anyone who can read the file (ACC-1)."""

    def test_D4_database_file_is_not_encrypted(self):
        import_records(self.store, [record(1, seller="Anna")], seen_at=T0)
        self.store.close()
        with open(self.path, "rb") as handle:
            self.assertEqual(handle.read(16), b"SQLite format 3\x00")
        plain = sqlite3.connect(self.path)
        self.addCleanup(plain.close)
        self.assertEqual(plain.execute("SELECT seller FROM listings").fetchone()[0], "Anna")


if __name__ == "__main__":
    unittest.main()
