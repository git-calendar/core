//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/firu11/git-calendar-core/core"
)

// This is the starting point which gets called from JS.
func main() {
	jsonApi := core.NewJsonApi()

	RegisterCallbacks(jsonApi)

	select {} // block infinitely
}

func RegisterCallbacks(api core.JsonApi) {
	js.Global().Set("CalendarCore",
		js.ValueOf(map[string]any{ // we wrap each method
			"initialize": js.FuncOf(func(this js.Value, args []js.Value) any {
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
		}),
	)

	// tell JS we are ready
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
			if err != nil { // create a JS new Error(message) and invoke it ("throw" it or whatever)
				// get the error text (default behavior)
				errorMessage := err.Error()

				// check if the error is a wrapper for a JS value; if it is, try to get the "message" property and use it instead
				if jsErr, ok := err.(js.Error); ok {
					message := jsErr.Value.Get("message")
					if message.Truthy() {
						errorMessage = message.String()
					}
				}

				// create the JS Error object
				errorConstructor := js.Global().Get("Error")
				reject.Invoke(errorConstructor.New(errorMessage))
			} else { // no error, pass the result
				resolve.Invoke(js.ValueOf(res))
			}
		}()

		return nil
	})

	// return a JS Promise
	promiseClass := js.Global().Get("Promise")
	return promiseClass.New(handler)
}
