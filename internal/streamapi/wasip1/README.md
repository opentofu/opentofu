To build the WebAssembly module:

    GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o main.wasm
