package quickjs

import (
	"fmt"
	"os"
	"runtime/cgo"
	"unsafe"
)

/*
#include <stdint.h> // for uintptr_t
#include "bridge.h"
*/
import "C"

// Context represents a Javascript context (or Realm). Each JSContext has its own global objects and system objects. There can be several JSContexts per JSRuntime and they can share objects, similar to frames of the same origin sharing Javascript objects in a web browser.
type Context struct {
	runtime    *Runtime
	ref        *C.JSContext
	globals    *Value
	proxy      *Value
	asyncProxy *Value
}

// Runtime returns the runtime of the context.
func (ctx *Context) Runtime() *Runtime {
	return ctx.runtime
}

// Free will free context and all associated objects.
func (ctx *Context) Close() {
	if ctx.proxy != nil {
		ctx.proxy.Free()
	}

	if ctx.asyncProxy != nil {
		ctx.asyncProxy.Free()
	}

	if ctx.globals != nil {
		ctx.globals.Free()
	}

	C.JS_FreeContext(ctx.ref)
}

// Null return a null value.
func (ctx *Context) Null() Value {
	return Value{ctx: ctx, ref: C.JS_NewNull()}
}

// Undefined return a undefined value.
func (ctx *Context) Undefined() Value {
	return Value{ctx: ctx, ref: C.JS_NewUndefined()}
}

// Uninitialized returns a uninitialized value.
func (ctx *Context) Uninitialized() Value {
	return Value{ctx: ctx, ref: C.JS_NewUninitialized()}
}

// Error returns a new exception value with given message.
func (ctx *Context) Error(err error) Value {
	val := Value{ctx: ctx, ref: C.JS_NewError(ctx.ref)}
	val.Set("message", ctx.String(err.Error()))
	return val
}

// Bool returns a bool value with given bool.
func (ctx *Context) Bool(b bool) Value {
	bv := 0
	if b {
		bv = 1
	}
	return Value{ctx: ctx, ref: C.JS_NewBool(ctx.ref, C.int(bv))}
}

// Int32 returns a int32 value with given int32.
func (ctx *Context) Int32(v int32) Value {
	return Value{ctx: ctx, ref: C.JS_NewInt32(ctx.ref, C.int32_t(v))}
}

// Int64 returns a int64 value with given int64.
func (ctx *Context) Int64(v int64) Value {
	return Value{ctx: ctx, ref: C.JS_NewInt64(ctx.ref, C.int64_t(v))}
}

// Uint32 returns a uint32 value with given uint32.
func (ctx *Context) Uint32(v uint32) Value {
	return Value{ctx: ctx, ref: C.JS_NewUint32(ctx.ref, C.uint32_t(v))}
}

// BigInt64 returns a int64 value with given uint64.
func (ctx *Context) BigInt64(v int64) Value {
	return Value{ctx: ctx, ref: C.JS_NewBigInt64(ctx.ref, C.int64_t(v))}
}

// BigUint64 returns a uint64 value with given uint64.
func (ctx *Context) BigUint64(v uint64) Value {
	return Value{ctx: ctx, ref: C.JS_NewBigUint64(ctx.ref, C.uint64_t(v))}
}

// Float64 returns a float64 value with given float64.
func (ctx *Context) Float64(v float64) Value {
	return Value{ctx: ctx, ref: C.JS_NewFloat64(ctx.ref, C.double(v))}
}

// String returns a string value with given string.
func (ctx *Context) String(v string) Value {
	ptr := C.CString(v)
	defer C.free(unsafe.Pointer(ptr))
	return Value{ctx: ctx, ref: C.JS_NewString(ctx.ref, ptr)}
}

// ArrayBuffer returns a string value with given binary data.
func (ctx *Context) ArrayBuffer(binaryData []byte) Value {
	return Value{ctx: ctx, ref: C.JS_NewArrayBufferCopy(ctx.ref, (*C.uchar)(&binaryData[0]), C.size_t(len(binaryData)))}
}

// Object returns a new object value.
func (ctx *Context) Object() Value {
	return Value{ctx: ctx, ref: C.JS_NewObject(ctx.ref)}
}

