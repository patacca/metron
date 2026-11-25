"""Prices as the marketplaces show them, and the currencies the store accepts.

Prices are stored as integers in the minor unit of their currency (cents for
EUR, öre for SEK, yen for JPY). This module reads a scraped price, either a
number taken from a marketplace's JSON or the text its pages display, and
returns the amount it stands for.

Marketplaces format numbers the local way: "1.299,00 €" in Germany,
"£1,299.00" in the UK, "1 299 kr" in Sweden, "CHF 1'299.90" in Switzerland.
The rule that reads all of them: a separator followed by one or two digits at
the end is the decimal point, and any other separator groups thousands.
"""

from __future__ import annotations

import math
import re

# Digits after the decimal point in each currency's minor unit (ISO 4217).
CURRENCY_EXPONENTS = {
    "CHF": 2,
    "CZK": 2,
    "DKK": 2,
    "EUR": 2,
    "GBP": 2,
    "JPY": 0,
    "NOK": 2,
    "PLN": 2,
    "SEK": 2,
}

# Symbols and codes the marketplaces put before or after the amount.
CURRENCY_SYMBOLS = {
    "CHF": ("CHF", "SFr.", "Fr."),
    "CZK": ("CZK", "Kč"),
    "DKK": ("DKK", "kr.", "kr"),
    "EUR": ("EUR", "€"),
    "GBP": ("GBP", "£"),
    "JPY": ("JPY", "¥", "円"),
    "NOK": ("NOK", "kr"),
    "PLN": ("PLN", "zł"),
    "SEK": ("SEK", "kr"),
}

# An amount: digits, optionally grouped in thousands by one consistent
# separator, then optionally a decimal point and a fraction.
_AMOUNT = re.compile(
    r"(?P<whole>[1-9]\d{0,2}(?P<group>[.,'   ’])\d{3}(?:(?P=group)\d{3})*|\d+)"
    r"(?:(?P<point>[.,])(?P<fraction>\d+))?"
)


def currency_exponent(currency: object) -> int:
    """Return the number of minor-unit digits of currency, or raise ValueError."""
    if currency not in CURRENCY_EXPONENTS:
        supported = ", ".join(sorted(CURRENCY_EXPONENTS))
        raise ValueError(f"unsupported currency {currency!r}: use one of {supported}")
    return CURRENCY_EXPONENTS[currency]


def minor_unit_factor(currency: object) -> int:
    """Return how many minor units make one major unit of currency."""
    return 10 ** currency_exponent(currency)


def parse_amount(price: object, currency: object) -> float:
    """Return the amount a scraped price stands for, in major units of currency.

    price is a non-negative number, or text such as "19,99 €", "1.299,00",
    "£1,299.99", "1 299 kr" or "¥1,500". The text may carry the currency's
    symbol or code before or after the amount. Raises ValueError for
    anything else, including a fraction finer than the currency's minor unit.
    """
    exponent = currency_exponent(currency)
    if isinstance(price, bool):
        raise ValueError(f"price {price!r} is not a number")
    if isinstance(price, (int, float)):
        if not math.isfinite(price) or price < 0:
            raise ValueError(f"price {price!r} is not a non-negative amount")
        return float(price)
    if not isinstance(price, str):
        raise ValueError(f"price must be a number or text, not {type(price).__name__}")

    text = _strip_symbol(price.strip(), currency)
    match = _AMOUNT.fullmatch(text)
    if match is None:
        raise ValueError(f"cannot read {price!r} as a {currency} price")
    if match["point"] is not None and match["point"] == match["group"]:
        raise ValueError(f"{price!r} uses {match['point']!r} both to group thousands and as the decimal point")
    fraction = match["fraction"] or ""
    if len(fraction) > exponent:
        raise ValueError(f"{price!r} has more decimals than {currency} has minor units ({exponent})")
    whole = re.sub(r"\D", "", match["whole"])
    return float(f"{whole}.{fraction}" if fraction else whole)


def _strip_symbol(text: str, currency: str) -> str:
    """Remove the currency's symbol or code from either end of text."""
    for symbol in sorted(CURRENCY_SYMBOLS[currency], key=len, reverse=True):
        if text.startswith(symbol):
            return text[len(symbol) :].strip()
        if text.endswith(symbol):
            return text[: -len(symbol)].strip()
    return text


def format_minor(price_minor: int, currency: str) -> str:
    """Format a stored price for display, for example 129900 EUR as "1299.00 EUR"."""
    exponent = currency_exponent(currency)
    if exponent == 0:
        return f"{price_minor} {currency}"
    whole, fraction = divmod(price_minor, 10**exponent)
    return f"{whole}.{fraction:0{exponent}d} {currency}"
