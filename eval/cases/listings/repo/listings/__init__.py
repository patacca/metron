"""Store for the second-hand marketplace listings that the search agents scrape.

    from listings import open_store, apply_update, import_jsonl

    with open_store("listings.db") as store:
        store.upsert_listing("kleinanzeigen", "2861734412", title="Trek Marlin 7, 2022",
                             price="649 €", currency="EUR", category="bikes",
                             url="https://www.kleinanzeigen.de/s-anzeige/2861734412")
        apply_update(store, "kleinanzeigen", "2861734412", price="599 €")
        import_jsonl(store, "dumps/blocket-2026-11-20.jsonl")
        cheapest = store.cheapest_active("bikes", currency="EUR", query="trek marlin")
"""

from .bulk import import_jsonl, import_records
from .dedup import PRICE_TOLERANCE_PERCENT, SIMILARITY_THRESHOLD, find_duplicates, title_similarity
from .normalize import BACKENDS, STATUSES
from .prices import CURRENCY_EXPONENTS, format_minor, parse_amount
from .schema import SCHEMA_VERSION
from .store import MAX_RESULTS, ListingStore, open_store
from .updates import apply_update

__all__ = [
    "BACKENDS",
    "CURRENCY_EXPONENTS",
    "MAX_RESULTS",
    "PRICE_TOLERANCE_PERCENT",
    "SCHEMA_VERSION",
    "SIMILARITY_THRESHOLD",
    "STATUSES",
    "ListingStore",
    "apply_update",
    "find_duplicates",
    "format_minor",
    "import_jsonl",
    "import_records",
    "open_store",
    "parse_amount",
    "title_similarity",
]
