"""Bulk import of scraper dumps.

A scraper that walks a marketplace's search pages writes what it saw as a
dump: one record per listing, in JSON Lines. A dump is imported in one
transaction, so a bad record leaves the store exactly as it was.
"""

from __future__ import annotations

import json
import os
import sqlite3
from collections.abc import Iterable, Mapping
from datetime import datetime

from .normalize import (
    check_status,
    listing_key,
    normalize_title,
    optional_text,
    require_text,
    utc_timestamp,
)
from .prices import minor_unit_factor, parse_amount
from .store import ListingStore

REQUIRED_FIELDS = ("backend", "external_id", "title", "price", "currency", "url", "category")
OPTIONAL_FIELDS = ("seller", "status", "seen_at")


def import_records(
    store: ListingStore,
    records: Iterable[Mapping[str, object]],
    *,
    seen_at: datetime | str | None = None,
) -> dict[str, int]:
    """Import scraped records into the store, all of them or none.

    Each record is a mapping with the fields in REQUIRED_FIELDS and
    optionally those in OPTIONAL_FIELDS; the values mean what the keyword
    arguments of ListingStore.upsert_listing mean. A record without its own
    seen_at was seen at seen_at, or now when that is None too.

    Returns the number of listings inserted and updated and of prices
    appended to the price history. Raises ValueError naming the first
    invalid record, by its position counted from 1, and stores nothing.
    """
    default_seen = utc_timestamp(seen_at)
    known_fields = set(REQUIRED_FIELDS) | set(OPTIONAL_FIELDS)
    counts = {"inserted": 0, "updated": 0, "price_changes": 0}
    factors: dict[str, int] = {}

    with store.transaction() as conn:
        for position, record in enumerate(records, start=1):
            try:
                if not isinstance(record, Mapping):
                    raise ValueError(f"expected a mapping of fields, got {type(record).__name__}")
                unknown = sorted(str(field) for field in record if field not in known_fields)
                if unknown:
                    raise ValueError(f"unknown fields {', '.join(unknown)}")
                missing = [field for field in REQUIRED_FIELDS if field not in record]
                if missing:
                    raise ValueError(f"missing fields {', '.join(missing)}")

                backend, external_id = listing_key(record["backend"], record["external_id"])
                title = require_text("title", record["title"])
                url = require_text("url", record["url"])
                category = require_text("category", record["category"])
                seller = optional_text("seller", record.get("seller"))
                status = check_status(record.get("status", "active"))
                record_seen = record.get("seen_at")
                seen = default_seen if record_seen is None else utc_timestamp(record_seen)

                currency = record["currency"]
                if currency not in factors:
                    factors[currency] = minor_unit_factor(currency)
                price_minor = int(parse_amount(record["price"], currency) * factors[currency])

                existing = conn.execute(
                    "SELECT id, price_minor, currency FROM listings WHERE backend = ? AND external_id = ?",
                    (backend, external_id),
                ).fetchone()
                cursor = conn.execute(
                    "INSERT INTO listings (backend, external_id, title, search_text, category, seller, "
                    "url, price_minor, currency, status, first_seen_at, last_seen_at) "
                    "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) "
                    "ON CONFLICT (backend, external_id) DO UPDATE SET "
                    "title = excluded.title, search_text = excluded.search_text, "
                    "category = excluded.category, seller = excluded.seller, url = excluded.url, "
                    "price_minor = excluded.price_minor, currency = excluded.currency, "
                    "status = excluded.status, last_seen_at = excluded.last_seen_at",
                    (
                        backend,
                        external_id,
                        title,
                        normalize_title(title),
                        category,
                        seller,
                        url,
                        price_minor,
                        currency,
                        status,
                        seen,
                        seen,
                    ),
                )
                if existing is None:
                    listing_id = cursor.lastrowid
                    counts["inserted"] += 1
                else:
                    listing_id = existing["id"]
                    counts["updated"] += 1

                if existing is None or (existing["price_minor"], existing["currency"]) != (price_minor, currency):
                    conn.execute(
                        "INSERT INTO price_history (listing_id, price_minor, currency, observed_at) "
                        "VALUES (?, ?, ?, ?)",
                        (listing_id, price_minor, currency, seen),
                    )
                    counts["price_changes"] += 1
            except (TypeError, ValueError, OverflowError, sqlite3.IntegrityError) as error:
                # Text SQLite cannot store (a lone surrogate from a truncated
                # emoji), an integer too large for it, or a constraint the
                # record breaks: all of them are this record's fault.
                raise ValueError(f"record {position}: {error}") from error
    return counts


def import_jsonl(
    store: ListingStore,
    path: str | os.PathLike[str],
    *,
    seen_at: datetime | str | None = None,
) -> dict[str, int]:
    """Import a JSON Lines dump: one JSON object per line, as import_records takes them.

    Record positions in errors are line numbers.
    """

    def records() -> Iterable[object]:
        # Lines are decoded one by one so that an encoding error, like a
        # syntax error, is reported with its line number.
        with open(path, "rb") as dump:
            for number, line in enumerate(dump, start=1):
                try:
                    text = line.decode("utf-8")
                except UnicodeDecodeError as error:
                    raise ValueError(f"{os.fspath(path)}, line {number}: not UTF-8 ({error.reason})") from None
                try:
                    yield json.loads(text)
                except json.JSONDecodeError as error:
                    raise ValueError(f"{os.fspath(path)}, line {number}: not JSON ({error.msg})") from None

    return import_records(store, records(), seen_at=seen_at)
