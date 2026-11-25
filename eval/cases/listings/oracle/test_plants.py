"""Hidden oracle for the blocking plants of the listings case.

Each test fails on the seed tree and passes on any correct fix. They use only
the public API that the fixture's README documents, never its internals, so
a fix that restructures the code still passes.

    LISTINGS_REPO=/path/to/tree python3 -m unittest discover -s oracle -p 'test_plants.py' -v

The tree under test is imported from LISTINGS_REPO. No bytecode is written
into it, so running the oracle leaves a candidate's working tree clean.
"""

from __future__ import annotations

import os
import sys
import tempfile
import unittest
from datetime import datetime, timedelta, timezone

sys.dont_write_bytecode = True
_REPO = os.environ.get("LISTINGS_REPO")
if not _REPO:
    raise RuntimeError("set LISTINGS_REPO to the root of the listings tree under test")
sys.path.insert(0, os.path.abspath(_REPO))

from listings import apply_update, import_records, open_store  # noqa: E402

T0 = datetime(2026, 11, 20, 9, 0, tzinfo=timezone.utc)

# Prices inside the operating envelope, with the exact number of minor units
# each stands for. On the seed tree the first ten are stored one minor unit
# short: the amount becomes the nearest binary double, its product with the
# minor-unit factor lands just below the exact integer, and int() truncates.
# The last three land just above it; the seed stores them correctly, and they
# are here so that a "fix" which rounds up instead of to the nearest fails.
EXACT_PRICES = [
    ("19.99", "EUR", 1999),
    ("19,99 €", "EUR", 1999),
    ("4,35 €", "EUR", 435),
    ("69,99", "EUR", 6999),
    ("149,95 €", "EUR", 14995),
    ("£17.99", "GBP", 1799),
    ("8,20 zł", "PLN", 820),
    ("CHF 16.90", "CHF", 1690),
    (0.29, "EUR", 29),
    (19.99, "EUR", 1999),
    ("1,10 €", "EUR", 110),
    ("2.20", "EUR", 220),
    (0.55, "EUR", 55),
]


class StoreCase(unittest.TestCase):
    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.store = open_store(os.path.join(directory.name, "oracle.db"))
        self.addCleanup(self.store.close)

    def upsert(self, external_id: str, price: object, currency: str, hours: float = 0) -> None:
        self.store.upsert_listing(
            "kleinanzeigen",
            external_id,
            title="Trek Marlin 7, 2022, Rahmen M",
            price=price,
            currency=currency,
            url=f"https://www.kleinanzeigen.de/s-anzeige/{external_id}",
            category="bikes",
            seen_at=T0 + timedelta(hours=hours),
        )

    def stored_price(self, backend: str, external_id: str) -> int:
        listing = self.store.get_listing(backend, external_id)
        self.assertIsNotNone(listing, f"{backend}/{external_id} is not in the store")
        return listing["price_minor"]


class P1Test(StoreCase):
    """P1: re-scraping a listing destroys its price history (REQ-2, RISK-1)."""

    def changes(self, external_id: str) -> list[int]:
        """Return the listing's history as prices, with consecutive repeats collapsed."""
        prices = [entry["price_minor"] for entry in self.store.price_history("kleinanzeigen", external_id)]
        return [p for i, p in enumerate(prices) if i == 0 or p != prices[i - 1]]

    def test_P1_rescrape_keeps_price_history(self):
        # All prices have exact binary representations, so that this test
        # does not depend on P2. A second listing with its own history comes
        # first, so that listing ids and history row ids differ: a fix that
        # attaches history to the wrong listing does not pass by accident.
        self.upsert("other", "50,00 €", "EUR", hours=0)
        apply_update(self.store, "kleinanzeigen", "other", price="45,00 €", observed_at=T0 + timedelta(minutes=10))
        apply_update(self.store, "kleinanzeigen", "other", price="40,00 €", observed_at=T0 + timedelta(minutes=20))

        self.upsert("p1", "100,00 €", "EUR", hours=0)
        self.upsert("p1", "90,00 €", "EUR", hours=1)
        self.upsert("p1", "90,00 €", "EUR", hours=2)
        apply_update(self.store, "kleinanzeigen", "p1", price="85,00 €", observed_at=T0 + timedelta(hours=3))
        self.upsert("p1", "85,00 €", "EUR", hours=4)
        self.upsert("other", "40,00 €", "EUR", hours=5)

        history = self.store.price_history("kleinanzeigen", "p1")
        self.assertEqual(
            self.changes("p1"),
            [10000, 9000, 8500],
            f"price history after re-scrapes is {history}: every observed price must be kept",
        )
        self.assertEqual(history[0]["observed_at"], "2026-11-20T09:00:00.000000Z")
        self.assertEqual(self.changes("other"), [5000, 4500, 4000])
        listing = self.store.get_listing("kleinanzeigen", "p1")
        self.assertEqual(listing["first_seen_at"], "2026-11-20T09:00:00.000000Z")
        self.assertEqual(listing["price_minor"], 8500)


class P2Test(StoreCase):
    """P2: prices are truncated through binary floating point (REQ-1, RISK-2), on three paths."""

    def test_P2_single(self):
        for n, (price, currency, expected) in enumerate(EXACT_PRICES):
            with self.subTest(price=price, currency=currency):
                self.upsert(f"s{n}", price, currency)
                self.assertEqual(self.stored_price("kleinanzeigen", f"s{n}"), expected)

    def test_P2_update(self):
        for n, (price, currency, expected) in enumerate(EXACT_PRICES):
            with self.subTest(price=price, currency=currency):
                self.upsert(f"u{n}", "100", currency)
                apply_update(
                    self.store,
                    "kleinanzeigen",
                    f"u{n}",
                    price=price,
                    currency=currency,
                    observed_at=T0 + timedelta(hours=1),
                )
                self.assertEqual(self.stored_price("kleinanzeigen", f"u{n}"), expected)

    def test_P2_bulk(self):
        records = [
            {
                "backend": "willhaben",
                "external_id": f"b{n}",
                "title": "Cube Aim Race",
                "price": price,
                "currency": currency,
                "url": f"https://www.willhaben.at/iad/kaufen-und-verkaufen/d/b{n}",
                "category": "bikes",
            }
            for n, (price, currency, _) in enumerate(EXACT_PRICES)
        ]
        import_records(self.store, records, seen_at=T0)
        for n, (price, currency, expected) in enumerate(EXACT_PRICES):
            with self.subTest(price=price, currency=currency):
                self.assertEqual(self.stored_price("willhaben", f"b{n}"), expected)


if __name__ == "__main__":
    unittest.main()
