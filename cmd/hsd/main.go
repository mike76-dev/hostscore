package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/mike76-dev/hostscore/persist"

	"golang.org/x/term"
)

// Default config values.
var defaultConfig = persist.HSDConfig{
	Name:        "",
	UserAgent:   "HostScore",
	GatewayAddr: ":9981",
	APIAddr:     "localhost:9980",
	Dir:         ".",
	Bootstrap:   true,
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

	// Parse command line flags. If set, they override the loaded config.
	name := flag.String("name", "", "name of the benchmarking node")
	userAgent := flag.String("agent", "", "custom agent used for API calls")
	gatewayAddr := flag.String("addr", "", "address to listen on for peer connections")
	apiAddr := flag.String("api-addr", "", "address to serve API on")
	dir := flag.String("dir", "", "directory to store logs in")
	bootstrap := flag.Bool("bootstrap", true, "bootstrap the gateway and consensus modules")
	dbUser := flag.String("db-user", "", "username for accessing the database")
	dbName := flag.String("db-name", "", "name of MYSQL database")
	network := flag.String("network", "mainnet", "name of the network")
	flag.Parse()
	if *name != "" {
		config.Name = *name
	}
	if *userAgent != "" {
		config.UserAgent = *userAgent
	}
	if *gatewayAddr != "" {
		config.GatewayAddr = *gatewayAddr
	}
	if *apiAddr != "" {
		config.APIAddr = *apiAddr
	}
	if *dir != "" {
		config.Dir = *dir
	}
	config.Bootstrap = *bootstrap
	if *dbUser != "" {
		config.DBUser = *dbUser
	}
	if *dbName != "" {
		config.DBName = *dbName
	}
	if *network != "" {
		config.Network = *network
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

	// Create the logs directory if it does not yet exist.
	// This also checks if the provided directory parameter is valid.
	err = os.MkdirAll(config.Dir, 0700)
	if err != nil {
		log.Fatalf("Provided parameter is invalid: %v\n", config.Dir)
	}

	// Start satd. startDaemon will only return when it is shutting down.
	err = startDaemon(&config, apiPassword, dbPassword)
	if err != nil {
		log.Fatalln(err)
	}

	// Daemon seems to have closed cleanly. Print a 'closed' message.
	log.Println("Shutdown complete.")
}
