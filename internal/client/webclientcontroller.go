//go:build wasm && js && webclient

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"syscall/js"
	"time"

	model "github.com/Ekotlikoff/gopoker/internal/model/table"
	gateway "github.com/Ekotlikoff/gopoker/internal/server"
)

var (
	debug bool   = true
	ctp   string = "application/json"
)

func (cm *ClientModel) initController() {
	cm.document.Call("addEventListener", "mousemove", cm.genMouseMove(), false)
	cm.document.Call("addEventListener", "touchmove", cm.genTouchMove(), false)
	cm.document.Call("addEventListener", "mouseup", cm.genMouseUp(), false)
	cm.document.Call("addEventListener", "touchend", cm.genTouchEnd(), false)
	cm.document.Call("addEventListener", "mousedown",
		cm.genGlobalOnTouchStart(), false)
	cm.document.Call("addEventListener", "touchstart",
		cm.genGlobalOnTouchStart(), false)
	cm.board.Call("addEventListener", "contextmenu",
		js.FuncOf(preventDefault), false)
	js.Global().Set("beginMatchmaking", cm.genBeginMatchmaking())
	js.Global().Set("onclick", cm.genGlobalOnclick())
}

func (cm *ClientModel) checkForSession() {
	resp, err := cm.client.Get("session")
	if err == nil {
		defer resp.Body.Close()
	}
	if err != nil || resp.StatusCode != 200 {
		log.Println("No session found")
		return
	}
	sessionResponse := gateway.SessionResponse{}
	err = json.NewDecoder(resp.Body).Decode(&sessionResponse)
	if err != nil {
		log.Println(err)
	}
	cm.document.Call("getElementById", "username").Set("value",
		sessionResponse.Credentials.Username)
	cm.SetPlayerName(sessionResponse.Credentials.Username)
	cm.SetHasSession(true)
	if sessionResponse.InMatch {
		log.Println("Rejoining match")
		cm.handleRejoinMatch(sessionResponse.Match)
	}
}

func (cm *ClientModel) genMouseDown() js.Func {
	return js.FuncOf(func(this js.Value, i []js.Value) interface{} {
		if len(i) > 0 && !cm.GetIsMouseDown() {
			i[0].Call("preventDefault")
			cm.handleClickStart(this, i[0])
		}
		return 0
	})
}

func (cm *ClientModel) genTouchStart() js.Func {
	return js.FuncOf(func(this js.Value, i []js.Value) interface{} {
		if len(i) > 0 && !cm.GetIsMouseDown() {
			i[0].Call("preventDefault")
			touch := i[0].Get("touches").Index(0)
			cm.handleClickStart(this, touch)
		}
		return 0
	})
}

func (cm *ClientModel) handleClickStart(
	this js.Value, event js.Value) {
	cm.LockMouseDown()
	cm.SetDraggingElement(this)
	positionOriginal, err := cm.getGamePositionFromPieceElement(this)
	if err != nil {
		log.Println("ERROR: Issue getting position from element,", err)
		return
	}
	cm.positionOriginal = positionOriginal
	cm.SetDraggingPiece(cm.positionOriginal)
	if cm.GetDraggingPiece() == nil {
		if debug {
			log.Println("ERROR: Clicked a piece that is not on the board")
			log.Println(cm.positionOriginal)
			log.Println(cm.GetBoardString())
		}
		cm.UnlockMouseDown()
		return
	}
	addClass(cm.GetDraggingElement(), "dragging")
	cm.SetDraggingOriginalTransform(
		cm.GetDraggingElement().Get("style").Get("transform"))
}

func (cm *ClientModel) genMouseMove() js.Func {
	return js.FuncOf(func(this js.Value, i []js.Value) interface{} {
		i[0].Call("preventDefault")
		cm.handleMoveEvent(i[0])
		return 0
	})
}

func (cm *ClientModel) genTouchMove() js.Func {
	return js.FuncOf(func(this js.Value, i []js.Value) interface{} {
		i[0].Call("preventDefault")
		touch := i[0].Get("touches").Index(0)
		cm.handleMoveEvent(touch)
		return 0
	})
}

func (cm *ClientModel) handleMoveEvent(moveEvent js.Value) {
	if cm.GetIsMouseDown() {
		cm.viewDragPiece(cm.GetDraggingElement(), moveEvent)
	}
}

func (cm *ClientModel) genMouseUp() js.Func {
	return js.FuncOf(func(this js.Value, i []js.Value) interface{} {
		if cm.GetIsMouseDown() && len(i) > 0 {
			i[0].Call("preventDefault")
			cm.handleClickEnd(i[0])
		}
		return 0
	})
}

func (cm *ClientModel) genTouchEnd() js.Func {
	return js.FuncOf(func(this js.Value, i []js.Value) interface{} {
		if cm.GetIsMouseDown() && len(i) > 0 {
			i[0].Call("preventDefault")
			touch := i[0].Get("changedTouches").Index(0)
			cm.handleClickEnd(touch)
		}
		return 0
	})
}

