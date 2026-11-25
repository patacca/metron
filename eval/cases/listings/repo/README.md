# listings

The store behind the search agents. The scrapers for our 17 second-hand
marketplaces write every listing they see into one SQLite file; the search
agents read it to find the cheapest offers for what a buyer is looking for.

Python 3.10 or later, standard library only.

## What it keeps

- **Listings**, keyed by backend (the marketplace, as its scraper names it:
  `kleinanzeigen`, `blocket`, `mercari_jp`, ...) and the marketplace's own id
  for the listing. Title, category, seller, URL, current price, status
  (`active`, `sold`, `removed`), and when the listing was first and last seen.
- **Price history**: every price a listing has had, with when it was
  observed. The search agents use it to spot price drops.

Prices are integers in the currency's minor unit: `129900` EUR is
1,299.00 €, `1500` JPY is ¥1,500. Supported currencies are EUR, GBP, CHF,
SEK, NOK, DKK, PLN, CZK and JPY.

## Using it

```python
from listings import open_store, apply_update, import_jsonl, find_duplicates

with open_store("listings.db") as store:
    # A scraper saw a listing (new or already known).
    store.upsert_listing(
        "kleinanzeigen", "2861734412",
        title="Trek Marlin 7, 2022, Rahmen M", price="649 €", currency="EUR",
        url="https://www.kleinanzeigen.de/s-anzeige/2861734412", category="bikes",
        seller="Jonas",
    )

    # A watcher re-checked the listing's page.
    apply_update(store, "kleinanzeigen", "2861734412", price="599 €")
    apply_update(store, "kleinanzeigen", "2861734412", status="sold")

    # A scraper's dump of a whole search run, one JSON object per line.
    import_jsonl(store, "dumps/blocket-2026-11-20.jsonl")

    # What the search agents ask.
    offers = store.cheapest_active("bikes", currency="EUR", query="trek marlin", limit=20)
    same_item = find_duplicates(store, "kleinanzeigen", "2861734412")
    history = store.price_history("kleinanzeigen", "2861734412")
```

There are three ways in, one per kind of scraper:

| Path | Module | Used by |
| --- | --- | --- |
| `ListingStore.upsert_listing` | `listings/store.py` | the live scrapers, one listing at a time |
| `apply_update` | `listings/updates.py` | the watchers that poll listings the agents are following |
| `import_records`, `import_jsonl` | `listings/bulk.py` | dumps of a full search run |

Prices can be given as the marketplace shows them (`"1.299,00 €"`,
`"£1,299.99"`, `"1 299 kr"`, `"CHF 1'299.90"`, `"¥1,500"`) or as a number
from the marketplace's JSON. `listings/prices.py` documents the rules.

Times are timezone-aware `datetime`s or ISO 8601 text with an offset; they
default to now. They are stored as UTC text.

Other operations: `get_listing`, `delete_listing`, `purge_removed` (deletes
listings marked removed and not seen since a given time) and `stats`.

## Schema

`listings/schema.py` holds the numbered migrations. `open_store` applies the
pending ones, so opening a database created by an older version upgrades
it. Never edit a migration that has shipped; add a new one.

The database uses write-ahead logging, so the search agents can query while
the ingestion process writes.

## Tests

```
python3 -m unittest discover -s tests -q
```

The tests create their databases in temporary directories and need no
network. One of them builds a 50,000-listing store to time the search query,
so the suite takes a few seconds.
