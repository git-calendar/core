//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/git-calendar/core/pkg/api"
)

var a *api.Api

func main() {
	a = api.NewApi()
	js.Global().Call("__appReady")
	select {}
}

func GetEvents(from, to string) (string, error) {
	return a.GetEvents(from, to)
}

func LoadCalendars() error {
	return a.LoadCalendars()
}

func ListCalendars() (string, error) {
	return a.ListCalendars()
}