func (cm *ClientModel) handleClickEnd(event js.Value) {
	cm.UnlockMouseDown()
	elDragging := cm.GetDraggingElement()
	_, _, _, _, gridX, gridY := cm.getEventMousePosition(event)
	newPosition := cm.getPositionFromGrid(uint8(gridX), uint8(gridY))
	pieceDragging := cm.GetDraggingPiece()
	var promoteTo *model.PieceType
	moveRequest := model.MoveRequest{cm.positionOriginal, model.Move{
		int8(newPosition.File) - int8(cm.positionOriginal.File),
		int8(newPosition.Rank) - int8(cm.positionOriginal.Rank)},
		promoteTo,
	}
	if pieceDragging.PieceType == model.Pawn &&
		((cm.positionOriginal.Rank == 1 && cm.game.Turn() == model.Black) ||
			(cm.positionOriginal.Rank == 6 && cm.game.Turn() == model.White)) &&
		(newPosition.Rank == 0 || newPosition.Rank == 7) {
		cm.viewCreatePromotionWindow(
			int(newPosition.File), int(newPosition.Rank))
		elDragging.Get("style").Set("transform",
			cm.GetDraggingOriginalTransform())
		cm.isMouseDown = false
		cm.SetPromotionMoveRequest(moveRequest)
		return
	}
	cm.handleMove(moveRequest)
}

func (cm *ClientModel) handleSyncUpdate(opponentMove model.MoveRequest) {
	err := cm.MakeMove(opponentMove)
	if err != nil {
		log.Println("FATAL: We do not expect an invalid move from the opponent.")
		return
	}
	cm.ClearRequestedDraw()
	newPos := model.Position{
		opponentMove.Position.File + uint8(opponentMove.Move.X),
		opponentMove.Position.Rank + uint8(opponentMove.Move.Y),
	}
	originalPosClass :=
		getPositionClass(opponentMove.Position, cm.GetPlayerColor())
	elements :=
		cm.document.Call("getElementsByClassName", originalPosClass)
	elMoving := elements.Index(0)
	cm.viewHandleMove(opponentMove, newPos, elMoving)
}

func (cm *ClientModel) handleResponseAsync(
	responseAsync matchserver.ResponseAsync) {
	if responseAsync.GameOver {
		cm.remoteGameEnd()
		winType := ""
		if responseAsync.Resignation {
			winType = "resignation"
		} else if responseAsync.Draw {
			winType = "draw"
			cm.ClearRequestedDraw()
		} else if responseAsync.Timeout {
			winType = "timeout"
		} else {
			winType = "mate"
		}
		log.Println("Winner:", responseAsync.Winner, "by", winType)
		cm.viewSetGameOver(responseAsync.Winner, winType)
		return
	} else if responseAsync.RequestToDraw {
		log.Println("Requested draw")
		cm.SetRequestedDraw(cm.GetOpponentColor(),
			!cm.GetRequestedDraw(cm.GetOpponentColor()))
	} else if responseAsync.Matched {
		cm.SetPlayerColor(responseAsync.MatchDetails.Color)
		cm.SetOpponentName(responseAsync.MatchDetails.OpponentName)
		cm.SetMaxTimeMs(responseAsync.MatchDetails.MaxTimeMs)
		cm.handleStartMatch()
	}
}

func (cm *ClientModel) handleResponseSync(responseSync matchserver.ResponseSync) {
	if responseSync.MoveSuccess {
		cm.SetPlayerElapsedMs(cm.playerColor, int64(responseSync.ElapsedMs))
		cm.SetPlayerElapsedMs(cm.GetOpponentColor(),
			int64(responseSync.ElapsedMsOpponent))
	}
}

func (cm *ClientModel) genBeginMatchmaking() js.Func {
	return js.FuncOf(func(this js.Value, i []js.Value) interface{} {
		if !cm.GetIsMatchmaking() && !cm.GetIsMatched() {
			go cm.lookForMatch()
		}
		return 0
	})
}

func (cm *ClientModel) lookForMatch() {
	cm.SetIsMatchmaking(true)
	cm.buttonBeginLoading(
		cm.document.Call("getElementById", "beginMatchmakingButton"))
	if !cm.GetHasSession() {
		username := cm.document.Call(
			"getElementById", "username").Get("value").String()
		credentialsBuf := new(bytes.Buffer)
		credentials := gateway.Credentials{username}
		json.NewEncoder(credentialsBuf).Encode(credentials)
		resp, err := cm.client.Post("session", ctp, credentialsBuf)
		if err == nil {
			resp.Body.Close()
		}
		if err != nil || resp.StatusCode != 200 {
			log.Println("Error starting session")
			cm.GetButtonLoader().Call("remove")
			cm.SetIsMatchmaking(false)
			return
		}
		cm.SetPlayerName(username)
		cm.SetHasSession(true)
	}
	err := cm.wsMatch()
	if err != nil {
		cm.GetButtonLoader().Call("remove")
	}
}

