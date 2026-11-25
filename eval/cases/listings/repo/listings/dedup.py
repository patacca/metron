"""Finding the same item listed on several marketplaces.

Sellers often post one item on two or three marketplaces at once. The search
agents use find_duplicates() to show such an item once, with links to each
posting. Two active listings are taken to be the same item when they are on
different backends, in the same category and currency, close in price, and
have similar titles.
"""

from __future__ import annotations

from difflib import SequenceMatcher

from .normalize import listing_key, normalize_title
from .store import ListingStore

# Titles at least this similar (difflib ratio of the normalized titles, from
# 0 to 1) count as the same item.
SIMILARITY_THRESHOLD = 0.82

# Prices may differ by up to this many percent of the listing's own price.
PRICE_TOLERANCE_PERCENT = 15


def title_similarity(a: str, b: str) -> float:
    """Return how similar two titles are, from 0.0 (nothing shared) to 1.0 (equal)."""
    return SequenceMatcher(None, normalize_title(a), normalize_title(b)).ratio()


def find_duplicates(
    store: ListingStore,
    backend: str,
    external_id: str | int,
    *,
    threshold: float = SIMILARITY_THRESHOLD,
) -> list[dict]:
    """Return the active listings on other backends that are probably the same item.

    Each entry has backend, external_id, title, price_minor and score, the
    title similarity rounded to three decimals. The most similar come first;
    equal scores are ordered by backend and external id.
    """
    backend, external_id = listing_key(backend, external_id)
    listing = store.get_listing(backend, external_id)
    if listing is None:
        raise LookupError(f"no listing {backend}/{external_id} in the store")

    price = listing["price_minor"]
    lowest = price * (100 - PRICE_TOLERANCE_PERCENT) // 100
    highest = price * (100 + PRICE_TOLERANCE_PERCENT) // 100
    candidates = store.connection.execute(
        "SELECT backend, external_id, title, price_minor FROM listings "
        "WHERE category = ? AND currency = ? AND status = 'active' AND backend <> ? "
        "AND price_minor BETWEEN ? AND ?",
        (listing["category"], listing["currency"], backend, lowest, highest),
    )

    matches = []
    for candidate in candidates:
        score = title_similarity(listing["title"], candidate["title"])
        if score >= threshold:
            matches.append(
                {
                    "backend": candidate["backend"],
                    "external_id": candidate["external_id"],
                    "title": candidate["title"],
                    "price_minor": candidate["price_minor"],
                    "score": round(score, 3),
                }
            )
    matches.sort(key=lambda match: (-match["score"], match["backend"], match["external_id"]))
    return matches
