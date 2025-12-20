//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/firu11/git-calendar-core/core"
)

func RegisterCallbacks(api core.Api) {
	// we wrap each method
	js.Global().Set("api", js.ValueOf(map[string]any{
		"initialize": js.FuncOf(func(this js.Value, args []js.Value) any {
			// error handling: returns a promise to JS
			return wrapPromise(func() (any, error) {
				err := api.Initialize(args[0].String())
				return nil, err
			})
		}),
		"addEvent": js.FuncOf(func(this js.Value, args []js.Value) any {
			return wrapPromise(func() (any, error) {
				err := api.AddEvent(args[0].String())
				return nil, err
			})
		}),
		"getEvent": js.FuncOf(func(this js.Value, args []js.Value) any {
			return wrapPromise(func() (any, error) {
				return api.GetEvent(args[0].Int())
			})
		}),
		"getEvents": js.FuncOf(func(this js.Value, args []js.Value) any {
			return wrapPromise(func() (any, error) {
				return api.GetEvents(int64(args[0].Int()), int64(args[1].Int()))
			})
		}),
		// TODO others
	}))
}

// helper to handle the async nature and error throwing of JS
func wrapPromise(fn func() (any, error)) any {
	handler := js.FuncOf(func(this js.Value, args []js.Value) any {
		resolve := args[0]
		reject := args[1]

		go func() {
			res, err := fn()
			if err != nil {
				reject.Invoke(js.ValueOf(err.Error()))
			} else {
				resolve.Invoke(js.ValueOf(res))
			}
		}()
		return nil
	})

	promiseClass := js.Global().Get("Promise")
	return promiseClass.New(handler)
}
