package controlplane

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxManifestBytes = 16 << 20
	maxPeerEndpoints = 32
)

// SignedManifest authenticates a complete manifest. The signature covers the
// canonical JSON encoding of Manifest; KeyID selects a pinned trusted key.
type SignedManifest struct {
	Manifest  Manifest `json:"manifest"`
	KeyID     string   `json:"key_id"`
	Signature []byte   `json:"signature"`
}

// SignedHandler serves manifests that remote peers can authenticate. The host
// should also use TLS and restrict access to its peer network.
func (r *Replica) SignedHandler(keyID string, privateKey ed25519.PrivateKey) (http.Handler, error) {
	if keyID == "" || len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("controlplane: signing key ID and Ed25519 private key required")
	}
	privateKey = append(ed25519.PrivateKey(nil), privateKey...)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r == nil {
			http.Error(w, "manifest unavailable", http.StatusServiceUnavailable)
			return
		}
		current := r.current.Load()
		if current == nil {
			http.Error(w, "manifest unavailable", http.StatusServiceUnavailable)
			return
		}
		payload, err := json.Marshal(current.manifest)
		if err != nil {
			http.Error(w, "manifest encoding failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(SignedManifest{
			Manifest: current.manifest, KeyID: keyID,
			Signature: ed25519.Sign(privateKey, payload),
		})
	}), nil
}

// PeerSource reads manifests from independent peers. It chooses the highest
// valid revision and rejects conflicting content at that revision. Remote
// peers require HTTPS and an Ed25519 key pinned by the caller.
type PeerSource struct {
	urls   []string
	keys   map[string]ed25519.PublicKey
	client *http.Client
}

func NewPeerSource(endpoints []string, trustedKeys map[string]ed25519.PublicKey) (*PeerSource, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("controlplane: at least one peer URL is required")
	}
	if len(endpoints) > maxPeerEndpoints {
		return nil, fmt.Errorf("controlplane: at most %d peer URLs are supported", maxPeerEndpoints)
	}
	urls := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
			return nil, fmt.Errorf("controlplane: invalid peer URL %q", endpoint)
		}
		host := u.Hostname()
		ip := net.ParseIP(host)
		local := strings.EqualFold(host, "localhost") || ip != nil && ip.IsLoopback()
		if u.Scheme != "https" && !(u.Scheme == "http" && local) {
			return nil, fmt.Errorf("controlplane: peer %q requires HTTPS", endpoint)
		}
		if !local && len(trustedKeys) == 0 {
			return nil, fmt.Errorf("controlplane: remote peers require trusted signing keys")
		}
		urls = append(urls, endpoint)
	}
	keys := make(map[string]ed25519.PublicKey, len(trustedKeys))
	for id, key := range trustedKeys {
		if id == "" || len(key) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("controlplane: invalid trusted key %q", id)
		}
		keys[id] = append(ed25519.PublicKey(nil), key...)
	}
	return &PeerSource{urls: urls, keys: keys, client: &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return fmt.Errorf("controlplane: peer redirects are not allowed")
		},
	}}, nil
}

// Latest polls peers concurrently. A failed or stale peer does not prevent a
// valid newer peer from serving updates. If valid peers disagree at the newest
// revision, the update is rejected instead of choosing arbitrarily.
func (p *PeerSource) Latest(ctx context.Context) (Manifest, error) {
	if p == nil || len(p.urls) == 0 {
		return Manifest{}, fmt.Errorf("controlplane: peer source is not initialized")
	}
	type result struct {
		manifest Manifest
		err      error
	}
	results := make(chan result, len(p.urls))
	for _, endpoint := range p.urls {
		go func() {
			manifest, err := p.fetch(ctx, endpoint)
			results <- result{manifest: manifest, err: err}
		}()
	}
	var best Manifest
	var bestDigest [32]byte
	conflict := false
	var firstErr error
	for range p.urls {
		result := <-results
		if result.err != nil {
			if firstErr == nil {
				firstErr = result.err
			}
			continue
		}
		if result.manifest.Revision == 0 {
			if firstErr == nil {
				firstErr = fmt.Errorf("controlplane: peer returned zero revision")
			}
			continue
		}
		digest, err := manifestDigest(result.manifest)
		if err != nil {
			return Manifest{}, err
		}
		if result.manifest.Revision > best.Revision {
			best, bestDigest = result.manifest, digest
			conflict = false
		} else if result.manifest.Revision == best.Revision && digest != bestDigest {
			conflict = true
		}
	}
	if best.Revision > 0 {
		if conflict {
			return Manifest{}, fmt.Errorf("controlplane: peers disagree at revision %d", best.Revision)
		}
		return best, nil
	}
	if firstErr == nil {
		firstErr = fmt.Errorf("controlplane: no peer returned a valid manifest")
	}
	return Manifest{}, firstErr
}

func (p *PeerSource) fetch(ctx context.Context, endpoint string) (Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Manifest{}, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return Manifest{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Manifest{}, fmt.Errorf("controlplane: peer returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes+1))
	if err != nil {
		return Manifest{}, err
	}
	if len(data) > maxManifestBytes {
		return Manifest{}, fmt.Errorf("controlplane: peer manifest exceeds %d bytes", maxManifestBytes)
	}
	if len(p.keys) == 0 {
		var manifest Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return Manifest{}, err
		}
		return manifest, nil
	}
	var signed SignedManifest
	if err := json.Unmarshal(data, &signed); err != nil {
		return Manifest{}, err
	}
	key := p.keys[signed.KeyID]
	if len(key) != ed25519.PublicKeySize {
		return Manifest{}, fmt.Errorf("controlplane: untrusted signing key %q", signed.KeyID)
	}
	payload, err := json.Marshal(signed.Manifest)
	if err != nil {
		return Manifest{}, err
	}
	if !ed25519.Verify(key, payload, signed.Signature) {
		return Manifest{}, fmt.Errorf("controlplane: invalid manifest signature")
	}
	return signed.Manifest, nil
}
