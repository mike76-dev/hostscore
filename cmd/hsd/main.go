package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mike76-dev/hostscore/internal/build"
	"github.com/mike76-dev/hostscore/persist"
	"github.com/mike76-dev/hostscore/wallet"
	"go.sia.tech/core/types"
	"golang.org/x/term"
	"lukechampine.com/flagg"
)

// Default config values.
var defaultConfig = persist.HSDConfig{
	Name:        "",
	GatewayAddr: ":9981",
	APIAddr:     "localhost:9980",
	Dir:         ".",
	DBUser:      "",
	DBName:      "hostscore",
	Network:     "mainnet",
}

var config persist.HSDConfig
var configDir string

func getAPIPassword() string {
	apiPassword := os.Getenv("HSD_API_PASSWORD")
	if apiPassword != "" {
		log.Println("Using HSD_API_PASSWORD environment variable.")
	} else {
		fmt.Print("Enter API password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			log.Fatalf("Could not read API password: %v\n", err)
		}
		apiPassword = string(pw)
	}
	return apiPassword
}

func getDBPassword() string {
	dbPassword := os.Getenv("HSD_DB_PASSWORD")
	if dbPassword != "" {
		log.Println("Using HSD_DB_PASSWORD environment variable.")
	} else {
		fmt.Print("Enter database password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			log.Fatalf("Could not read database password: %v\n", err)
		}
		dbPassword = string(pw)
	}
	return dbPassword
}

func getWalletSeed() string {
	seed := os.Getenv("HSD_WALLET_SEED")
	if seed != "" {
		log.Println("Using HSD_WALLET_SEED environment variable.")
	} else {
		fmt.Print("Enter wallet seed: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			log.Fatalf("Could not read wallet seed: %v\n", err)
		}
		seed = string(pw)
	}
	return seed
}

var (
	rootUsage = `Usage:
    hsd [flags] [action]

Run 'hsd' with no arguments to start the blockchain node and API server.

Actions:
    version     print hsd version
    seed        generate a seed
`
	versionUsage = `Usage:
    hsd version

Prints the version of the hsd binary.
`
	seedUsage = `Usage:
    hsd seed

Generates a secure seed.
`
)

func main() {
	log.SetFlags(0)

	// Load config file if it exists. Otherwise load the defaults.
	configDir = os.Getenv("HSD_CONFIG_DIR")
	if configDir != "" {
		log.Println("Using HSD_CONFIG_DIR environment variable to load config.")
	}
	ok, err := config.Load(configDir)
	if err != nil {
		log.Fatalln("Could not load config file")
	}
	if !ok {
		config = defaultConfig
	}

	var name, gatewayAddr, apiAddr, dir, dbUser, dbName, network string

	rootCmd := flagg.Root
	rootCmd.Usage = flagg.SimpleUsage(rootCmd, rootUsage)
	rootCmd.StringVar(&name, "name", "", "name of the benchmarking node")
	rootCmd.StringVar(&gatewayAddr, "addr", ":9981", "p2p address to listen on")
	rootCmd.StringVar(&apiAddr, "api-addr", "localhost:9980", "address to serve API on")
	rootCmd.StringVar(&dir, "dir", ".", "directory to store node state in")
	rootCmd.StringVar(&dbUser, "db-user", "", "username for accessing the database")
	rootCmd.StringVar(&dbName, "db-name", "", "name of MYSQL database")
	rootCmd.StringVar(&network, "network", "mainnet", "network to connect to")
	versionCmd := flagg.New("version", versionUsage)
	seedCmd := flagg.New("seed", seedUsage)

	cmd := flagg.Parse(flagg.Tree{
		Cmd: rootCmd,
		Sub: []flagg.Tree{
			{Cmd: versionCmd},
			{Cmd: seedCmd},
		},
	})

	switch cmd {
	case rootCmd:
		if len(cmd.Args()) != 0 {
			cmd.Usage()
			return
		}

		// Parse command line flags. If set, they override the loaded config.
		if name != "" {
			config.Name = name
		}
		if gatewayAddr != "" {
			config.GatewayAddr = gatewayAddr
		}
		if apiAddr != "" {
			config.APIAddr = apiAddr
		}
		if dir != "" {
			config.Dir = dir
		}
		if dbUser != "" {
			config.DBUser = dbUser
		}
		if dbName != "" {
			config.DBName = dbName
		}
		if network != "" {
			config.Network = network
		}

		// Save the configuration.
		err = config.Save(configDir)
		if err != nil {
			log.Fatalln("Unable to save config file")
		}

		// Fetch API password.
		apiPassword := getAPIPassword()

		// Fetch DB password.
		dbPassword := getDBPassword()

		// Fetch wallet seed.
		seed := getWalletSeed()

		// Create the directory if it does not yet exist.
		// This also checks if the provided directory parameter is valid.
		err = os.MkdirAll(config.Dir, 0700)
		if err != nil {
			log.Fatalf("Provided parameter is invalid: %v\n", config.Dir)
		}

		// Start hsd. startDaemon will only return when it is shutting down.
		err = startDaemon(&config, apiPassword, dbPassword, seed)
		if err != nil {
			log.Fatalln(err)
		}

		// Daemon seems to have closed cleanly. Print a 'closed' message.
		log.Println("Shutdown complete.")

	case versionCmd:
		if len(cmd.Args()) != 0 {
			cmd.Usage()
			return
		}
		fmt.Printf("hsd v%v\n", build.NodeVersion)
		if build.GitRevision != "" {
			fmt.Println("Git Revision " + build.GitRevision)
		}

	case seedCmd:
		if len(cmd.Args()) != 0 {
			cmd.Usage()
			return
		}
		seed := wallet.NewSeedPhrase()
		sk, err := wallet.KeyFromPhrase(seed)
		if err != nil {
			log.Fatalln(err)
		}
		addr := types.StandardUnlockHash(sk.PublicKey())
		fmt.Printf("Seed:    %s\n", seed)
		fmt.Printf("Address: %v\n", strings.TrimPrefix(addr.String(), "addr:"))
	}
}
