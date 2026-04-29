//go:build js && wasm

package idb

import (
	"fmt"
	"syscall/js"
	"time"
)

const reqTimeout = 2 * time.Second // should be more than enough

type Transaction struct {
	pending []*Operation
}

type opType int

const (
	opGet opType = iota
	opPut
	opDelete
	opGetAllKeys
)

type Operation struct {
	typee opType
	store string
	key   string
	value js.Value

	result js.Value
	err    error

	done chan int
}

// Creates a transaction-like interface. The real JS IDB tx is created and commited inside Tx.Commit().
func NewTx() *Transaction {
	return &Transaction{
		pending: make([]*Operation, 0),
	}
}

// JS: store.get(key)
func (tx *Transaction) Get(store, key string) *Operation {
	op := &Operation{
		typee: opGet,
		store: store,
		key:   key,
		done:  make(chan int, 1),
	}
	tx.pending = append(tx.pending, op)
	return op
}

// JS: store.delete(key)
func (tx *Transaction) Delete(store, key string) *Operation {
	op := &Operation{
		typee: opDelete,
		store: store,
		key:   key,
		done:  make(chan int, 1),
	}
	tx.pending = append(tx.pending, op)
	return op
}

// JS: store.put(value, key)
func (tx *Transaction) Put(store, key string, value js.Value) *Operation {
	op := &Operation{
		typee: opPut,
		store: store,
		key:   key,
		value: value,
		done:  make(chan int, 1),
	}
	tx.pending = append(tx.pending, op)
	return op
}

// JS: store.getAllKeys(query)
func (tx *Transaction) GetAllKeys(store string, query js.Value) *Operation {
	op := &Operation{
		typee: opGetAllKeys,
		store: store,
		value: query,
		done:  make(chan int, 1),
	}
	tx.pending = append(tx.pending, op)
	return op
}

// It creates the real JS IDB tx, runs all the queued request/queries and waits for them to finish.
func (tx *Transaction) Commit(db js.Value) error {
	if len(tx.pending) == 0 {
		return nil
	}

	// duplicate store names
	storeSet := make(map[string]bool)
	stores := js.Global().Get("Array").New()
	for _, op := range tx.pending {
		if !storeSet[op.store] {
			stores.Call("push", op.store)
			storeSet[op.store] = true
		}
	}

	// create the transaction
	idbTx := db.Call("transaction", stores, "readwrite") // TODO: not only readwrite

	// run requests/queries
	for _, op := range tx.pending {
		store := idbTx.Call("objectStore", op.store)
		req := tx.dispatch(store, op)
		tx.bind(req, op)
	}

	return tx.waitAll()
}

// A helper to call the JS methods.
func (tx *Transaction) dispatch(store js.Value, op *Operation) js.Value {
	switch op.typee {
	case opGet:
		return store.Call("get", op.key)
	case opPut:
		return store.Call("put", op.value, op.key)
	case opDelete:
		return store.Call("delete", op.key)
	case opGetAllKeys:
		if op.value.IsNull() || op.value.IsUndefined() { // jsArray.Truthy() is always true even if empty
			return store.Call("getAllKeys")
		}
		return store.Call("getAllKeys", op.value)
	default:
		panic("unknown op")
	}
}

// Binds the callbacks for specified op.
func (tx *Transaction) bind(req js.Value, op *Operation) {
	var success, fail js.Func

	success = js.FuncOf(func(this js.Value, args []js.Value) any {
		// release memory
		defer success.Release()
		defer fail.Release()

		op.result = args[0].Get("target").Get("result")
		op.done <- 1
		return nil
	})

	fail = js.FuncOf(func(this js.Value, args []js.Value) any {
		// release memory
		defer success.Release()
		defer fail.Release()

		errObj := args[0].Get("target").Get("error")
		op.err = fmt.Errorf("IDB error: %s", errObj.String())
		op.done <- 1
		return nil
	})

	req.Set("onsuccess", success)
	req.Set("onerror", fail)
}

// Waits for all op
func (tx *Transaction) waitAll() error {
	timeout := time.After(reqTimeout)

	for _, op := range tx.pending {
		select {
		case <-op.done:
			if op.err != nil {
				return op.err
			}
		case <-timeout:
			return fmt.Errorf("transaction timeout after %v", reqTimeout)
		}
	}
	return nil
}

// Result simply returns the stored value, safe to call multiple times.
func (op *Operation) Result() js.Value {
	return op.result
}
