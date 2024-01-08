package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
	"go.sia.tech/core/chain"
	"go.sia.tech/core/consensus"
	"go.sia.tech/core/types"
)

// TestnetAnagami returns the chain parameters and genesis block for the "Anagami"
// testnet chain.
func TestnetAnagami() (*consensus.Network, types.Block) {
	n := &consensus.Network{
		Name: "anagami",

		InitialCoinbase: types.Siacoins(300000),
		MinimumCoinbase: types.Siacoins(300000),
		InitialTarget:   types.BlockID{3: 1},
	}

	n.HardforkDevAddr.Height = 1
	n.HardforkDevAddr.OldAddress = types.Address{}
	n.HardforkDevAddr.NewAddress = types.Address{}

	n.HardforkTax.Height = 2

	n.HardforkStorageProof.Height = 3

	n.HardforkOak.Height = 5
	n.HardforkOak.FixHeight = 8
	n.HardforkOak.GenesisTimestamp = time.Unix(1702300000, 0) // Dec 11, 2023 @ 13:06 GMT

	n.HardforkASIC.Height = 13
	n.HardforkASIC.OakTime = 10 * time.Minute
	n.HardforkASIC.OakTarget = n.InitialTarget

	n.HardforkFoundation.Height = 21
	n.HardforkFoundation.PrimaryAddress, _ = types.ParseAddress("addr:5949fdf56a7c18ba27f6526f22fd560526ce02a1bd4fa3104938ab744b69cf63b6b734b8341f")
	n.HardforkFoundation.FailsafeAddress = n.HardforkFoundation.PrimaryAddress

	n.HardforkV2.AllowHeight = 2016         // ~2 weeks in
	n.HardforkV2.RequireHeight = 2016 + 288 // ~2 days later

	b := types.Block{
		Timestamp: n.HardforkOak.GenesisTimestamp,
		Transactions: []types.Transaction{{
			SiacoinOutputs: []types.SiacoinOutput{{
				Address: n.HardforkFoundation.PrimaryAddress,
				Value:   types.Siacoins(1).Mul64(1e12),
			}},
			SiafundOutputs: []types.SiafundOutput{{
				Address: n.HardforkFoundation.PrimaryAddress,
				Value:   10000,
			}},
		}},
	}

	return n, b
}

func testnetFixDBTree(dir string) {
	bdb, err := bolt.Open(filepath.Join(dir, "consensus.db"), 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	db := &boltDB{db: bdb}
	defer db.Close()
	if db.Bucket([]byte("tree-fix-2")) != nil {
		return
	}

	fmt.Print("Fixing consensus.db Merkle tree...")

	network, genesisBlock := TestnetAnagami()
	dbstore, tipState, err := chain.NewDBStore(db, network, genesisBlock)
	if err != nil {
		log.Fatal(err)
	}
	cm := chain.NewManager(dbstore, tipState)

	bdb2, err := bolt.Open(filepath.Join(dir, "consensus.db-fixed"), 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	db2 := &boltDB{db: bdb2}
	defer db2.Close()
	dbstore2, tipState2, err := chain.NewDBStore(db2, network, genesisBlock)
	if err != nil {
		log.Fatal(err)
	}
	cm2 := chain.NewManager(dbstore2, tipState2)

	for cm2.Tip() != cm.Tip() {
		fmt.Printf("\rFixing consensus.db Merkle tree...%v/%v", cm2.Tip().Height, cm.Tip().Height)
		index, _ := cm.BestIndex(cm2.Tip().Height + 1)
		b, _ := cm.Block(index.ID)
		if err := cm2.AddBlocks([]types.Block{b}); err != nil {
			break
		}
	}
	fmt.Println()

	if _, err := db2.CreateBucket([]byte("tree-fix-2")); err != nil {
		log.Fatal(err)
	} else if err := db.Close(); err != nil {
		log.Fatal(err)
	} else if err := db2.Close(); err != nil {
		log.Fatal(err)
	} else if err := os.Rename(filepath.Join(dir, "consensus.db-fixed"), filepath.Join(dir, "consensus.db")); err != nil {
		log.Fatal(err)
	}

	fmt.Print("Backing up old wallet state...")
	os.RemoveAll(filepath.Join(dir, "wallets.json-bck"))
	os.Rename(filepath.Join(dir, "wallets.json"), filepath.Join(dir, "wallets.json-bck"))
	os.RemoveAll(filepath.Join(dir, "wallets-bck"))
	os.Rename(filepath.Join(dir, "wallets"), filepath.Join(dir, "wallets-bck"))
	fmt.Println("done.")
	fmt.Println("NOTE: Your wallet will resync automatically on first use; this may take a few seconds.")
}

func testnetDeleteV1DBState(dir string) {
	bdb, err := bolt.Open(filepath.Join(dir, "consensus.db"), 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	var needUpdate bool
	bdb.View(func(tx *bolt.Tx) error {
		needUpdate = tx.Bucket([]byte("Tree")) != nil
		return nil
	})
	if !needUpdate {
		return
	}

	fmt.Println("Deleting unneeded v1 state...")

	var blockIDs []types.BlockID
	bdb.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("Blocks")).ForEach(func(k, v []byte) error {
			blockIDs = append(blockIDs, *(*types.BlockID)(k))
			return nil
		})
	})
	var total int
	err = bdb.Update(func(tx *bolt.Tx) error {
		for _, bucket := range []struct {
			name     string
			elemName string
		}{
			{"SiacoinElements", "siacoin elements"},
			{"SiafundElements", "siafund elements"},
			{"FileContracts", "file contract elements"},
			{"AncestorTimestamps", "ancestor timestamps"},
			{"Tree", "Merkle tree hashes"},
		} {
			b := tx.Bucket([]byte(bucket.name))
			if b == nil {
				continue
			}
			b.ForEach(func(k, v []byte) error {
				fmt.Printf("\rDeleting %v...%x", bucket.elemName, k)
				total += len(k) + len(v)
				return b.Delete(k)
			})
			tx.DeleteBucket([]byte(bucket.name))
			fmt.Println("done.")
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	db := &boltDB{db: bdb}
	defer db.Close()
	network, genesisBlock := TestnetAnagami()
	dbstore, _, err := chain.NewDBStore(db, network, genesisBlock)
	if err != nil {
		log.Fatal(err)
	}

	var buf bytes.Buffer
	e := types.NewEncoder(&buf)
	for _, id := range blockIDs {
		fmt.Printf("\rDeleting v1 block supplements...%v", id)
		if b, bs, _ := dbstore.Block(id); bs != nil {
			buf.Reset()
			for _, txn := range bs.Transactions {
				txn.EncodeTo(e)
			}
			for _, fc := range bs.ExpiringFileContracts {
				fc.EncodeTo(e)
			}
			e.Flush()
			total += buf.Len()
			bs.Transactions = nil
			bs.ExpiringFileContracts = nil
			dbstore.AddBlock(b, bs)
		}
	}
	fmt.Println("done.")
	fmt.Printf("All v1 state deleted. Your consensus.db is now %v MB lighter!\n", total/1e6)
}