func (cm *ClientModel) handleStartMatch() {
	cm.resetGame()
	// - TODO once matched briefly display matched icon?
	cm.SetGameType(Remote)
	cm.SetIsMatched(true)
	cm.SetIsMatchmaking(false)
	cm.GetButtonLoader().Call("remove")
	cm.remoteMatchModel.endRemoteGameChan = make(chan bool, 0)
	cm.viewSetMatchControls()
	go cm.matchDetailsUpdateLoop()
}

func (cm *ClientModel) handleRejoinMatch(match gateway.CurrentMatch) {
	myColor := model.Black
	opponentName := match.WhiteName
	if opponentName == cm.GetPlayerName() {
		myColor = model.White
		opponentName = match.BlackName
	}
	cm.SetPlayerColor(myColor)
	cm.SetOpponentName(opponentName)
	cm.SetMaxTimeMs(match.MaxTimeMs)
	cm.SetPlayerElapsedMs(model.Black, match.MaxTimeMs-match.BlackRemainingTimeMs)
	cm.SetPlayerElapsedMs(model.White, match.MaxTimeMs-match.WhiteRemainingTimeMs)
	cm.resetGameWithInProgressGame(match)
	cm.SetGameType(Remote)
	cm.SetIsMatched(true)
	cm.SetIsMatchmaking(false)
	cm.remoteMatchModel.endRemoteGameChan = make(chan bool, 0)
	cm.viewSetMatchControls()
	if cm.backendType == WebsocketBackend {
		err := cm.wsConnect()
		if err != nil {
			cm.SetIsMatched(false)
			cm.SetIsMatchmaking(false)
			cm.resetGame()
		} else {
			go cm.matchDetailsUpdateLoop()
		}
	} else if cm.backendType == HttpBackend {
		go cm.matchDetailsUpdateLoop()
		go cm.listenForSyncUpdateHttp()
		go cm.listenForAsyncUpdateHttp()
	}
}

func (cm *ClientModel) wsMatch() error {
	var err error
	if cm.GetWSConn().Equal(js.Undefined()) {
		err = cm.wsConnect()
	}
	if err == nil {
		message := matchserver.WebsocketRequest{
			WebsocketRequestType: matchserver.RequestAsyncT,
			RequestAsync:         matchserver.RequestAsync{Match: true},
		}
		jsonMsg, _ := json.Marshal(message)
		cm.GetWSConn().Call("send", string(jsonMsg))
	} else {
		cm.SetIsMatchmaking(false)
		cm.GetButtonLoader().Call("remove")
	}
	return err
}

func (cm *ClientModel) wsConnect() error {
	pathname := js.Global().Get("location").Get("pathname").String()
	scheme := "ws"
	if cm.tls {
		scheme = "wss"
	}
	u := scheme + "://" + cm.origin + pathname + "ws"
	ws := js.Global().Get("WebSocket").New(u)
	retries := 0
	maxRetries := 100
	for true {
		if ws.Get("readyState").Equal(js.Global().Get("WebSocket").Get("OPEN")) {
			cm.SetWSConn(ws)
			if debug {
				log.Println("Websocket connection successfully initiated")
				go cm.wsListener()
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
		retries++
		if retries > maxRetries {
			log.Println("ERROR: Error opening websocket connection")
			return errors.New("Error opening websocket connection")
		}
	}
	return nil
}

func (cm *ClientModel) wsListener() {
	ws := cm.GetWSConn()
	ws.Set("onmessage",
		js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			jsonString := args[0].Get("data").String()
			if debug {
				log.Println(jsonString)
			}
			message := matchserver.WebsocketResponse{}
			json.Unmarshal([]byte(jsonString), &message)
			switch message.WebsocketResponseType {
			case matchserver.OpponentPlayedMoveT:
				cm.handleSyncUpdate(message.OpponentPlayedMove)
			case matchserver.ResponseSyncT:
				cm.handleResponseSync(message.ResponseSync)
			case matchserver.ResponseAsyncT:
				cm.handleResponseAsync(message.ResponseAsync)
			}
			return nil
		}))
}

func (cm *ClientModel) matchDetailsUpdateLoop() {
	for true {
		cm.viewSetMatchDetails()
		time.Sleep(100 * time.Millisecond)
		select {
		case <-cm.remoteMatchModel.endRemoteGameChan:
			return
		default:
		}
		turn := cm.game.Turn()
		cm.AddPlayerElapsedMs(turn, 100)
	}
}

func preventDefault(this js.Value, i []js.Value) interface{} {
	if len(i) > 0 {
		i[0].Call("preventDefault")
	}
	return 0
}
