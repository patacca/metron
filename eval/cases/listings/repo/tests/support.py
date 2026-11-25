"""Shared test helpers: a fresh store in a temporary directory, and fixed times."""

from __future__ import annotations

import os
import tempfile
import unittest
from datetime import datetime, timedelta, timezone

from listings import open_store

T0 = datetime(2026, 11, 20, 9, 0, tzinfo=timezone.utc)


def at(hours: float) -> datetime:
    """Return the time `hours` after T0."""
    return T0 + timedelta(hours=hours)


def stamp(hours: float) -> str:
    """Return the stored form of at(hours)."""
    return at(hours).strftime("%Y-%m-%dT%H:%M:%S.%fZ")


def bike(**overrides: object) -> dict:
    """Return the keyword arguments of upsert_listing for a typical listing."""
    fields = {
        "title": "Trek Marlin 7, 2022, Rahmen M",
        "price": "649 €",
        "currency": "EUR",
        "url": "https://www.kleinanzeigen.de/s-anzeige/2861734412",
        "category": "bikes",
        "seller": "Jonas",
        "seen_at": T0,
    }
    fields.update(overrides)
    return fields


def record(external_id: object, **overrides: object) -> dict:
    """Return a bulk import record for a listing on willhaben."""
    fields = {
        "backend": "willhaben",
        "external_id": external_id,
        "title": f"Cube Aim Race {external_id}",
        "price": "450,00",
        "currency": "EUR",
        "url": f"https://www.willhaben.at/iad/kaufen-und-verkaufen/d/{external_id}",
        "category": "bikes",
    }
    fields.update(overrides)
    return fields


class StoreTestCase(unittest.TestCase):
    """A test with self.store open on a new database file."""

    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.path = os.path.join(directory.name, "listings.db")
        self.store = open_store(self.path)
        self.addCleanup(self.store.close)
