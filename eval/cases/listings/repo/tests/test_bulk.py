import json
import os
import tempfile
import unittest

from listings import import_jsonl, import_records

from support import StoreTestCase, at, record, stamp


class ImportRecordsTest(StoreTestCase):
    def test_new_records_are_inserted(self):
        counts = import_records(
            self.store,
            [
                record(1, price="1.299,00"),
                record(2, price=12.5, seller="Anna"),
                record("g9", backend="gumtree", price="£85", currency="GBP", url="https://www.gumtree.com/p/g9",
                       seen_at="2026-11-19T18:30:00Z"),
            ],
            seen_at=at(0),
        )
        self.assertEqual(counts, {"inserted": 3, "updated": 0, "price_changes": 3})
        self.assertEqual(self.store.get_listing("willhaben", 1)["price_minor"], 129900)
        self.assertEqual(self.store.get_listing("willhaben", 1)["first_seen_at"], stamp(0))
        self.assertEqual(self.store.get_listing("willhaben", 2)["price_minor"], 1250)
        self.assertEqual(self.store.get_listing("willhaben", 2)["seller"], "Anna")
        self.assertEqual(self.store.get_listing("gumtree", "g9")["price_minor"], 8500)
        self.assertEqual(self.store.get_listing("gumtree", "g9")["first_seen_at"], "2026-11-19T18:30:00.000000Z")

    def test_reimport_updates_listings_and_records_price_changes(self):
        import_records(self.store, [record(1, price="1.299,00"), record(2), record(3)], seen_at=at(0))
        counts = import_records(
            self.store,
            [record(1, price="1.199,00"), record(2, title="Cube Aim Race 29"), record(3)],
            seen_at=at(24),
        )
        self.assertEqual(counts, {"inserted": 0, "updated": 3, "price_changes": 1})
        history = self.store.price_history("willhaben", 1)
        self.assertEqual([entry["price_minor"] for entry in history], [129900, 119900])
        self.assertEqual(self.store.get_listing("willhaben", 2)["title"], "Cube Aim Race 29")
        listing = self.store.get_listing("willhaben", 3)
        self.assertEqual((listing["first_seen_at"], listing["last_seen_at"]), (stamp(0), stamp(24)))

    def test_one_invalid_record_rejects_the_batch(self):
        cases = [
            (record(2, price="auf Anfrage"), "record 2"),
            (record(2, colour="red"), "unknown fields colour"),
            ({"backend": "willhaben", "external_id": 2}, "missing fields"),
            (record(2, currency="HUF"), "unsupported currency"),
            (record(2, title="iPhone 12 \ud83d"), "record 2: title is not valid Unicode"),
            (record(2, price=10**30), "record 2"),
            ("willhaben,2", "expected a mapping"),
        ]
        for bad, message in cases:
            with self.subTest(message=message):
                with self.assertRaisesRegex(ValueError, message):
                    import_records(self.store, [record(1), bad, record(3)], seen_at=at(0))
                self.assertEqual(self.store.stats()["listings"], 0)


class ImportJsonlTest(StoreTestCase):
    def write_dump(self, lines):
        handle, path = tempfile.mkstemp(suffix=".jsonl")
        self.addCleanup(os.remove, path)
        with os.fdopen(handle, "wb") as dump:
            dump.write(b"".join((line if isinstance(line, bytes) else line.encode("utf-8")) + b"\n" for line in lines))
        return path

    def test_dump_is_imported(self):
        path = self.write_dump([json.dumps(record(1)), json.dumps(record(2, price="380,00"))])
        self.assertEqual(
            import_jsonl(self.store, path, seen_at=at(0)),
            {"inserted": 2, "updated": 0, "price_changes": 2},
        )
        self.assertEqual(self.store.get_listing("willhaben", 2)["price_minor"], 38000)

    def test_malformed_line_rejects_the_dump(self):
        for bad in ("{not json", '{"title": "Fahrrad f\xfcr Kinder"}'.encode("latin-1")):
            with self.subTest(bad=bad):
                path = self.write_dump([json.dumps(record(1)), bad, json.dumps(record(3))])
                with self.assertRaisesRegex(ValueError, "line 2"):
                    import_jsonl(self.store, path, seen_at=at(0))
                self.assertEqual(self.store.stats()["listings"], 0)


if __name__ == "__main__":
    unittest.main()
