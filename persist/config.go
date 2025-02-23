package persist

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mike76-dev/hostscore/internal/utils"
)

// configFilename is the name of the configuration file.
const configFilename = "hsdconfig.json"

// HSDConfig contains the fields that are passed on to the new node.
type HSDConfig struct {
	GatewayMainnet string `json:"mainnet,omitempty"`
	GatewayZen     string `json:"zen,omitempty"`
	GatewayAnagami string `json:"anagami,omitempty"`
	APIAddr        string `json:"api"`
	Dir            string `json:"dir"`
	DBUser         string `json:"dbUser"`
	DBName         string `json:"dbName"`
}

// hsdMetadata contains the header and version strings that identify the
// config file.
type hsdMetadata = struct {
	Header  string
	Version string
}

// metadata contains the actual values.
var metadata = hsdMetadata{
	Header:  "HSD Configuration",
	Version: "0.3.0",
}

// Load loads the configuration from disk.
func (hsdc *HSDConfig) Load(dir string) (ok bool, err error) {
	ok = false
	err = loadJSON(metadata, &hsdc, filepath.Join(dir, configFilename))
	if os.IsNotExist(err) {
		// There is no config.json, nothing to load.
		err = nil
		return
	}
	if err != nil {
		err = fmt.Errorf("failed to load the configuration: %s", err)
		return
	}
	ok = true
	return
}

// Save stores the configuration on disk.
func (hsdc *HSDConfig) Save(dir string) error {
	return saveJSON(metadata, hsdc, filepath.Join(dir, configFilename))
}

// loadJSON will try to read a persisted json object from a file.
func loadJSON(meta hsdMetadata, object interface{}, filename string) error {
	// Open the file.
	file, err := os.Open(filename)
	if os.IsNotExist(err) {
		return err
	}
	if err != nil {
		return fmt.Errorf("unable to open persisted json object file: %s", err)
	}
	defer func() {
		err = utils.ComposeErrors(err, file.Close())
	}()

	// Read the metadata from the file.
	var header, version string
	dec := json.NewDecoder(file)
	if err := dec.Decode(&header); err != nil {
		return fmt.Errorf("unable to read header from persisted json object file: %s", err)
	}
	if header != meta.Header {
		return errors.New("wrong config file header")
	}
	if err := dec.Decode(&version); err != nil {
		return fmt.Errorf("unable to read version from persisted json object file: %s", err)
	}
	if version != meta.Version {
		return errors.New("wrong config file version")
	}

	// Read everything else.
	remainingBytes, err := io.ReadAll(dec.Buffered())
	if err != nil {
		return fmt.Errorf("unable to read persisted json object data: %s", err)
	}
	// The buffer may or may not have read the rest of the file, read the rest
	// of the file to be certain.
	remainingBytesExtra, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("unable to read persisted json object data: %s", err)
	}
	remainingBytes = append(remainingBytes, remainingBytesExtra...)

	// Parse the json object.
	return json.Unmarshal(remainingBytes, &object)
}

// saveJSON will save a json object to disk.
func saveJSON(meta hsdMetadata, object interface{}, filename string) error {
	// Write the metadata to the buffer.
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	if err := enc.Encode(meta.Header); err != nil {
		return fmt.Errorf("unable to encode metadata header: %s", err)
	}
	if err := enc.Encode(meta.Version); err != nil {
		return fmt.Errorf("unable to encode metadata version: %s", err)
	}

	// Marshal the object into json and write the result to the buffer.
	objBytes, err := json.MarshalIndent(object, "", "\t")
	if err != nil {
		return fmt.Errorf("unable to marshal the provided object: %s", err)
	}
	buf.Write(objBytes)
	data := buf.Bytes()

	// Write out the data to the file, with a sync.
	err = func() (err error) {
		file, err := os.OpenFile(filename, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0600)
		if err != nil {
			return fmt.Errorf("unable to open file: %s", err)
		}
		defer func() {
			err = utils.ComposeErrors(err, file.Close())
		}()
		_, err = file.Write(data)
		if err != nil {
			return fmt.Errorf("unable to write file: %s", err)
		}
		err = file.Sync()
		if err != nil {
			return fmt.Errorf("unable to sync file: %s", err)
		}
		return nil
	}()

	return err
}
