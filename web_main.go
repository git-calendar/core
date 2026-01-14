//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/firu11/git-calendar-core/core"
)

func main() {
	jsonApi := core.NewJsonApi()

	RegisterCallbacks(jsonApi)

	select {} // block infinitely
}

func RegisterCallbacks(api core.JsonApi) {
	// we wrap each method
	js.Global().Set("CalendarCore", js.ValueOf(map[string]any{
		"initialize": js.FuncOf(func(this js.Value, args []js.Value) any {
			// error handling: returns a promise to JS
			return wrapPromise(func() (any, error) {
				return nil, api.Initialize()
			})
		}),
		"addEvent": js.FuncOf(func(this js.Value, args []js.Value) any {
			return wrapPromise(func() (any, error) {
				return nil, api.AddEvent(args[0].String())
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
		"setCorsProxy": js.FuncOf(func(this js.Value, args []js.Value) any {
			return wrapPromise(func() (any, error) {
				return nil, api.SetCorsProxy(args[0].String())
			})
		}),
		"clone": js.FuncOf(func(this js.Value, args []js.Value) any {
			return wrapPromise(func() (any, error) {
				return nil, api.Clone(args[0].String())
			})
		}),
		// TODO others
	}))

	if readyFn := js.Global().Get("onWasmReady"); readyFn.Type() == js.TypeFunction {
		readyFn.Invoke()
	}
}

// helper to handle the async nature and error throwing of JS
func wrapPromise(fn func() (any, error)) any {
	handler := js.FuncOf(func(this js.Value, args []js.Value) any {
		resolve := args[0]
		reject := args[1]

		go func() {
			res, err := fn()
			if err != nil {
				// cretes a JS new Error(message) and invokes it (throws it whatever)

				// Check if the error is a wrapper for a JS value
				// If it is, try to get the 'message' property
				errorMessage := err.Error()

				// This is a trick to see if the error contains a JS value
				if jsErr, ok := err.(js.Error); ok {
					message := jsErr.Value.Get("message")
					if message.Truthy() {
						errorMessage = message.String()
					}
				}

				errorConstructor := js.Global().Get("Error")
				reject.Invoke(errorConstructor.New(errorMessage))
			} else {
				resolve.Invoke(js.ValueOf(res))
			}
		}()
		return nil
	})

	promiseClass := js.Global().Get("Promise")
	return promiseClass.New(handler)
}
