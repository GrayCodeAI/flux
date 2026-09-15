package catalog

import "errors"

// ErrCatalogCacheRequired is returned when no valid ~/.flux/model_catalog.json exists.
// Run catalog discovery (rho models refresh / flux catalog discover) to populate the cache.
var ErrCatalogCacheRequired = errors.New("model catalog cache required")
