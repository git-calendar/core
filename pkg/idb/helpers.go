//go:build js && wasm

package idb

import (
	"fmt"
	"math/rand/v2"
	"syscall/js"
)

// onUpgrade is called if db version is higher or DB does not exist. It's like custom migration method.
func awaitOpen(req js.Value, onUpgrade func(db js.Value)) (js.Value, error) {
	resultCh := make(chan js.Value, 1)
	errCh := make(chan error, 1)

	success := js.FuncOf(func(this js.Value, args []js.Value) any {
		resultCh <- req.Get("result")
		return nil
	})

	fail := js.FuncOf(func(this js.Value, args []js.Value) any {
		errObj := args[0].Get("error")
		errCh <- fmt.Errorf("failed to open indexeddb: %s", errObj.String())
		return nil
	})

	upgrade := js.FuncOf(func(this js.Value, args []js.Value) any {
		db := req.Get("result")
		onUpgrade(db)
		return nil
	})

	req.Set("onsuccess", success)
	req.Set("onerror", fail)
	req.Set("onupgradeneeded", upgrade) // version is higher or DB does not exist

	defer success.Release()
	defer fail.Release()
	defer upgrade.Release()

	select {
	case err := <-errCh:
		return js.Null(), err
	case res := <-resultCh:
		return res, nil
	}
}

const letters string = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = letters[rand.IntN(len(letters))]
	}
	return string(b)
}
