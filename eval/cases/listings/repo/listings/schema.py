"""Database schema, created and upgraded by numbered migrations.

The schema version is kept in SQLite's user_version. Each migration runs in
its own transaction together with the version bump, so a database is always
at exactly one version. Migrations are append-only: never edit one that has
shipped, add a new one.
"""

from __future__ import annotations

import sqlite3

MIGRATIONS: tuple[tuple[int, tuple[str, ...]], ...] = (
    (
        1,
        (
            """
            CREATE TABLE listings (
                id INTEGER PRIMARY KEY,
                backend TEXT NOT NULL,
                external_id TEXT NOT NULL,
                title TEXT NOT NULL,
                search_text TEXT NOT NULL,
                category TEXT NOT NULL,
                url TEXT NOT NULL,
                price_minor INTEGER NOT NULL CHECK (price_minor >= 0),
                currency TEXT NOT NULL,
                first_seen_at TEXT NOT NULL,
                last_seen_at TEXT NOT NULL,
                UNIQUE (backend, external_id)
            )
            """,
        ),
    ),
    (
        2,
        (
            """
            CREATE TABLE price_history (
                id INTEGER PRIMARY KEY,
                listing_id INTEGER NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
                price_minor INTEGER NOT NULL CHECK (price_minor >= 0),
                currency TEXT NOT NULL,
                observed_at TEXT NOT NULL
            )
            """,
            "CREATE INDEX price_history_by_listing ON price_history (listing_id, observed_at)",
            # Listings stored before there was a history start it with their
            # current price, as last seen.
            """
            INSERT INTO price_history (listing_id, price_minor, currency, observed_at)
            SELECT id, price_minor, currency, last_seen_at FROM listings ORDER BY id
            """,
        ),
    ),
    (
        3,
        (
            """
            ALTER TABLE listings ADD COLUMN status TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'sold', 'removed'))
            """,
            "CREATE INDEX listings_by_category ON listings (category, status)",
        ),
    ),
    (
        4,
        ("ALTER TABLE listings ADD COLUMN seller TEXT",),
    ),
)

SCHEMA_VERSION = MIGRATIONS[-1][0]


def schema_version(connection: sqlite3.Connection) -> int:
    """Return the schema version of the database behind connection."""
    return connection.execute("PRAGMA user_version").fetchone()[0]


def migrate(connection: sqlite3.Connection, target: int = SCHEMA_VERSION) -> None:
    """Bring the database up to schema version target.

    connection must be in autocommit mode (isolation_level=None). Each
    pending migration takes the write lock before it checks the version
    again, so two processes opening a new database at once apply every
    migration exactly once.
    """
    current = schema_version(connection)
    if current > SCHEMA_VERSION:
        raise RuntimeError(
            f"database schema version {current} is newer than this package supports "
            f"({SCHEMA_VERSION}): upgrade the listings package"
        )
    if current >= target:
        return
    for number, statements in MIGRATIONS:
        if number > target:
            break
        connection.execute("BEGIN IMMEDIATE")
        try:
            if schema_version(connection) < number:
                for statement in statements:
                    connection.execute(statement)
                connection.execute(f"PRAGMA user_version = {number}")
        except BaseException:
            connection.execute("ROLLBACK")
            raise
        connection.execute("COMMIT")
