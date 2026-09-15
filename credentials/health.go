package credentials

import (
	"context"
	"errors"
)

// StorageStatus reports whether the credential store can be read (and optionally written).
func StorageStatus(ctx context.Context) (ok bool, detail string) {
	if ctx == nil {
		ctx = context.Background()
	}
	store := DefaultStore()
	if store == nil {
		return false, "credential store not initialized"
	}
	_, err := store.Get(ctx, AccountForEnv("___FLUX_STORAGE_PROBE___"))
	if err != nil && !errors.Is(err, ErrNotFound) {
		return false, err.Error()
	}
	return true, PlatformSecretStoreName() + " reachable"
}

// KeychainWriteAvailable reports whether secrets can be persisted to the OS keychain.
func KeychainWriteAvailable(ctx context.Context) (ok bool, detail string) {
	if ctx == nil {
		ctx = context.Background()
	}
	cs, okStore := DefaultStore().(*CombinedStore)
	if !okStore || cs.Keychain == nil {
		return false, PlatformSecretStoreName() + " unavailable"
	}
	if err := cs.Keychain.Set(ctx, "___hawk_write_probe___", "probe"); err != nil {
		return false, err.Error()
	}
	_ = cs.Keychain.Delete(ctx, "___hawk_write_probe___")
	return true, PlatformSecretStoreName() + " writable"
}