// ParseJson parses given json string and returns a object value.
func (ctx *Context) ParseJSON(v string) Value {
	ptr := C.CString(v)
	defer C.free(unsafe.Pointer(ptr))

	filenamePtr := C.CString("")
	defer C.free(unsafe.Pointer(filenamePtr))

	return Value{ctx: ctx, ref: C.JS_ParseJSON(ctx.ref, ptr, C.size_t(len(v)), filenamePtr)}
}

// Array returns a new array value.
func (ctx *Context) Array() *Array {
	val := Value{ctx: ctx, ref: C.JS_NewArray(ctx.ref)}
	return NewQjsArray(val, ctx)
}

func (ctx *Context) Map() *Map {
	ctor := ctx.Globals().Get("Map")
	defer ctor.Free()
	val := Value{ctx: ctx, ref: C.JS_CallConstructor(ctx.ref, ctor.ref, 0, nil)}
	return NewQjsMap(val, ctx)
}

func (ctx *Context) Set() *Set {
	ctor := ctx.Globals().Get("Set")
	defer ctor.Free()
	val := Value{ctx: ctx, ref: C.JS_CallConstructor(ctx.ref, ctor.ref, 0, nil)}
	return NewQjsSet(val, ctx)
}

// Function returns a js function value with given function template.
func (ctx *Context) Function(fn func(ctx *Context, this Value, args []Value) Value) Value {
	if ctx.proxy == nil {
		ctx.proxy = &Value{
			ctx: ctx,
			ref: C.JS_NewCFunction(ctx.ref, (*C.JSCFunction)(unsafe.Pointer(C.InvokeProxy)), nil, C.int(0)),
		}
	}

	fnHandler := ctx.Int64(int64(cgo.NewHandle(fn)))
	ctxHandler := ctx.Int64(int64(cgo.NewHandle(ctx)))
	args := []C.JSValue{ctx.proxy.ref, fnHandler.ref, ctxHandler.ref}

	val, err := ctx.Eval(`(proxy, fnHandler, ctx) => function() { return proxy.call(this, fnHandler, ctx, ...arguments); }`)
	defer val.Free()
	if err != nil {
		panic(err)
	}

	return Value{ctx: ctx, ref: C.JS_Call(ctx.ref, val.ref, ctx.Null().ref, C.int(len(args)), &args[0])}
}

// AsyncFunction returns a js async function value with given function template.
func (ctx *Context) AsyncFunction(asyncFn func(ctx *Context, this Value, promise Value, args []Value) Value) Value {
	if ctx.asyncProxy == nil {
		ctx.asyncProxy = &Value{
			ctx: ctx,
			ref: C.JS_NewCFunction(ctx.ref, (*C.JSCFunction)(unsafe.Pointer(C.InvokeAsyncProxy)), nil, C.int(0)),
		}
	}

	fnHandler := ctx.Int64(int64(cgo.NewHandle(asyncFn)))
	ctxHandler := ctx.Int64(int64(cgo.NewHandle(ctx)))
	args := []C.JSValue{ctx.asyncProxy.ref, fnHandler.ref, ctxHandler.ref}

	val, err := ctx.Eval(`(proxy, fnHandler, ctx) => async function(...arguments) {
		let resolve, reject;
		const promise = new Promise((resolve_, reject_) => {
		  resolve = resolve_;
		  reject = reject_;
		});
		promise.resolve = resolve;
		promise.reject = reject;

		proxy.call(this, fnHandler, ctx, promise,  ...arguments);
		return await promise;
	}`)
	defer val.Free()
	if err != nil {
		panic(err)
	}

	return Value{ctx: ctx, ref: C.JS_Call(ctx.ref, val.ref, ctx.Null().ref, C.int(len(args)), &args[0])}
}

// InterruptHandler is a function type for interrupt handler.
/* return != 0 if the JS code needs to be interrupted */
type InterruptHandler func() int

// SetInterruptHandler sets a interrupt handler.
func (ctx *Context) SetInterruptHandler(handler InterruptHandler) {
	handlerArgs := C.handlerArgs{
		fn: (C.uintptr_t)(cgo.NewHandle(handler)),
	}
	C.SetInterruptHandler(ctx.runtime.ref, unsafe.Pointer(&handlerArgs))
}

// Atom returns a new Atom value with given string.
func (ctx *Context) Atom(v string) Atom {
	ptr := C.CString(v)
	defer C.free(unsafe.Pointer(ptr))
	return Atom{ctx: ctx, ref: C.JS_NewAtom(ctx.ref, ptr)}
}

