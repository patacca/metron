import unittest

from listings.prices import format_minor, minor_unit_factor, parse_amount


class ParseAmountTest(unittest.TestCase):
    def test_decimal_point_or_comma(self):
        for text in ("12.50", "12,50", "12,5", "12.5"):
            with self.subTest(text=text):
                self.assertEqual(parse_amount(text, "EUR"), 12.5)
        self.assertEqual(parse_amount("250", "EUR"), 250.0)
        self.assertEqual(parse_amount("0", "EUR"), 0.0)

    def test_thousands_grouping(self):
        for text in ("1.299,00", "1,299.00", "1 299,00", "1 299,00", "1 299,00", "1'299.00", "1.299", "1,299"):
            with self.subTest(text=text):
                self.assertEqual(parse_amount(text, "EUR"), 1299.0)
        self.assertEqual(parse_amount("100.000", "EUR"), 100000.0)
        self.assertEqual(parse_amount("12.345,75", "EUR"), 12345.75)

    def test_currency_symbols_and_codes(self):
        cases = [
            ("1.299,00 €", "EUR"),
            ("€ 1.299,00", "EUR"),
            ("EUR 1299", "EUR"),
            ("£1,299.00", "GBP"),
            ("1 299 kr", "SEK"),
            ("1 299 kr", "NOK"),
            ("kr. 1.299,00", "DKK"),
            ("1.299 kr.", "DKK"),
            ("CHF 1'299.00", "CHF"),
            ("Fr. 1'299.00", "CHF"),
            ("1 299 Kč", "CZK"),
            ("1.299,00 zł", "PLN"),
        ]
        for text, currency in cases:
            with self.subTest(text=text, currency=currency):
                self.assertEqual(parse_amount(text, currency), 1299.0)

    def test_yen_has_no_minor_unit(self):
        self.assertEqual(parse_amount("¥1,500", "JPY"), 1500.0)
        self.assertEqual(parse_amount("1,500円", "JPY"), 1500.0)
        with self.assertRaises(ValueError):
            parse_amount("1.5", "JPY")

    def test_numbers_from_json(self):
        self.assertEqual(parse_amount(1299, "EUR"), 1299.0)
        self.assertEqual(parse_amount(12.5, "EUR"), 12.5)
        self.assertEqual(parse_amount(0, "SEK"), 0.0)

    def test_rejects_what_is_not_a_price(self):
        for price in ("", "free", "-5", "€ -5", "1.299.00", ".50", "12,5,0", "0.299", "12.9999", "1 2345",
                      True, None, -1, float("inf"), float("nan"), ["12"]):
            with self.subTest(price=price):
                with self.assertRaises(ValueError):
                    parse_amount(price, "EUR")

    def test_rejects_unsupported_currencies(self):
        for currency in ("USD", "eur", "", None):
            with self.subTest(currency=currency):
                with self.assertRaises(ValueError):
                    parse_amount("12,50", currency)

    def test_minor_units(self):
        self.assertEqual(minor_unit_factor("EUR"), 100)
        self.assertEqual(minor_unit_factor("JPY"), 1)
        self.assertEqual(format_minor(129900, "EUR"), "1299.00 EUR")
        self.assertEqual(format_minor(5, "SEK"), "0.05 SEK")
        self.assertEqual(format_minor(1500, "JPY"), "1500 JPY")


if __name__ == "__main__":
    unittest.main()
