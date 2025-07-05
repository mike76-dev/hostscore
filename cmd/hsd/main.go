package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mike76-dev/hostscore/internal/build"
	"github.com/mike76-dev/hostscore/persist"
	"go.sia.tech/core/types"
	"go.sia.tech/coreutils/wallet"
	"golang.org/x/term"
	"lukechampine.com/flagg"
)

// Default config values.
var defaultConfig = persist.HSDConfig{
	GatewayMainnet: ":9981",
	GatewayZen:     ":9881",
	APIAddr:        ":9980",
	Dir:            ".",
	DBUser:         "",
	DBName:         "hostscore",
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

func getWalletSeed(network string) string {
	env := "HSD_WALLET_SEED"
	name := strings.ToUpper(network)
	var title string
	switch name {
	case "MAINNET":
		title = "Mainnet"
	case "ZEN":
		title = "Zen"
		env += "_ZEN"
	case "":
		name = "MAINNET"
		title = "Mainnet"
	default:
		log.Fatalf("Unsupported network: %s", network)
	}
	seed := os.Getenv(env)
	if seed != "" {
		log.Printf("Using %s environment variable.\n", env)
	} else {
		fmt.Printf("Enter %s wallet seed: ", title)
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

	var gatewayMainnet,
		gatewayZen,
		apiAddr,
		dir,
		dbUser,
		dbName string

	var disableMainnet,
		disableZen bool

	rootCmd := flagg.Root
	rootCmd.Usage = flagg.SimpleUsage(rootCmd, rootUsage)
	rootCmd.StringVar(&gatewayMainnet, "addr-mainnet", "", "Mainnet p2p address to listen on")
	rootCmd.StringVar(&gatewayZen, "addr-zen", "", "Zen p2p address to listen on")
	rootCmd.StringVar(&apiAddr, "api-addr", "", "address to serve API on")
	rootCmd.StringVar(&dir, "dir", "", "directory to store node state in")
	rootCmd.StringVar(&dbUser, "db-user", "", "username for accessing the database")
	rootCmd.StringVar(&dbName, "db-name", "", "name of MYSQL database")
	rootCmd.BoolVar(&disableMainnet, "no-mainnet", false, "disable Mainnet")
	rootCmd.BoolVar(&disableZen, "no-zen", false, "disable Zen")
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
		if gatewayMainnet != "" {
			config.GatewayMainnet = gatewayMainnet
		}
		if gatewayZen != "" {
			config.GatewayZen = gatewayZen
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
		if disableMainnet {
			config.GatewayMainnet = ""
		}
		if disableZen {
			config.GatewayZen = ""
		}

		if disableMainnet && disableZen {
			// All networks disabled, exiting.
			log.Fatalln("All networks disabled")
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

		// Fetch wallet seeds.
		seeds := make(map[string]string)
		if !disableMainnet {
			seeds["mainnet"] = getWalletSeed("mainnet")
		}
		if !disableZen {
			seeds["zen"] = getWalletSeed("zen")
		}

		// Create the directory if it does not yet exist.
		// This also checks if the provided directory parameter is valid.
		err = os.MkdirAll(config.Dir, 0700)
		if err != nil {
			log.Fatalf("Provided parameter is invalid: %v\n", config.Dir)
		}

		// Start hsd. startDaemon will only return when it is shutting down.
		err = startDaemon(&config, apiPassword, dbPassword, seeds)
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
		fmt.Printf("%s v%v\n", build.NodeBinaryName, build.NodeVersion)
		if build.GitRevision != "" {
			fmt.Println("Git Revision " + build.GitRevision)
		}

	case seedCmd:
		if len(cmd.Args()) != 0 {
			cmd.Usage()
			return
		}
		seed := wallet.NewSeedPhrase()
		var s [32]byte
		if err := wallet.SeedFromPhrase(&s, seed); err != nil {
			log.Fatalln(err)
		}
		addr := types.StandardUnlockHash(wallet.KeyFromSeed(&s, 0).PublicKey())
		fmt.Printf("Seed:    %s\n", seed)
		fmt.Printf("Address: %v\n", strings.TrimPrefix(addr.String(), "addr:"))
	}
}