// Atom returns a new Atom value with given idx.
func (ctx *Context) AtomIdx(idx int64) Atom {
	return Atom{ctx: ctx, ref: C.JS_NewAtomUInt32(ctx.ref, C.uint32_t(idx))}
}

// Invoke invokes a function with given this value and arguments.
func (ctx *Context) Invoke(fn Value, this Value, args ...Value) Value {
	cargs := []C.JSValue{}
	for _, x := range args {
		cargs = append(cargs, x.ref)
	}
	if len(cargs) == 0 {
		return Value{ctx: ctx, ref: C.JS_Call(ctx.ref, fn.ref, this.ref, 0, nil)}
	}
	return Value{ctx: ctx, ref: C.JS_Call(ctx.ref, fn.ref, this.ref, C.int(len(cargs)), &cargs[0])}
}

type EvalOptions struct {
	js_eval_type_global       bool
	js_eval_type_module       bool
	js_eval_flag_strict       bool
	js_eval_flag_strip        bool
	js_eval_flag_compile_only bool
	filename                  string
	await                     bool
}

type EvalOption func(*EvalOptions)

func EvalFlagGlobal(global bool) EvalOption {
	return func(flags *EvalOptions) {
		flags.js_eval_type_global = global
	}
}

func EvalFlagModule(module bool) EvalOption {
	return func(flags *EvalOptions) {
		flags.js_eval_type_module = module
	}
}

func EvalFlagStrict(strict bool) EvalOption {
	return func(flags *EvalOptions) {
		flags.js_eval_flag_strict = strict
	}
}

func EvalFlagStrip(strip bool) EvalOption {
	return func(flags *EvalOptions) {
		flags.js_eval_flag_strip = strip
	}
}

func EvalFlagCompileOnly(compileOnly bool) EvalOption {
	return func(flags *EvalOptions) {
		flags.js_eval_flag_compile_only = compileOnly
	}
}

func EvalFileName(filename string) EvalOption {
	return func(flags *EvalOptions) {
		flags.filename = filename
	}
}

func EvalAwait(await bool) EvalOption {
	return func(flags *EvalOptions) {
		flags.await = await
	}
}

// Eval returns a js value with given code.
// Need call Free() `quickjs.Value`'s returned by `Eval()` and `EvalFile()` and `EvalBytecode()`.
// func (ctx *Context) Eval(code string) (Value, error) { return ctx.EvalFile(code, "code") }
func (ctx *Context) Eval(code string, opts ...EvalOption) (Value, error) {
	options := EvalOptions{
		js_eval_type_global: true,
		filename:            "<input>",
		await:               false,
	}
	for _, fn := range opts {
		fn(&options)
	}

	cFlag := C.int(0)
	if options.js_eval_type_global {
		cFlag |= C.JS_EVAL_TYPE_GLOBAL
	}
	if options.js_eval_type_module {
		cFlag |= C.JS_EVAL_TYPE_MODULE
	}
	if options.js_eval_flag_strict {
		cFlag |= C.JS_EVAL_FLAG_STRICT
	}
	if options.js_eval_flag_strip {
		cFlag |= C.JS_EVAL_FLAG_STRIP
	}
	if options.js_eval_flag_compile_only {
		cFlag |= C.JS_EVAL_FLAG_COMPILE_ONLY
	}

	codePtr := C.CString(code)
	defer C.free(unsafe.Pointer(codePtr))

	filenamePtr := C.CString(options.filename)
	defer C.free(unsafe.Pointer(filenamePtr))

	if C.JS_DetectModule(codePtr, C.size_t(len(code))) != 0 {
		cFlag |= C.JS_EVAL_TYPE_MODULE
	}

	var val Value
	if options.await {
		val = Value{ctx: ctx, ref: C.js_std_await(ctx.ref, C.JS_Eval(ctx.ref, codePtr, C.size_t(len(code)), filenamePtr, cFlag))}
	} else {
		val = Value{ctx: ctx, ref: C.JS_Eval(ctx.ref, codePtr, C.size_t(len(code)), filenamePtr, cFlag)}
	}
	if val.IsException() {
		return val, ctx.Exception()
	}

	return val, nil
}

