//go:build wasm && js && webclient

package main

import (
	"syscall/js"
)

func (clientModel *ClientModel) initStyle() {
}

func (clientModel *ClientModel) buttonBeginLoading(button js.Value) {
	buttonLoader := clientModel.document.Call("createElement", "div")
	clientModel.SetButtonLoader(buttonLoader)
	buttonLoader.Get("classList").Call("add", "loading")
	button.Call("appendChild", buttonLoader)
}

func addClass(element js.Value, class string) {
	element.Get("classList").Call("add", class)
}

func removeClass(element js.Value, class string) {
	element.Get("classList").Call("remove", class)
}
