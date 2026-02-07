package pokerserver

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	// Blank import to embed config.json
	_ "embed"

	tableserver "github.com/ekotlikoff/gopoker/internal/server/backend"
	gateway "github.com/ekotlikoff/gopoker/internal/server/frontend"
)

//go:embed config.json
var config []byte

type (
	// Configuration is a struct that configures the chess server
	Configuration struct {
		ServiceName             string
		Environment             string
		BackendType             BackendType
		BasePath                string
		EnableBotMatching       bool
		EngineConnectionTimeout string
		EngineAddr              string
		GatewayPort             int
		HTTPPort                int
		WSPort                  int
		MaxMatchingDuration     string
		MatchPlayerTimeSeconds  int
		LogFile                 string
		EnableTracing           bool
		Quiet                   bool
	}
	// BackendType represents different types of backends
	BackendType string
)

const (
	// HTTPBackend type
	HTTPBackend = BackendType("http")
	// WebsocketBackend type
	WebsocketBackend = BackendType("websocket")
)

// RunServer runs the gochess server
func RunServer() {
	c := loadConfig()
	RunServerWithConfig(c)
}

// RunServerWithConfig runs the gochess server with a custom config
func RunServerWithConfig(config Configuration) {
	configureLogging(config)
	ts := tableserver.NewTableServer()
	go ts.Serve()
	gw := gateway.Gateway{
		TableServer: ts,
		BasePath:    config.BasePath,
		Port:        config.GatewayPort,
	}
	gw.Serve()
}

func configureLogging(config Configuration) {
	if config.LogFile != "" {
		file, err := os.OpenFile(config.LogFile, os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal(err)
		}
		log.SetOutput(file)
	}
	if config.Quiet {
		log.SetOutput(io.Discard)
	}
}

func loadConfig() Configuration {
	configuration := Configuration{}
	err := json.Unmarshal(config, &configuration)
	if err != nil {
		fmt.Println("ERROR:", err)
	}
	return configuration
}
