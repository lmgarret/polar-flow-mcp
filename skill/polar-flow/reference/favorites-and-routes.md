# Favorites and routes

A **favorite** is a reusable workout template with no date. A **route** is an
imported GPS track. Polar stores routes as favorites of type `ROUTE`, so the
list, rename, sport and delete tools work on both.

## Favorite or target?

- One workout on a date → a **target** (`create_training_target`).
- Something to reuse ("my Tuesday intervals", "my usual long run") → a
  **favorite** (`create_favorite`).
- Template → diary: `schedule_favorite` creates a **new target** and returns
  its `target_id`. Edit that target with the target tools. The favorite stays
  unchanged.
- Diary → template: `save_target_as_favorite`. The copy isn't linked back to
  the target, so calling it twice makes two favorites.

## The two ids

`list_favorites` returns both ids for every entry:

- `favorite_id` works with every favorite tool.
- `exercise_target_id` is what Polar uses to find route geometry. `get_route`
  accepts either id. `set_favorite_sport` needs it only for multi-sport
  favorites.

## Editing

Pick the narrowest tool for the change:

| Change | Tool |
|---|---|
| Name only | `rename_favorite` (templates and routes) |
| Sport only | `set_favorite_sport` (templates and routes) |
| Goal, description or several fields | `get_favorite` → `update_favorite` with everything that should stay. Not available for routes or multi-sport favorites |

`get_favorite` on a PHASED favorite returns the same raw phase format as
`get_training_target`. Translate it the same way (see
[training-targets.md](training-targets.md#editing-turning-the-read-back-into-update-arguments)).

## Routes

Only import a file the user actually provided. Never make up a GPS track.
`get_favorite` on a route returns only its name, sport and distance. Use
`get_route` for the points.
