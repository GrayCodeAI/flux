package catalog

import "errors"

// ErrCatalogCacheRequired is returned when no valid ~/.graycode-router/model_catalog.json exists.
// Run catalog discovery (hawk models refresh / graycode-router catalog discover) to populate the cache.
var ErrCatalogCacheRequired = errors.New("model catalog cache required")
