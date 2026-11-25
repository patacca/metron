import unittest
from datetime import datetime

from listings import apply_update, import_records

from support import StoreTestCase, at, bike, record, stamp


class UpsertTest(StoreTestCase):
    def test_new_listing_is_stored(self):
        listing_id = self.store.upsert_listing("kleinanzeigen", 2861734412, **bike(price="1.299,00 €"))
        listing = self.store.get_listing("kleinanzeigen", "2861734412")
        self.assertEqual(listing["id"], listing_id)
        self.assertEqual(listing["external_id"], "2861734412")
        self.assertEqual(listing["title"], "Trek Marlin 7, 2022, Rahmen M")
        self.assertEqual(listing["price_minor"], 129900)
        self.assertEqual(listing["currency"], "EUR")
        self.assertEqual(listing["status"], "active")
        self.assertEqual(listing["seller"], "Jonas")
        self.assertEqual(listing["first_seen_at"], stamp(0))
        self.assertEqual(listing["last_seen_at"], stamp(0))

    def test_new_listing_starts_its_price_history(self):
        self.store.upsert_listing("kleinanzeigen", "1", **bike())
        self.assertEqual(
            self.store.price_history("kleinanzeigen", "1"),
            [{"observed_at": stamp(0), "price_minor": 64900, "currency": "EUR"}],
        )

    def test_rescrape_refreshes_the_listing(self):
        first_id = self.store.upsert_listing("kleinanzeigen", "1", **bike())
        second_id = self.store.upsert_listing(
            "kleinanzeigen", "1", **bike(title="Trek Marlin 7 (2022) Größe M", seller=None, seen_at=at(30))
        )
        listing = self.store.get_listing("kleinanzeigen", "1")
        self.assertEqual(second_id, first_id)
        self.assertEqual(listing["title"], "Trek Marlin 7 (2022) Größe M")
        self.assertIsNone(listing["seller"])
        self.assertEqual(listing["first_seen_at"], stamp(0))
        self.assertEqual(listing["last_seen_at"], stamp(30))

    def test_rescrape_with_a_new_price_sets_the_current_price(self):
        self.store.upsert_listing("kleinanzeigen", "1", **bike(price="250,00 €"))
        self.store.upsert_listing("kleinanzeigen", "1", **bike(price="225,00 €", seen_at=at(5)))
        self.assertEqual(self.store.get_listing("kleinanzeigen", "1")["price_minor"], 22500)

    def test_invalid_listing_is_refused(self):
        cases = [
            ("ebay", bike()),
            ("kleinanzeigen", bike(title="  ")),
            ("kleinanzeigen", bike(currency="USD")),
            ("kleinanzeigen", bike(price="ab 50 €")),
            ("kleinanzeigen", bike(status="reserved")),
            ("kleinanzeigen", bike(seen_at=datetime(2026, 11, 20, 9, 0))),
        ]
        for backend, fields in cases:
            with self.subTest(backend=backend, fields=fields):
                with self.assertRaises(ValueError):
                    self.store.upsert_listing(backend, "1", **fields)
        self.assertEqual(self.store.stats()["listings"], 0)


class DeletionTest(StoreTestCase):
    def test_delete_listing_removes_it_and_its_history(self):
        self.store.upsert_listing("kleinanzeigen", "1", **bike())
        apply_update(self.store, "kleinanzeigen", "1", price="600 €", observed_at=at(1))
        self.assertTrue(self.store.delete_listing("kleinanzeigen", "1"))
        self.assertIsNone(self.store.get_listing("kleinanzeigen", "1"))
        self.assertEqual(self.store.price_history("kleinanzeigen", "1"), [])
        self.assertEqual(self.store.stats()["price_history_rows"], 0)
        self.assertFalse(self.store.delete_listing("kleinanzeigen", "1"))

    def test_purge_removed_deletes_only_old_removed_listings(self):
        import_records(self.store, [record(1), record(2), record(3)], seen_at=at(0))
        apply_update(self.store, "willhaben", 2, status="removed", observed_at=at(1))
        apply_update(self.store, "willhaben", 3, status="removed", observed_at=at(48))
        self.assertEqual(self.store.purge_removed(at(24)), 1)
        self.assertIsNotNone(self.store.get_listing("willhaben", 1))
        self.assertIsNone(self.store.get_listing("willhaben", 2))
        self.assertIsNotNone(self.store.get_listing("willhaben", 3))


class StatsTest(StoreTestCase):
    def test_stats_count_listings_and_history(self):
        import_records(
            self.store,
            [record(1), record(2), record(3, backend="tori", url="https://tori.fi/3")],
            seen_at=at(0),
        )
        apply_update(self.store, "willhaben", 1, status="sold", observed_at=at(2))
        apply_update(self.store, "willhaben", 2, price="420,00", observed_at=at(2))
        self.assertEqual(
            self.store.stats(),
            {
                "listings": 3,
                "by_backend": {"tori": {"active": 1}, "willhaben": {"active": 1, "sold": 1}},
                "price_history_rows": 4,
            },
        )


if __name__ == "__main__":
    unittest.main()
