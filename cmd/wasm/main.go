//go:build js && wasm

package main

import (
	"syscall/js"
	_ "time/tzdata" // microslop dates need this

	"github.com/git-calendar/core/pkg/api"
)

// This is the starting point which gets called from JS.
func main() {
	api := api.NewApi()

	registerCallbacks(api)

	select {} // block infinitely
}

func registerCallbacks(api *api.Api) {
	js.Global().Set(
		"CalendarCore",
		js.ValueOf(map[string]any{ // we wrap each method
			"createCalendar": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.CreateCalendar(args[0].String(), args[1].String())
				})
			}),
			"cloneCalendar": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.CloneCalendar(args[0].String(), args[1].String(), args[2].Bool())
				})
			}),
			"removeCalendar": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.RemoveCalendar(args[0].String())
				})
			}),
			"renameCalendar": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.RenameCalendar(args[0].String(), args[1].String())
				})
			}),
			"listCalendars": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return api.ListCalendars()
				})
			}),
			"loadCalendars": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.LoadCalendars()
				})
			}),
			"importICalFile": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.ImportICalFile(args[0].String(), args[1].String(), args[2].String())
				})
			}),
			"importICalURL": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.ImportICalURL(args[0].String(), args[1].String())
				})
			}),
			"updateICalURL": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.UpdateICalURL(args[0].String(), args[1].String())
				})
			}),
			"updateRemote": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.UpdateRemote(args[0].String(), args[1].String(), args[2].Bool())
				})
			}),
			"createEvent": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return api.CreateEvent(args[0].String())
				})
			}),
			"updateEvent": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return api.UpdateEvent(args[0].String())
				})
			}),
			"updateRepeatingEvent": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return api.UpdateRepeatingEvent(args[0].String(), args[1].String(), args[2].Int())
				})
			}),
			"removeEvent": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.RemoveEvent(args[0].String())
				})
			}),
			"removeRepeatingEvent": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.RemoveRepeatingEvent(args[0].String(), args[1].Int())
				})
			}),
			"getEvent": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return api.GetEvent(args[0].String())
				})
			}),
			"getEvents": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					if args[2].IsNull() {
						return api.GetEvents(args[0].String(), args[1].String(), "")
					}
					return api.GetEvents(args[0].String(), args[1].String(), args[2].String())
				})
			}),
			"createTag": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return api.CreateTag(args[0].String(), args[1].String())
				})
			}),
			"updateTag": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return api.UpdateTag(args[0].String(), args[1].String())
				})
			}),
			"removeTag": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.RemoveTag(args[0].String(), args[1].String())
				})
			}),
			"setCorsProxy": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.SetCorsProxy(args[0].String())
				})
			}),
			"syncAll": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return nil, api.SyncAll()
				})
			}),
			"exportICal": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return api.ExportICal(args[0].String())
				})
			}),
			"exportZip": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					return api.ExportZip(args[0].String())
				})
			}),
			"restoreZip": js.FuncOf(func(this js.Value, args []js.Value) any {
				return wrapPromise(func() (any, error) {
					data := make([]byte, args[0].Get("byteLength").Int())
					js.CopyBytesToGo(data, args[0])
					return nil, api.RestoreZip(data)
				})
			}),
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
				resolve.Invoke(toJSValue(res))
			}
		}()

		return nil
	})

	// return a JS Promise
	promiseClass := js.Global().Get("Promise")
	return promiseClass.New(handler)
}

func toJSValue(v any) js.Value {
	switch x := v.(type) {
	case nil:
		return js.Null()
	case js.Value:
		return x
	case []byte:
		u8 := js.Global().Get("Uint8Array").New(len(x))
		js.CopyBytesToJS(u8, x)
		return u8
	default:
		return js.ValueOf(v)
	}
}
