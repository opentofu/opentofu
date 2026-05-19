package streamapi

import (
	"context"
	"encoding/json"
	"io"
	"sync"
)

// MainLoop is the main entrypoint to the streaming API.
//
// Pass the reader and writer to use for communication with the calling program.
// The protocol in each direction involves a series of minified JSON objects
// separated by newline ("\n"), with the reader and writer running
// asynchronously from one another so both caller and callee are expected to
// keep a table of active requests.
//
// This function does not return. The caller is expected to terminate the
// process using a signal once all the work is finished.
func MainLoop(ctx context.Context, in io.Reader, out io.Writer) {
	dec := json.NewDecoder(in)
	enc := json.NewEncoder(out)
	inst := &Instance{
		in:  dec,
		out: enc,
	}
	inst.runLoop(ctx)
}

type Instance struct {
	in  *json.Decoder
	out *json.Encoder

	// We must hold outMu whenever writing to "out" to ensure that
	// concurrently-written messages are not interleaved.
	outMu sync.Mutex
}

func (inst *Instance) runLoop(_ context.Context) {
	for {
		var msg any
		err := inst.in.Decode(&msg)
		if err != nil {
			// TODO: something better
			panic(err)
		}

		inst.outMu.Lock()
		err = inst.out.Encode(msg)
		inst.outMu.Unlock()
		if err != nil {
			// TODO: something better
			panic(err)
		}
	}
}
