//go:build wasm && js && webclient

package main

import (
	"net/http"
	"sync"
	"syscall/js"

	model "github.com/Ekotlikoff/gopoker/internal/model/table"
)

const (
	Local  = GameType(iota)
	Remote = GameType(iota)
)

type (
	GameType    uint8
	ClientModel struct {
		cmMutex                  sync.RWMutex
		gameType                 GameType
		playerIndex              int
		isMatchmaking, isMatched bool
		playerName               string
		hasSession               bool
		wsConn                   js.Value
		tls                      bool
		tableMutex               sync.RWMutex
		table                    *model.Table
		remoteMatchModel         RemoteMatchModel
		buttonLoader             js.Value
		// Unchanging elements
		document          js.Value
		board             js.Value
		matchingServerURI string
		origin            string
		client            *http.Client
	}
)

type RemoteMatchModel struct {
	opponentName          string
	maxTimeMs             int64
	playerElapsedMs       int64
	opponentElapsedMs     int64
	opponentRequestedDraw bool
	playerRequestedDraw   bool
	endRemoteGameChan     chan bool
}

func (cm *ClientModel) ResetRemoteMatchModel() {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.remoteMatchModel = RemoteMatchModel{}
}

func (cm *ClientModel) GetGameType() GameType {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.gameType
}

func (cm *ClientModel) SetGameType(gameType GameType) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.gameType = gameType
}

func (cm *ClientModel) SetGame(game *model.Game) {
	cm.tableMutex.Lock()
	defer cm.tableMutex.Unlock()
	cm.table = game
}

func (cm *ClientModel) GetPromotionMoveRequest() model.MoveRequest {
	cm.tableMutex.Lock()
	defer cm.tableMutex.Unlock()
	return cm.promotionMoveRequest
}

func (cm *ClientModel) SetPromotionMoveRequest(moveRequest model.MoveRequest) {
	cm.tableMutex.Lock()
	defer cm.tableMutex.Unlock()
	cm.promotionMoveRequest = moveRequest
}

func (cm *ClientModel) MakeMove(moveRequest model.MoveRequest) error {
	cm.tableMutex.Lock()
	defer cm.tableMutex.Unlock()
	return cm.table.Move(moveRequest)
}

func (cm *ClientModel) GetDraggingElement() js.Value {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.elDragging
}

func (cm *ClientModel) SetDraggingElement(el js.Value) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.elDragging = el
}

func (cm *ClientModel) GetDraggingPiece() *model.Piece {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.pieceDragging
}

func (cm *ClientModel) SetPromotionWindow(el js.Value) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.promotionWindow = el
}

func (cm *ClientModel) GetPromotionWindow() js.Value {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.promotionWindow
}

func (cm *ClientModel) GetPiece(position model.Position) *model.Piece {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.table.GetBoard().Piece(position)
}

func (cm *ClientModel) SetDraggingPiece(position model.Position) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.pieceDragging = cm.table.GetBoard().Piece(position)
}

func (cm *ClientModel) GetDraggingOriginalTransform() js.Value {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.draggingOrigTransform
}

func (cm *ClientModel) SetDraggingOriginalTransform(el js.Value) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.draggingOrigTransform = el
}

func (cm *ClientModel) LockMouseDown() {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.isMouseDownLock.Lock()
	cm.isMouseDown = true
}

func (cm *ClientModel) UnlockMouseDown() {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	defer cm.isMouseDownLock.Unlock()
	cm.isMouseDown = false
}

func (cm *ClientModel) GetIsMouseDown() bool {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.isMouseDown
}

func (cm *ClientModel) GetClickOriginalPosition() model.Position {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.positionOriginal
}

func (cm *ClientModel) SetClickOriginalPosition(position model.Position) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.positionOriginal = position
}

func (cm *ClientModel) GetIsMatchmaking() bool {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.isMatchmaking
}

func (cm *ClientModel) SetIsMatchmaking(isMatchmaking bool) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.isMatchmaking = isMatchmaking
}

func (cm *ClientModel) GetIsMatched() bool {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.isMatched
}

func (cm *ClientModel) SetIsMatched(isMatched bool) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.isMatched = isMatched
}

func (cm *ClientModel) GetPlayerName() string {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.playerName
}

func (cm *ClientModel) SetPlayerName(name string) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.playerName = name
}

func (cm *ClientModel) GetOpponentName() string {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.remoteMatchModel.opponentName
}

func (cm *ClientModel) SetOpponentName(name string) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.remoteMatchModel.opponentName = name
}

func (cm *ClientModel) GetMaxTimeMs() int64 {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.remoteMatchModel.maxTimeMs
}

func (cm *ClientModel) SetMaxTimeMs(maxTimeMs int64) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.remoteMatchModel.maxTimeMs = maxTimeMs
}

func (cm *ClientModel) ClearRequestedDraw() {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.remoteMatchModel.playerRequestedDraw = false
	cm.remoteMatchModel.opponentRequestedDraw = false
}

func (cm *ClientModel) GetHasSession() bool {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.hasSession
}

func (cm *ClientModel) SetHasSession(hasSession bool) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.hasSession = hasSession
}

func (cm *ClientModel) GetWSConn() js.Value {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.wsConn
}

func (cm *ClientModel) SetWSConn(conn js.Value) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.wsConn = conn
}

func (cm *ClientModel) GetButtonLoader() js.Value {
	cm.cmMutex.RLock()
	defer cm.cmMutex.RUnlock()
	return cm.buttonLoader
}

func (cm *ClientModel) SetButtonLoader(buttonLoader js.Value) {
	cm.cmMutex.Lock()
	defer cm.cmMutex.Unlock()
	cm.buttonLoader = buttonLoader
}
