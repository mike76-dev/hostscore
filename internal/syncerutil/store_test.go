package syncerutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestJSONPeerStoreLoadNilMaps is a regression test: loading a store file
// with null or missing maps used to leave the in-memory maps nil, making
// the next AddPeer or Ban panic.
func TestJSONPeerStoreLoadNilMaps(t *testing.T) {
	for name, contents := range map[string]string{
		"null values":  `{"peers": null, "bans": null}`,
		"missing keys": `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "peers.json")
			if err := os.WriteFile(path, []byte(contents), 0660); err != nil {
				t.Fatal(err)
			}
			jps, err := NewJSONPeerStore(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := jps.AddPeer("1.2.3.4:9981"); err != nil {
				t.Fatal(err)
			}
			if err := jps.Ban("5.6.7.8:9981", time.Hour, "test"); err != nil {
				t.Fatal(err)
			}
			peers, err := jps.Peers()
			if err != nil {
				t.Fatal(err)
			}
			if len(peers) != 1 || peers[0].Address != "1.2.3.4:9981" {
				t.Fatalf("unexpected peers: %v", peers)
			}
		})
	}
}

// TestJSONPeerStoreRoundTrip verifies that peers and bans survive a save
// and reload.
func TestJSONPeerStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	jps, err := NewJSONPeerStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jps.AddPeer("1.2.3.4:9981"); err != nil {
		t.Fatal(err)
	}
	if err := jps.Ban("5.6.7.8:9981", time.Hour, "test"); err != nil {
		t.Fatal(err)
	}

	jps2, err := NewJSONPeerStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jps2.PeerInfo("1.2.3.4:9981"); err != nil {
		t.Fatal(err)
	}
	banned, err := jps2.Banned("5.6.7.8:9981")
	if err != nil {
		t.Fatal(err)
	}
	if !banned {
		t.Fatal("expected peer to be banned after reload")
	}
}
