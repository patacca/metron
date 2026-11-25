"""Updates the watchers send when they re-check a listing the store already holds.

A watcher polls the page of a listing the search agents are interested in
and reports what changed: a new price, or that the item was sold or the
listing removed. Unlike a full scrape, an update carries only those fields.
"""

from __future__ import annotations

from datetime import datetime

from .normalize import check_status, listing_key, utc_timestamp
from .prices import minor_unit_factor, parse_amount
from .store import ListingStore


def apply_update(
    store: ListingStore,
    backend: str,
    external_id: str | int,
    *,
    price: str | int | float | None = None,
    currency: str | None = None,
    status: str | None = None,
    observed_at: datetime | str | None = None,
) -> bool:
    """Record what a watcher observed on a known listing's page.

    price is read in currency, or in the listing's stored currency when
    currency is None. A changed price is stored and appended to the price
    history. status, when given, replaces the stored status. The listing's
    last_seen_at becomes observed_at in any case.

    Returns whether the price changed. Raises LookupError for a listing the
    store does not hold and ValueError for an invalid value; either way
    nothing is changed.
    """
    backend, external_id = listing_key(backend, external_id)
    if status is not None:
        check_status(status)
    if price is None and currency is not None:
        raise ValueError("currency given without a price: pass the price it belongs to")
    observed = utc_timestamp(observed_at)

    with store.transaction() as conn:
        row = conn.execute(
            "SELECT id, price_minor, currency FROM listings WHERE backend = ? AND external_id = ?",
            (backend, external_id),
        ).fetchone()
        if row is None:
            raise LookupError(
                f"no listing {backend}/{external_id} in the store: store it with upsert_listing or import_records first"
            )

        changed = False
        if price is not None:
            if currency is None:
                currency = row["currency"]
            amount = parse_amount(price, currency)
            new_price = int(amount * minor_unit_factor(currency))
            if (new_price, currency) != (row["price_minor"], row["currency"]):
                conn.execute(
                    "UPDATE listings SET price_minor = ?, currency = ? WHERE id = ?",
                    (new_price, currency, row["id"]),
                )
                conn.execute(
                    "INSERT INTO price_history (listing_id, price_minor, currency, observed_at) "
                    "VALUES (?, ?, ?, ?)",
                    (row["id"], new_price, currency, observed),
                )
                changed = True

        conn.execute(
            "UPDATE listings SET last_seen_at = ?, status = COALESCE(?, status) WHERE id = ?",
            (observed, status, row["id"]),
        )
    return changed
