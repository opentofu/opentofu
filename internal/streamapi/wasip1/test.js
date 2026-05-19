async function run() {
    var mem;
    const { instance } = await WebAssembly.instantiateStreaming(fetch('main.wasm'), {
        wasi_snapshot_preview1: {
            sched_yield: function () {
                return 0;
            },
            proc_exit: function (status) {
                console.log('exited with status', status);
            },
            args_get: function (argvPtr, argvBufPtr) {
                // args_sizes_get returns argc=0, so we have nothing to do here.
                return 0;
            },
            args_sizes_get: function(argcPtr, argvBufSizePtr) {
                const mem = new DataView(instance.exports.memory.buffer);

                // No arguments at all
                mem.setUint32(argcPtr, 0, true);
                mem.setUint32(argvBufSizePtr, 0, true);
                return 0;
            },
            clock_time_get: function (clockId, precision, outPtr) {
                const mem = new DataView(instance.exports.memory.buffer);

                const now = BigInt(Date.now()) * 1000000n;
                mem.setBigUint64(outPtr, now, true);
                return 0;
            },
            environ_get: function(environPtr, environBufPtr) {
                // environ_sizes_get returns environLenPtr=0, so we have nothing to do here
                return 0;
            },
            environ_sizes_get: function(environLenPtr, environBufSizePtr) {
                const mem = new DataView(instance.exports.memory.buffer);

                // No environment variables at all
                mem.setUint32(environLenPtr, 0, true);
                mem.setUint32(environBufSizePtr, 0, true);
                return 0;
            },
            fd_write: function(fd, iovsPtr, iovsLen, writtenPtr) {
                console.log("TODO: write to fd", fd);
                return 0;
            },
            random_get: function(bufPtr, bufLen) {
                // TODO: actually write random data!
                return 0;
            },
            poll_oneoff: function(subsPtr, outPtr, subsLen, outLenPtr) {
                const mem = new DataView(instance.exports.memory.buffer);

                // TODO: Actually implement this. For now we just report that
                // nothing is ready, which should cause the Go runtime to
                // yield.
                mem.setUint32(outLenPtr, 0, true);
                return 0;
            },
            fd_close: function(fd) {
                return 0;
            },
            fd_read: function(fd, iovsPtr, iovsLen, readCountPtr) {
                const mem = new DataView(instance.exports.memory.buffer);

                console.log("TODO: read from fd", fd);
                mem.setUint32(readCountPtr, 0, true);
                return 0;
            },
            fd_fdstat_get: function (fd, fdStatPtr) {
                return 52; // ENOSYS
            },
            fd_fdstat_set_flags: function (fd, fdFlags) {
                return 52; // ENOSYS
            },
            fd_prestat_get: function (fd, prestatPtr) {
                return 52; // ENOSYS
            },
            fd_prestat_dir_name: function (fd, pathPtr, pathLen) {
                return 52; // ENOSYS
            },
        },
    });
    console.log('instance has ', instance.exports);
    instance.exports._initialize(); // Go runtime startup
    instance.exports.poll() // Initial poll to get things moving
}
run().then(function () {
    console.log('ran!');
});
