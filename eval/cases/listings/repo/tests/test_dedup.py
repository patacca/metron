import unittest

from listings import apply_update, find_duplicates, import_records, title_similarity

from support import StoreTestCase, at, record


class TitleSimilarityTest(unittest.TestCase):
    def test_similarity_ignores_case_and_spacing(self):
        self.assertEqual(title_similarity("Trek Marlin 7  2022", "trek marlin 7 2022"), 1.0)
        self.assertLess(title_similarity("Trek Marlin 7", "Kühlschrank Bosch"), 0.5)


class FindDuplicatesTest(StoreTestCase):
    def setUp(self):
        super().setUp()
        import_records(
            self.store,
            [
                record("k1", backend="kleinanzeigen", title="Trek Marlin 7 2022 Rahmen M", price="650,00",
                       url="https://www.kleinanzeigen.de/s-anzeige/k1"),
                record("w1", title="Trek Marlin 7 (2022) Rahmen M", price="620,00"),
                record("w2", title="Trek Marlin 7 2022 Rahmen M", price="300,00"),
                record("w3", title="Trek Marlin 7 2022 Rahmen M", price="640,00", category="parts"),
                record("w4", title="Cube Attention 2021", price="650,00"),
                record("k2", backend="kleinanzeigen", title="Trek Marlin 7 2022 Rahmen M", price="655,00",
                       url="https://www.kleinanzeigen.de/s-anzeige/k2"),
                record("t1", backend="tori", title="Trek Marlin 7 2022 Rahmen M", price="660,00",
                       url="https://www.tori.fi/t1"),
            ],
            seen_at=at(0),
        )

    def test_finds_the_same_item_on_other_backends(self):
        matches = find_duplicates(self.store, "kleinanzeigen", "k1")
        self.assertEqual([(m["backend"], m["external_id"]) for m in matches], [("tori", "t1"), ("willhaben", "w1")])
        self.assertEqual(matches[0]["score"], 1.0)
        self.assertGreaterEqual(matches[1]["score"], 0.82)

    def test_sold_listings_are_not_duplicates(self):
        apply_update(self.store, "tori", "t1", status="sold", observed_at=at(1))
        matches = find_duplicates(self.store, "kleinanzeigen", "k1")
        self.assertEqual([m["external_id"] for m in matches], ["w1"])

    def test_unknown_listing(self):
        with self.assertRaises(LookupError):
            find_duplicates(self.store, "kleinanzeigen", "k9")


if __name__ == "__main__":
    unittest.main()
