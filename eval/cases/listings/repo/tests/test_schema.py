import sqlite3
import unittest

from listings import SCHEMA_VERSION, open_store
from listings.schema import migrate, schema_version

from support import StoreTestCase


class SchemaTest(StoreTestCase):
    def test_new_database_is_at_the_current_version(self):
        self.assertEqual(schema_version(self.store.connection), SCHEMA_VERSION)
        tables = {
            row[0]
            for row in self.store.connection.execute("SELECT name FROM sqlite_master WHERE type = 'table'")
        }
        self.assertTrue({"listings", "price_history"} <= tables)

    def test_older_database_is_upgraded_in_place(self):
        self.store.close()
        old_path = self.path + ".v1"
        connection = sqlite3.connect(old_path, isolation_level=None)
        migrate(connection, target=1)
        connection.execute(
            "INSERT INTO listings (backend, external_id, title, search_text, category, url, "
            "price_minor, currency, first_seen_at, last_seen_at) "
            "VALUES ('blocket', '77', 'Kayak', 'kayak', 'boats', 'https://www.blocket.se/77', 450000, 'SEK', "
            "'2026-10-01T00:00:00.000000Z', '2026-11-01T00:00:00.000000Z')"
        )
        connection.close()

        with open_store(old_path) as store:
            self.assertEqual(schema_version(store.connection), SCHEMA_VERSION)
            listing = store.get_listing("blocket", "77")
            history = store.price_history("blocket", "77")
        self.assertEqual(listing["status"], "active")
        self.assertIsNone(listing["seller"])
        self.assertEqual(listing["price_minor"], 450000)
        self.assertEqual(
            history, [{"observed_at": "2026-11-01T00:00:00.000000Z", "price_minor": 450000, "currency": "SEK"}]
        )

    def test_reopening_keeps_data(self):
        self.store.upsert_listing(
            "tori", "9", title="Jopo", price="120 €", currency="EUR", url="https://tori.fi/9", category="bikes"
        )
        self.store.close()
        with open_store(self.path) as store:
            self.assertEqual(store.get_listing("tori", "9")["price_minor"], 12000)

    def test_refuses_a_newer_schema(self):
        self.store.connection.execute(f"PRAGMA user_version = {SCHEMA_VERSION + 1}")
        self.store.close()
        with self.assertRaises(RuntimeError):
            open_store(self.path)


if __name__ == "__main__":
    unittest.main()
