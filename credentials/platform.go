package credentials

import (
	"fmt"
	"runtime"
)

// PlatformSecretStoreName is the user-facing label for the OS credential backend.
func PlatformSecretStoreName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS Keychain"
	case "linux":
		return "Linux secret store (GNOME Keyring / KWallet)"
	case "windows":
		return "Windows Credential Manager"
	default:
		return "OS secret store"
	}
}

// KeyringUnavailableHelp explains how to enable secret storage on this OS.
func KeyringUnavailableHelp() string {
	switch runtime.GOOS {
	case "linux":
		return "install and unlock a Secret Service provider (e.g. gnome-keyring or KWallet). " +
			"Ensure DBUS_SESSION_BUS_ADDRESS is set in your shell; on headless systems run: eval $(gnome-keyring-daemon --start --components=secrets)"
	case "darwin":
		return "allow Keychain access when macOS prompts for Rho"
	case "windows":
		return "ensure Windows Credential Manager is available"
	default:
		return "configure your OS secret store"
	}
}

// ErrKeychainUnavailable is returned when credentials cannot be stored in the OS secret store.
var ErrKeychainUnavailable = fmt.Errorf("credentials: OS secret store unavailable")

// KeychainUnavailableDetail returns a detailed error explaining why the keychain is unavailable.
func KeychainUnavailableDetail() error {
	return fmt.Errorf("credentials: %s unavailable — %s",
		PlatformSecretStoreName(), KeyringUnavailableHelp())
}
