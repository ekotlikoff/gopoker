//go:build wasm && js && webclient

package main

import (
	_ "embed"
	"encoding/json"
	"io/ioutil"
	"log"
)

var (
	//go:embed config.json
	config []byte
	quiet  bool = false
)

type Configuration struct {
	TLS           bool
	ClientTimeout string
}

func main() {
	if quiet {
		log.SetOutput(ioutil.Discard)
	}
	done := make(chan struct{})
	/*
		config := loadConfig()
		table := model.NewTable()
		jar, _ := cookiejar.New(&cookiejar.Options{})
		clientTimeout, _ := time.ParseDuration(config.ClientTimeout)
		client := &http.Client{Jar: jar, Timeout: clientTimeout}
		clientModel := ClientModel{
			game: game, playerColor: model.White,
			document: js.Global().Get("document"),
			board: js.Global().Get("document").Call(
				"getElementById", "board-layout-chessboard"), tls: config.TLS,
			backendType: config.BackendType, client: client, gameType: Local,
			origin: js.Global().Get("window").Get("location").Get("host").String(),
		}
		clientModel.initController()
		clientModel.initStyle()
		clientModel.viewInitBoard(clientModel.playerColor)
		clientModel.checkForSession()
	*/
	<-done
}

func loadConfig() Configuration {
	configuration := Configuration{}
	err := json.Unmarshal(config, &configuration)
	if err != nil {
		log.Println("ERROR:", err)
	}
	return configuration
}
