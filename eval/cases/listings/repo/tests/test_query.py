import time
import unittest

from listings import apply_update, import_records

from support import StoreTestCase, at, record

# The largest store the operating envelope allows.
ENVELOPE_LISTINGS = 50_000

# The response time the search agents need from cheapest_active.
RESPONSE_BUDGET_SECONDS = 0.250


class CheapestActiveTest(StoreTestCase):
    def setUp(self):
        super().setUp()
        import_records(
            self.store,
            [
                record(1, title="Kühlschrank Bosch KGN39", price="180,00", category="appliances"),
                record(2, title="KÜHLSCHRANK Liebherr", price="150,00", category="appliances"),
                record(3, title="Gefrierschrank Bosch", price="90,00", category="appliances"),
                record(4, title="Kühlschrank Miele", price="150,00", category="appliances"),
                record(5, title="Kühlschrank AEG", price="60,00", category="appliances"),
                record(6, title="Kühlschrank Gorenje", price="40,00", category="kitchen"),
                record(7, title="Kühlschrank Smeg", price="1.300,00", category="appliances", currency="CHF"),
            ],
            seen_at=at(0),
        )
        apply_update(self.store, "willhaben", 5, status="sold", observed_at=at(1))

    def ids(self, **kwargs):
        return [row["external_id"] for row in self.store.cheapest_active("appliances", currency="EUR", **kwargs)]

    def test_cheapest_first_with_ties_in_insertion_order(self):
        self.assertEqual(self.ids(), ["3", "2", "4", "1"])

    def test_only_active_listings_of_the_category_and_currency(self):
        rows = self.store.cheapest_active("appliances", currency="CHF")
        self.assertEqual([row["external_id"] for row in rows], ["7"])
        self.assertEqual(rows[0]["price_minor"], 130000)
        self.assertNotIn("5", self.ids())
        self.assertNotIn("6", self.ids())

    def test_query_terms_ignore_case_including_umlauts(self):
        self.assertEqual(self.ids(query="kühlschrank"), ["2", "4", "1"])
        self.assertEqual(self.ids(query="BOSCH  Kühl"), ["1"])
        self.assertEqual(self.ids(query="bosch"), ["3", "1"])
        self.assertEqual(self.ids(query="waschmaschine"), [])

    def test_limit(self):
        self.assertEqual(self.ids(limit=2), ["3", "2"])
        for limit in (0, 101, "5", True, 2.0):
            with self.subTest(limit=limit):
                with self.assertRaises(ValueError):
                    self.ids(limit=limit)


class EnvelopeScaleTest(StoreTestCase):
    def test_cheapest_active_at_envelope_scale(self):
        # Worst case for the query: every listing in the envelope is active,
        # in the same category and in the same currency.
        import_records(
            self.store,
            (
                record(
                    n,
                    title=f"Mountainbike Modell {n % 977} Größe {'SML'[n % 3]}",
                    price=100 + (n * 7919) % 50_000,
                )
                for n in range(ENVELOPE_LISTINGS)
            ),
            seen_at=at(0),
        )
        for query in (None, "modell 12", "größe s"):
            with self.subTest(query=query):
                fastest = float("inf")
                for _ in range(5):
                    started = time.perf_counter()
                    rows = self.store.cheapest_active("bikes", currency="EUR", query=query, limit=20)
                    fastest = min(fastest, time.perf_counter() - started)
                self.assertEqual(len(rows), 20)
                prices = [row["price_minor"] for row in rows]
                self.assertEqual(prices, sorted(prices))
                self.assertLess(fastest, RESPONSE_BUDGET_SECONDS)


if __name__ == "__main__":
    unittest.main()
