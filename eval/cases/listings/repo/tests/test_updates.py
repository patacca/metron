import unittest

from listings import apply_update

from support import StoreTestCase, at, bike, stamp


class ApplyUpdateTest(StoreTestCase):
    def setUp(self):
        super().setUp()
        self.store.upsert_listing("kleinanzeigen", "1", **bike(price="100 €"))

    def history(self):
        return [entry["price_minor"] for entry in self.store.price_history("kleinanzeigen", "1")]

    def test_new_price_is_stored_and_recorded(self):
        self.assertTrue(apply_update(self.store, "kleinanzeigen", "1", price="90 €", observed_at=at(1)))
        listing = self.store.get_listing("kleinanzeigen", "1")
        self.assertEqual(listing["price_minor"], 9000)
        self.assertEqual(listing["last_seen_at"], stamp(1))
        self.assertEqual(self.history(), [10000, 9000])
        self.assertEqual(self.store.price_history("kleinanzeigen", "1")[1]["observed_at"], stamp(1))

    def test_same_price_is_not_recorded_again(self):
        self.assertFalse(apply_update(self.store, "kleinanzeigen", "1", price="100,00 €", observed_at=at(1)))
        self.assertEqual(self.history(), [10000])
        self.assertEqual(self.store.get_listing("kleinanzeigen", "1")["last_seen_at"], stamp(1))

    def test_successive_changes_are_kept_in_order(self):
        for hours, price in ((1, "95 €"), (2, "90 €"), (3, "95 €")):
            apply_update(self.store, "kleinanzeigen", "1", price=price, observed_at=at(hours))
        self.assertEqual(self.history(), [10000, 9500, 9000, 9500])

    def test_status_change_keeps_the_price(self):
        self.assertFalse(apply_update(self.store, "kleinanzeigen", "1", status="sold", observed_at=at(3)))
        listing = self.store.get_listing("kleinanzeigen", "1")
        self.assertEqual(listing["status"], "sold")
        self.assertEqual(listing["price_minor"], 10000)

    def test_price_is_read_in_the_listing_currency(self):
        self.store.upsert_listing(
            "mercari_jp", "m123", **bike(price="¥1,500", currency="JPY", url="https://jp.mercari.com/item/m123")
        )
        apply_update(self.store, "mercari_jp", "m123", price="1,200", observed_at=at(1))
        self.assertEqual(self.store.get_listing("mercari_jp", "m123")["price_minor"], 1200)

    def test_refused_update_changes_nothing(self):
        before = self.store.get_listing("kleinanzeigen", "1")
        with self.assertRaises(LookupError):
            apply_update(self.store, "kleinanzeigen", "2", price="90 €", observed_at=at(1))
        for kwargs in ({"price": "neunzig"}, {"currency": "EUR"}, {"price": "90", "currency": "USD"}, {"status": "gone"}):
            with self.subTest(kwargs=kwargs):
                with self.assertRaises(ValueError):
                    apply_update(self.store, "kleinanzeigen", "1", observed_at=at(1), **kwargs)
        self.assertEqual(self.store.get_listing("kleinanzeigen", "1"), before)
        self.assertEqual(self.history(), [10000])


if __name__ == "__main__":
    unittest.main()