// EvalFile returns a js value with given code and filename.
// Need call Free() `quickjs.Value`'s returned by `Eval()` and `EvalFile()` and `EvalBytecode()`.
func (ctx *Context) EvalFile(filePath string, opts ...EvalOption) (Value, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return ctx.Null(), err
	}
	opts = append(opts, EvalFileName(filePath))
	return ctx.Eval(string(b), opts...)
}

// LoadModule returns a js value with given code and module name.
func (ctx *Context) LoadModule(code string, moduleName string) (Value, error) {
	codePtr := C.CString(code)
	defer C.free(unsafe.Pointer(codePtr))

	filenamePtr := C.CString(moduleName)
	defer C.free(unsafe.Pointer(filenamePtr))

	cFlag := C.JS_EVAL_TYPE_MODULE | C.JS_EVAL_FLAG_COMPILE_ONLY
	cVal := C.JS_Eval(ctx.ref, codePtr, C.size_t(len(code)), filenamePtr, C.int(cFlag))
	if C.ValueGetTag(cVal) != C.JS_TAG_MODULE {
		return ctx.Null(), fmt.Errorf("not a module")
	}
	if C.JS_ResolveModule(ctx.ref, cVal) != 0 {
		C.JS_FreeValue(ctx.ref, cVal)
		return ctx.Null(), fmt.Errorf("resolve module failed")
	}
	C.js_module_set_import_meta(ctx.ref, cVal, 0, 1)
	cVal = C.js_std_await(ctx.ref, cVal)

	return Value{ctx: ctx, ref: cVal}, nil
}

// LoadModuleFile returns a js value with given file path and module name.
func (ctx *Context) LoadModuleFile(filePath string, moduleName string) (Value, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return ctx.Null(), err
	}
	return ctx.LoadModule(string(b), moduleName)
}

// CompileModule returns a compiled bytecode with given code and module name.
func (ctx *Context) CompileModule(filePath string, moduleName string, opts ...EvalOption) ([]byte, error) {
	opts = append(opts, EvalFileName(moduleName))
	return ctx.CompileFile(filePath, opts...)
}

// LoadModuleByteCode returns a js value with given bytecode and module name.
func (ctx *Context) LoadModuleBytecode(buf []byte) (Value, error) {
	cbuf := C.CBytes(buf)
	cVal := C.JS_ReadObject(ctx.ref, (*C.uint8_t)(cbuf), C.size_t(len(buf)), C.JS_READ_OBJ_BYTECODE)
	defer C.js_free(ctx.ref, unsafe.Pointer(cbuf))
	if C.JS_IsException(cVal) == 1 {
		return ctx.Null(), ctx.Exception()
	}
	if C.ValueGetTag(cVal) != C.JS_TAG_MODULE {
		return ctx.Null(), fmt.Errorf("not a module")
	}
	if C.JS_ResolveModule(ctx.ref, cVal) != 0 {
		C.JS_FreeValue(ctx.ref, cVal)
		return ctx.Null(), fmt.Errorf("resolve module failed")
	}
	C.js_module_set_import_meta(ctx.ref, cVal, 0, 1)
	cVal = C.js_std_await(ctx.ref, cVal)

	return Value{ctx: ctx, ref: cVal}, nil
}

// EvalBytecode returns a js value with given bytecode.
// Need call Free() `quickjs.Value`'s returned by `Eval()` and `EvalFile()` and `EvalBytecode()`.
func (ctx *Context) EvalBytecode(buf []byte) (Value, error) {
	cbuf := C.CBytes(buf)
	obj := Value{ctx: ctx, ref: C.JS_ReadObject(ctx.ref, (*C.uint8_t)(cbuf), C.size_t(len(buf)), C.JS_READ_OBJ_BYTECODE)}
	defer C.js_free(ctx.ref, unsafe.Pointer(cbuf))
	if obj.IsException() {
		return obj, ctx.Exception()
	}

	val := Value{ctx: ctx, ref: C.JS_EvalFunction(ctx.ref, obj.ref)}
	if val.IsException() {
		return val, ctx.Exception()
	}

	return val, nil
}

