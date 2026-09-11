package catalog

import "errors"

// ErrCatalogCacheRequired is returned when no valid ~/.eyrie/model_catalog.json exists.
// Run catalog discovery (hawk models refresh / eyrie catalog discover) to populate the cache.
var ErrCatalogCacheRequired = errors.New("model catalog cache required")
