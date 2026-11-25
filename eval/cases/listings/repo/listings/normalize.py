"""Normalization shared by the ingestion paths, the query and deduplication."""

from __future__ import annotations

import unicodedata
from datetime import datetime, timezone

# The marketplaces the search agents scrape, by the name their scraper uses.
BACKENDS = frozenset(
    {
        "bazos_cz",
        "blocket",
        "dba",
        "finn",
        "gumtree",
        "kleinanzeigen",
        "leboncoin",
        "marktplaats",
        "mercari_jp",
        "olx_pl",
        "ricardo",
        "subito",
        "tori",
        "tutti",
        "tweedehands",
        "wallapop",
        "willhaben",
    }
)

STATUSES = ("active", "sold", "removed")


def normalize_title(title: str) -> str:
    """Return a title in the form used for searching and comparing.

    Compatibility characters are folded (NFKC), case is folded, and runs of
    whitespace become single spaces, so that "iPhone  12 PRO" and
    "iphone 12 pro" compare equal.
    """
    folded = unicodedata.normalize("NFKC", title).casefold()
    return " ".join(folded.split())


def search_terms(query: str | None) -> list[str]:
    """Split a search query into normalized terms. None means no terms."""
    if query is None:
        return []
    if not isinstance(query, str):
        raise ValueError(f"query must be text, not {type(query).__name__}")
    return normalize_title(query).split()


def require_text(field: str, value: object) -> str:
    """Return value stripped of surrounding whitespace, or raise ValueError."""
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{field} must be non-empty text, got {value!r}")
    try:
        value.encode("utf-8")
    except UnicodeEncodeError:
        # A lone surrogate, typically an emoji cut in half by a scraper.
        raise ValueError(f"{field} is not valid Unicode text: {value!r}") from None
    return value.strip()


def optional_text(field: str, value: object) -> str | None:
    """Like require_text, but None and blank text mean "not known"."""
    if value is None or (isinstance(value, str) and not value.strip()):
        return None
    return require_text(field, value)


def listing_key(backend: object, external_id: object) -> tuple[str, str]:
    """Validate and return the (backend, external id) pair that names a listing.

    Scrapers that read numeric identifiers from JSON may pass them as
    integers; they are stored as text so that 123 and "123" name the same
    listing.
    """
    if backend not in BACKENDS:
        raise ValueError(f"unknown backend {backend!r}: use one of {', '.join(sorted(BACKENDS))}")
    if isinstance(external_id, int) and not isinstance(external_id, bool):
        external_id = str(external_id)
    return backend, require_text("external_id", external_id)


def check_status(status: object) -> str:
    """Return status if it is one of STATUSES, or raise ValueError."""
    if status not in STATUSES:
        raise ValueError(f"unknown status {status!r}: use one of {', '.join(STATUSES)}")
    return status


def utc_timestamp(value: datetime | str | None = None) -> str:
    """Return the stored form of a point in time: UTC, microseconds, a trailing Z.

    value is a timezone-aware datetime, an ISO 8601 text with an offset or a
    trailing Z, or None for now. Naive datetimes are refused: the scrapers run
    in several time zones and a naive time cannot be placed.
    """
    if value is None:
        value = datetime.now(timezone.utc)
    elif isinstance(value, str):
        text = value.strip()
        if text.endswith(("Z", "z")):
            text = text[:-1] + "+00:00"
        try:
            value = datetime.fromisoformat(text)
        except ValueError:
            raise ValueError(f"cannot read {value!r} as an ISO 8601 time") from None
    if not isinstance(value, datetime):
        raise ValueError(f"a time must be a datetime or ISO 8601 text, not {type(value).__name__}")
    if value.tzinfo is None or value.utcoffset() is None:
        raise ValueError(f"time {value.isoformat()} has no UTC offset: pass a timezone-aware time")
    return value.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")