// Compile returns a compiled bytecode with given code.
func (ctx *Context) Compile(code string, opts ...EvalOption) ([]byte, error) {
	opts = append(opts, EvalFlagCompileOnly(true))
	val, err := ctx.Eval(code, opts...)
	if err != nil {
		return nil, err
	}
	defer val.Free()

	var kSize C.size_t
	ptr := C.JS_WriteObject(ctx.ref, &kSize, val.ref, C.JS_WRITE_OBJ_BYTECODE)
	defer C.js_free(ctx.ref, unsafe.Pointer(ptr))
	if C.int(kSize) <= 0 {
		return nil, ctx.Exception()
	}

	ret := make([]byte, C.int(kSize))
	copy(ret, C.GoBytes(unsafe.Pointer(ptr), C.int(kSize)))

	return ret, nil
}

// Compile returns a compiled bytecode with given filename.
func (ctx *Context) CompileFile(filePath string, opts ...EvalOption) ([]byte, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	options := EvalOptions{}
	for _, fn := range opts {
		fn(&options)
	}
	if options.filename == "" {
		opts = append(opts, EvalFileName(filePath))
	}

	return ctx.Compile(string(b), opts...)
}

// Global returns a context's global object.
func (ctx *Context) Globals() Value {
	if ctx.globals == nil {
		ctx.globals = &Value{
			ctx: ctx,
			ref: C.JS_GetGlobalObject(ctx.ref),
		}
	}
	return *ctx.globals
}

// Throw returns a context's exception value.
func (ctx *Context) Throw(v Value) Value {
	return Value{ctx: ctx, ref: C.JS_Throw(ctx.ref, v.ref)}
}

// ThrowError returns a context's exception value with given error message.
func (ctx *Context) ThrowError(err error) Value {
	return ctx.Throw(ctx.Error(err))
}

// ThrowSyntaxError returns a context's exception value with given error message.
func (ctx *Context) ThrowSyntaxError(format string, args ...interface{}) Value {
	cause := fmt.Sprintf(format, args...)
	causePtr := C.CString(cause)
	defer C.free(unsafe.Pointer(causePtr))
	return Value{ctx: ctx, ref: C.ThrowSyntaxError(ctx.ref, causePtr)}
}

// ThrowTypeError returns a context's exception value with given error message.
func (ctx *Context) ThrowTypeError(format string, args ...interface{}) Value {
	cause := fmt.Sprintf(format, args...)
	causePtr := C.CString(cause)
	defer C.free(unsafe.Pointer(causePtr))
	return Value{ctx: ctx, ref: C.ThrowTypeError(ctx.ref, causePtr)}
}

// ThrowReferenceError returns a context's exception value with given error message.
func (ctx *Context) ThrowReferenceError(format string, args ...interface{}) Value {
	cause := fmt.Sprintf(format, args...)
	causePtr := C.CString(cause)
	defer C.free(unsafe.Pointer(causePtr))
	return Value{ctx: ctx, ref: C.ThrowReferenceError(ctx.ref, causePtr)}
}

// ThrowRangeError returns a context's exception value with given error message.
func (ctx *Context) ThrowRangeError(format string, args ...interface{}) Value {
	cause := fmt.Sprintf(format, args...)
	causePtr := C.CString(cause)
	defer C.free(unsafe.Pointer(causePtr))
	return Value{ctx: ctx, ref: C.ThrowRangeError(ctx.ref, causePtr)}
}

// ThrowInternalError returns a context's exception value with given error message.
func (ctx *Context) ThrowInternalError(format string, args ...interface{}) Value {
	cause := fmt.Sprintf(format, args...)
	causePtr := C.CString(cause)
	defer C.free(unsafe.Pointer(causePtr))
	return Value{ctx: ctx, ref: C.ThrowInternalError(ctx.ref, causePtr)}
}

// Exception returns a context's exception value.
func (ctx *Context) Exception() error {
	val := Value{ctx: ctx, ref: C.JS_GetException(ctx.ref)}
	defer val.Free()
	return val.Error()
}

// Loop runs the context's event loop.
func (ctx *Context) Loop() {
	C.js_std_loop(ctx.ref)
}

// Wait for a promise and execute pending jobs while waiting for it. Return the promise result or JS_EXCEPTION in case of promise rejection.
func (ctx *Context) Await(v Value) (Value, error) {
	val := Value{ctx: ctx, ref: C.js_std_await(ctx.ref, v.ref)}
	if val.IsException() {
		return val, ctx.Exception()
	}
	return val, nil
}
