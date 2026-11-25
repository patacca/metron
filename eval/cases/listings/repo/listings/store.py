"""The listings store: one SQLite file with scraped listings and their prices."""

from __future__ import annotations

import os
import sqlite3
from collections.abc import Iterator
from contextlib import contextmanager
from datetime import datetime

from .normalize import (
    check_status,
    listing_key,
    normalize_title,
    optional_text,
    require_text,
    search_terms,
    utc_timestamp,
)
from .prices import currency_exponent, minor_unit_factor, parse_amount
from .schema import migrate

# The most rows cheapest_active returns at once.
MAX_RESULTS = 100

_LISTING_COLUMNS = (
    "id, backend, external_id, title, category, seller, url, price_minor, currency, "
    "status, first_seen_at, last_seen_at"
)


def open_store(path: str | os.PathLike[str]) -> ListingStore:
    """Open the store in the SQLite file at path, creating or upgrading it.

    The file uses write-ahead logging, so the search agents can read while
    the ingestion process writes.
    """
    connection = sqlite3.connect(path, timeout=10.0, isolation_level=None)
    try:
        connection.row_factory = sqlite3.Row
        connection.execute("PRAGMA foreign_keys = ON")
        connection.execute("PRAGMA journal_mode = WAL")
        migrate(connection)
    except BaseException:
        connection.close()
        raise
    return ListingStore(connection)


