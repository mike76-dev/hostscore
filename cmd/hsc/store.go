package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type node struct {
	Location string `json:"location"`
	Address  string `json:"address"`
	Password string `json:"password"`
}

type persistData struct {
	Nodes []node `json:"nodes"`
}

type jsonStore struct {
	nodes map[string]node
}

func newJSONStore(dir string) (*jsonStore, error) {
	s := &jsonStore{
		nodes: make(map[string]node),
	}
	err := s.load(dir)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *jsonStore) load(dir string) error {
	var p persistData
	if js, err := os.ReadFile(filepath.Join(dir, "nodes.json")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	} else if err := json.Unmarshal(js, &p); err != nil {
		return err
	}
	for _, n := range p.Nodes {
		s.nodes[n.Location] = n
	}
	return nil
}