class ListingStore:
    """Listings keyed by (backend, external id), with the history of their prices.

    Use open_store() to get one. The ingestion paths are upsert_listing()
    here, listings.updates.apply_update() and listings.bulk.import_records().
    """

    def __init__(self, connection: sqlite3.Connection) -> None:
        self.connection = connection

    def close(self) -> None:
        self.connection.close()

    def __enter__(self) -> ListingStore:
        return self

    def __exit__(self, *exc_info: object) -> None:
        self.close()

    @contextmanager
    def transaction(self) -> Iterator[sqlite3.Connection]:
        """Run the block in one transaction: all of its writes or none."""
        self.connection.execute("BEGIN")
        try:
            yield self.connection
        except BaseException:
            if self.connection.in_transaction:
                self.connection.execute("ROLLBACK")
            raise
        try:
            self.connection.execute("COMMIT")
        except BaseException:
            if self.connection.in_transaction:
                self.connection.execute("ROLLBACK")
            raise

    def upsert_listing(
        self,
        backend: str,
        external_id: str | int,
        *,
        title: str,
        price: str | int | float,
        currency: str,
        url: str,
        category: str,
        seller: str | None = None,
        status: str = "active",
        seen_at: datetime | str | None = None,
    ) -> int:
        """Store a listing as a scraper just saw it, and return its id.

        A listing seen for the first time is inserted. A listing seen again
        gets the scraped values; it keeps its id and first_seen_at. A new
        price, or a first price, is appended to the price history.
        """
        backend, external_id = listing_key(backend, external_id)
        title = require_text("title", title)
        url = require_text("url", url)
        category = require_text("category", category)
        seller = optional_text("seller", seller)
        check_status(status)
        price_minor = int(parse_amount(price, currency) * minor_unit_factor(currency))
        seen = utc_timestamp(seen_at)

        with self.transaction() as conn:
            previous = conn.execute(
                "SELECT id, price_minor, currency, first_seen_at FROM listings "
                "WHERE backend = ? AND external_id = ?",
                (backend, external_id),
            ).fetchone()
            listing_id = None if previous is None else previous["id"]
            first_seen = seen if previous is None else previous["first_seen_at"]
            cursor = conn.execute(
                "INSERT OR REPLACE INTO listings (id, backend, external_id, title, search_text, "
                "category, seller, url, price_minor, currency, status, first_seen_at, last_seen_at) "
                "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                (
                    listing_id,
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
                    first_seen,
                    seen,
                ),
            )
            listing_id = cursor.lastrowid
            if previous is None or (previous["price_minor"], previous["currency"]) != (price_minor, currency):
                conn.execute(
                    "INSERT INTO price_history (listing_id, price_minor, currency, observed_at) "
                    "VALUES (?, ?, ?, ?)",
                    (listing_id, price_minor, currency, seen),
                )
        return listing_id

    def get_listing(self, backend: str, external_id: str | int) -> dict | None:
        """Return the listing as a dict, or None if the store does not hold it."""
        backend, external_id = listing_key(backend, external_id)
        row = self.connection.execute(
            f"SELECT {_LISTING_COLUMNS} FROM listings WHERE backend = ? AND external_id = ?",
            (backend, external_id),
        ).fetchone()
        return None if row is None else dict(row)

    def price_history(self, backend: str, external_id: str | int) -> list[dict]:
        """Return the listing's prices, oldest first, each with when it was observed.

        Each entry has observed_at, price_minor and currency. An unknown
        listing has no history.
        """
        backend, external_id = listing_key(backend, external_id)
        rows = self.connection.execute(
            "SELECT h.observed_at, h.price_minor, h.currency FROM price_history AS h "
            "JOIN listings AS l ON l.id = h.listing_id "
            "WHERE l.backend = ? AND l.external_id = ? ORDER BY h.observed_at, h.id",
            (backend, external_id),
        ).fetchall()
        return [dict(row) for row in rows]

    def delete_listing(self, backend: str, external_id: str | int) -> bool:
        """Delete a listing and its price history. Return whether it existed."""
        backend, external_id = listing_key(backend, external_id)
        with self.transaction() as conn:
            cursor = conn.execute(
                "DELETE FROM listings WHERE backend = ? AND external_id = ?",
                (backend, external_id),
            )
        return cursor.rowcount > 0

    def purge_removed(self, before: datetime | str) -> int:
        """Delete listings marked removed and not seen since before.

        Their price history goes with them. Returns how many were deleted.
        """
        cutoff = utc_timestamp(before)
        with self.transaction() as conn:
            cursor = conn.execute(
                "DELETE FROM listings WHERE status = 'removed' AND last_seen_at < ?",
                (cutoff,),
            )
        return cursor.rowcount

    def cheapest_active(
        self,
        category: str,
        *,
        currency: str,
        query: str | None = None,
        limit: int = 20,
    ) -> list[dict]:
        """Return the cheapest active listings of a category priced in currency.

        With a query, only listings whose title contains every term of the
        query, ignoring case, are returned. Ties in price are broken by id,
        so the order is stable.
        """
        category = require_text("category", category)
        currency_exponent(currency)
        if isinstance(limit, bool) or not isinstance(limit, int) or not 1 <= limit <= MAX_RESULTS:
            raise ValueError(f"limit must be an integer from 1 to {MAX_RESULTS}, got {limit!r}")
        sql = (
            f"SELECT {_LISTING_COLUMNS} FROM listings "
            "WHERE category = ? AND status = 'active' AND currency = ?"
        )
        params: list[object] = [category, currency]
        for term in search_terms(query):
            sql += " AND instr(search_text, ?) > 0"
            params.append(term)
        sql += " ORDER BY price_minor, id LIMIT ?"
        params.append(limit)
        return [dict(row) for row in self.connection.execute(sql, params)]

    def stats(self) -> dict:
        """Return what the store holds: listings per backend and status, and totals."""
        by_backend: dict[str, dict[str, int]] = {}
        total = 0
        rows = self.connection.execute(
            "SELECT backend, status, COUNT(*) AS n FROM listings "
            "GROUP BY backend, status ORDER BY backend, status"
        )
        for row in rows:
            by_backend.setdefault(row["backend"], {})[row["status"]] = row["n"]
            total += row["n"]
        history = self.connection.execute("SELECT COUNT(*) FROM price_history").fetchone()[0]
        return {"listings": total, "by_backend": by_backend, "price_history_rows": history}
